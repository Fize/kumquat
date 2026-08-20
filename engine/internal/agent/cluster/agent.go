package cluster

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentmetrics "github.com/fize/kumquat/engine/internal/agent/metrics"
	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	"github.com/fize/kumquat/engine/pkg/constants"
	"github.com/fize/kumquat/engine/pkg/observability"
	"github.com/fize/kumquat/engine/pkg/scheme"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AgentOptions holds the configuration for the Agent
type AgentOptions struct {
	HubURL             string
	TunnelURL          string
	ClusterName        string
	BootstrapToken     string
	BootstrapTokenFile string
	HubCAFile          string
	TunnelCAFile       string
	HeartbeatInterval  time.Duration
}

// Agent is the edge agent that connects to the Hub
type Agent struct {
	Options     AgentOptions
	HubClient   client.Client
	HubConfig   *rest.Config
	LocalClient client.Client
}

var inClusterConfig = rest.InClusterConfig
var serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
var serviceAccountCAPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

// NewAgent creates a new Agent with the given options
func NewAgent(opts AgentOptions) *Agent {
	return &Agent{
		Options: opts,
	}
}

// InitHubClient initializes the client to talk to the Hub
func (a *Agent) InitHubClient() error {
	var config *rest.Config
	var err error

	hasBootstrapCredential := a.Options.BootstrapTokenFile != ""
	if (a.Options.HubURL == "") != !hasBootstrapCredential {
		return fmt.Errorf("hub URL and bootstrap token must be configured together")
	}
	if a.Options.HubURL != "" {
		if a.Options.HubCAFile == "" {
			return fmt.Errorf("hub CA file is required for remote authentication")
		}
		config = &rest.Config{
			Host: a.Options.HubURL,
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: a.Options.HubCAFile,
			},
		}
		config.BearerTokenFile = a.Options.BootstrapTokenFile
	} else {
		config, err = inClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to load in-cluster config: %w", err)
		}
	}

	c, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create hub client: %w", err)
	}

	a.HubClient = c
	a.HubConfig = config
	return nil
}

func (a *Agent) tunnelHeaders() (http.Header, error) {
	if a.Options.BootstrapTokenFile == "" {
		return nil, fmt.Errorf("tunnel token file is required")
	}
	data, err := os.ReadFile(a.Options.BootstrapTokenFile)
	if err != nil {
		return nil, fmt.Errorf("read tunnel token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return nil, fmt.Errorf("tunnel token is empty")
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("X-Kumquat-Cluster-Name", a.Options.ClusterName)
	headers.Set("X-Remotedialer-ID", a.Options.ClusterName)
	return headers, nil
}

func (a *Agent) tunnelTLSConfig() (*tls.Config, error) {
	if a.Options.TunnelCAFile == "" {
		return nil, fmt.Errorf("tunnel CA file is required")
	}
	caData, err := os.ReadFile(a.Options.TunnelCAFile)
	if err != nil {
		return nil, fmt.Errorf("read tunnel CA file: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("tunnel CA file contains no certificates")
	}
	u, err := url.Parse(a.Options.TunnelURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid tunnel URL")
	}
	return &tls.Config{RootCAs: caPool, ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}, nil
}

func (a *Agent) getClusterCredentials() (map[string]string, error) {
	creds := make(map[string]string)

	// Read CA
	caData, err := os.ReadFile(serviceAccountCAPath)
	if err == nil {
		creds[constants.AnnotationCredentialsCA] = base64.StdEncoding.EncodeToString(caData)
	}

	// Read Token
	tokenData, err := os.ReadFile(serviceAccountTokenPath)
	if err == nil {
		creds[constants.AnnotationCredentialsToken] = string(tokenData)
	}

	// Determine APIServer URL
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host != "" && port != "" {
		creds[constants.AnnotationAPIServerURL] = fmt.Sprintf("https://%s:%s", host, port)
	} else {
		creds[constants.AnnotationAPIServerURL] = constants.DefaultAPIServerURL
	}

	return creds, nil
}

func (a *Agent) getRequiredClusterCredentials() (map[string]string, error) {
	caData, err := os.ReadFile(serviceAccountCAPath)
	if err != nil {
		return nil, fmt.Errorf("read projected service account CA: %w", err)
	}
	tokenData, err := os.ReadFile(serviceAccountTokenPath)
	if err != nil {
		return nil, fmt.Errorf("read projected service account token: %w", err)
	}
	creds, _ := a.getClusterCredentials()
	creds[constants.AnnotationCredentialsCA] = base64.StdEncoding.EncodeToString(caData)
	creds[constants.AnnotationCredentialsToken] = strings.TrimSpace(string(tokenData))
	if !credentialsComplete(creds) {
		return nil, fmt.Errorf("projected cluster credentials are incomplete")
	}
	return creds, nil
}

func credentialsHash(creds map[string]string) string {
	keys := []string{constants.AnnotationAPIServerURL, constants.AnnotationCredentialsCA, constants.AnnotationCredentialsCert, constants.AnnotationCredentialsKey, constants.AnnotationCredentialsToken}
	h := sha256.New()
	for _, key := range keys {
		_, _ = h.Write([]byte(key))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(creds[key]))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func credentialsComplete(creds map[string]string) bool {
	hasCA := creds[constants.AnnotationCredentialsCA] != ""
	hasToken := creds[constants.AnnotationCredentialsToken] != ""
	hasClientCertificate := creds[constants.AnnotationCredentialsCert] != "" && creds[constants.AnnotationCredentialsKey] != ""
	return hasCA && (hasToken || hasClientCertificate)
}

func (a *Agent) Register(ctx context.Context) error {
	if a.HubClient == nil {
		return fmt.Errorf("hub client not initialized")
	}

	localReporter := a.Options.HubURL == ""
	creds := map[string]string{}
	if !localReporter {
		var err error
		creds, err = a.getRequiredClusterCredentials()
		if err != nil {
			return err
		}
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster := &clusterv1alpha1.ManagedCluster{}
		err := a.HubClient.Get(ctx, client.ObjectKey{Name: a.Options.ClusterName}, cluster)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to get cluster: %w", err)
			}

			if localReporter {
				return fmt.Errorf("local Hub reporter requires existing cluster %s", a.Options.ClusterName)
			}
			creds[constants.AnnotationCredentialsHash] = credentialsHash(creds)
			newCluster := &clusterv1alpha1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.Options.ClusterName,
					Annotations: creds,
					Labels: map[string]string{
						constants.LabelRegistrationSource: constants.RegistrationSourceAgent,
					},
				},
				Spec: clusterv1alpha1.ManagedClusterSpec{
					ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge,
				},
			}
			if err := a.HubClient.Create(ctx, newCluster); err != nil {
				return err
			}
			log.Log.Info("Registered new cluster", "cluster", a.Options.ClusterName)
			return nil
		}
		if localReporter && (cluster.Spec.ConnectionMode != clusterv1alpha1.ClusterConnectionModeHub || cluster.Spec.APIServer != clusterv1alpha1.LocalAPIServer) {
			return fmt.Errorf("local Hub reporter requires explicit local apiServer %q", clusterv1alpha1.LocalAPIServer)
		}
		if localReporter {
			return nil
		}

		original := cluster.DeepCopy()
		changed := false
		if cluster.Annotations == nil {
			cluster.Annotations = make(map[string]string)
		}
		if cluster.Labels == nil {
			cluster.Labels = make(map[string]string)
		}
		if cluster.Labels[constants.LabelRegistrationSource] != constants.RegistrationSourceAgent {
			cluster.Labels[constants.LabelRegistrationSource] = constants.RegistrationSourceAgent
			changed = true
		}
		hash := credentialsHash(creds)
		if cluster.Annotations[constants.AnnotationCredentialsHash] != hash {
			for k, v := range creds {
				cluster.Annotations[k] = v
			}
			cluster.Annotations[constants.AnnotationCredentialsHash] = hash
			changed = true
		}
		if changed {
			log.Log.Info("Updating cluster registration credentials", "cluster", a.Options.ClusterName)
			return a.HubClient.Patch(ctx, cluster, client.MergeFrom(original))
		}
		return nil
	})
}

func (a *Agent) StartHeartbeat(ctx context.Context) error {
	ticker := time.NewTicker(a.Options.HeartbeatInterval)
	defer ticker.Stop()

	log.Log.Info("Starting heartbeat loop", "interval", a.Options.HeartbeatInterval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				log.Log.Error(err, "Failed to send heartbeat")
			}
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	ctx, span := observability.Tracer().Start(ctx, "Agent.Heartbeat",
		trace.WithAttributes(
			attribute.String("cluster.name", a.Options.ClusterName),
		),
	)
	defer span.End()

	startTime := time.Now()
	cluster := &clusterv1alpha1.ManagedCluster{}
	if err := a.HubClient.Get(ctx, client.ObjectKey{Name: a.Options.ClusterName}, cluster); err != nil {
		agentmetrics.RecordHeartbeat("error", time.Since(startTime))
		observability.SpanError(ctx, err)
		return err
	}
	if a.Options.HubURL != "" {
		creds, err := a.getRequiredClusterCredentials()
		if err != nil {
			return err
		}
		hash := credentialsHash(creds)
		if cluster.Annotations == nil || cluster.Annotations[constants.AnnotationCredentialsHash] != hash {
			original := cluster.DeepCopy()
			if cluster.Annotations == nil {
				cluster.Annotations = map[string]string{}
			}
			for key, value := range creds {
				cluster.Annotations[key] = value
			}
			cluster.Annotations[constants.AnnotationCredentialsHash] = hash
			if err := a.HubClient.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
				return err
			}
			if err := a.HubClient.Get(ctx, client.ObjectKey{Name: a.Options.ClusterName}, cluster); err != nil {
				return err
			}
		}
	}

	now := metav1.Now()
	cluster.Status.LastKeepAliveTime = &now

	// Collect and update resource summary from local cluster
	if a.LocalClient != nil {
		if rs := a.collectResourceSummary(ctx); rs != nil {
			cluster.Status.ResourceSummary = rs
		}
		// Collect and update node summary from local cluster
		if ns := a.collectNodeSummary(ctx); ns != nil {
			cluster.Status.NodeSummary = ns
		}
	}

	if err := a.HubClient.Status().Update(ctx, cluster); err != nil {
		err = a.HubClient.Update(ctx, cluster)
		agentmetrics.RecordHeartbeat("error", time.Since(startTime))
		observability.SpanError(ctx, err)
		return err
	}
	agentmetrics.RecordHeartbeat("success", time.Since(startTime))
	return nil
}

// collectResourceSummary collects resource information from the local cluster nodes
// and returns a ResourceSummary slice for reporting to the Hub.
func (a *Agent) collectResourceSummary(ctx context.Context) []clusterv1alpha1.ResourceSummary {
	nodeList := &corev1.NodeList{}
	if err := a.LocalClient.List(ctx, nodeList); err != nil {
		log.Log.Error(err, "Failed to list nodes for resource summary")
		return nil
	}

	podList := &corev1.PodList{}
	if err := a.LocalClient.List(ctx, podList); err != nil {
		log.Log.Error(err, "Failed to list pods for resource summary")
		return nil
	}

	var totalAllocatable, totalAllocated corev1.ResourceList

	// Sum allocatable resources from all nodes
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if totalAllocatable == nil {
			totalAllocatable = make(corev1.ResourceList)
		}
		for name, quantity := range node.Status.Allocatable {
			if existing, ok := totalAllocatable[name]; ok {
				existing.Add(quantity)
				totalAllocatable[name] = existing
			} else {
				totalAllocatable[name] = quantity.DeepCopy()
			}
		}
	}

	// Sum allocated resources from running pods
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		if totalAllocated == nil {
			totalAllocated = make(corev1.ResourceList)
		}
		for _, container := range pod.Spec.Containers {
			for name, quantity := range container.Resources.Requests {
				if existing, ok := totalAllocated[name]; ok {
					existing.Add(quantity)
					totalAllocated[name] = existing
				} else {
					totalAllocated[name] = quantity.DeepCopy()
				}
			}
		}
	}

	if totalAllocatable == nil {
		return nil
	}

	if totalAllocated == nil {
		totalAllocated = make(corev1.ResourceList)
	}

	return []clusterv1alpha1.ResourceSummary{
		{
			Name:        "default",
			Allocatable: totalAllocatable,
			Allocated:   totalAllocated,
		},
	}
}

// collectNodeSummary collects node status information from the local cluster
// and returns a NodeSummary slice for reporting to the Hub.
func (a *Agent) collectNodeSummary(ctx context.Context) []clusterv1alpha1.NodeSummary {
	nodeList := &corev1.NodeList{}
	if err := a.LocalClient.List(ctx, nodeList); err != nil {
		log.Log.Error(err, "Failed to list nodes for node summary")
		return nil
	}

	var totalNum, readyNum int
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		totalNum++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				readyNum++
				break
			}
		}
	}

	return []clusterv1alpha1.NodeSummary{
		{
			Name:     "default",
			TotalNum: totalNum,
			ReadyNum: readyNum,
		},
	}
}

func (a *Agent) StartTunnel(ctx context.Context) error {
	url := fmt.Sprintf("%s/connect", a.Options.TunnelURL)
	if strings.HasPrefix(url, "https") {
		url = strings.Replace(url, "https", "wss", 1)
	} else if strings.HasPrefix(url, "http") {
		url = strings.Replace(url, "http", "ws", 1)
	}

	log.Log.Info("Starting tunnel", "url", url)

	agentmetrics.SetTunnelConnected(false)

	for {
		select {
		case <-ctx.Done():
			agentmetrics.SetTunnelConnected(false)
			return nil
		default:
			headers, err := a.tunnelHeaders()
			if err != nil {
				return err
			}
			tlsConfig, err := a.tunnelTLSConfig()
			if err != nil {
				return err
			}
			dialer := &websocket.Dialer{
				Proxy:            http.ProxyFromEnvironment,
				HandshakeTimeout: 45 * time.Second,
				TLSClientConfig:  tlsConfig,
			}
			_, span := observability.Tracer().Start(ctx, "Agent.TunnelConnect",
				trace.WithAttributes(
					attribute.String("cluster.name", a.Options.ClusterName),
					attribute.String("tunnel.url", url),
				),
			)

			log.Log.Info("Starting connection attempt", "url", url)
			err = remotedialer.ClientConnect(ctx, url, headers, dialer, func(proto, address string) bool {
				return true
			}, nil)
			if err != nil {
				log.Log.Error(err, "Tunnel disconnected")
				agentmetrics.RecordTunnelReconnect("error")
				observability.SpanError(ctx, err)
				span.End()
			} else {
				log.Log.Info("Tunnel connected successfully (no error returned)")
				agentmetrics.SetTunnelConnected(true)
				agentmetrics.RecordTunnelReconnect("success")
				span.End()
			}
			log.Log.Info("Sleeping 5 seconds before retry")
			time.Sleep(5 * time.Second)
		}
	}
}
