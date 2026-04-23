package cluster

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	"github.com/fize/kumquat/engine/pkg/constants"
	"github.com/fize/kumquat/engine/pkg/observability"
	"github.com/fize/kumquat/engine/pkg/scheme"
	agentmetrics "github.com/fize/kumquat/engine/internal/agent/metrics"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AgentOptions holds the configuration for the Agent
type AgentOptions struct {
	HubURL            string
	TunnelURL         string
	ClusterName       string
	BootstrapToken    string
	HeartbeatInterval time.Duration
}

// Agent is the edge agent that connects to the Hub
type Agent struct {
	Options     AgentOptions
	HubClient   client.Client
	HubConfig   *rest.Config
	LocalClient client.Client
}

// NewAgent creates a new Agent with the given options
func NewAgent(opts AgentOptions) *Agent {
	if opts.TunnelURL == "" {
		opts.TunnelURL = opts.HubURL
	}
	return &Agent{
		Options: opts,
	}
}

// InitHubClient initializes the client to talk to the Hub
func (a *Agent) InitHubClient() error {
	var config *rest.Config
	var err error

	if a.Options.HubURL != "" && a.Options.BootstrapToken != "" {
		config = &rest.Config{
			Host:        a.Options.HubURL,
			BearerToken: a.Options.BootstrapToken,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
	} else {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to load kubeconfig: %w", err)
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

func (a *Agent) getClusterCredentials() (map[string]string, error) {
	creds := make(map[string]string)

	// Read CA
	caData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err == nil {
		creds[constants.AnnotationCredentialsCA] = base64.StdEncoding.EncodeToString(caData)
	}

	// Read Token
	tokenData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
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

func (a *Agent) Register(ctx context.Context) error {
	if a.HubClient == nil {
		return fmt.Errorf("hub client not initialized")
	}

	creds, _ := a.getClusterCredentials()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster := &clusterv1alpha1.ManagedCluster{}
		err := a.HubClient.Get(ctx, client.ObjectKey{Name: a.Options.ClusterName}, cluster)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to get cluster: %w", err)
			}

			newCluster := &clusterv1alpha1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.Options.ClusterName,
					Annotations: creds,
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

		log.Log.Info("Cluster already exists, updating credentials", "cluster", a.Options.ClusterName)
		if cluster.Annotations == nil {
			cluster.Annotations = make(map[string]string)
		}
		for k, v := range creds {
			cluster.Annotations[k] = v
		}
		if err := a.HubClient.Update(ctx, cluster); err != nil {
			return err
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
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+a.Options.BootstrapToken)
	headers.Set("X-Kumquat-Cluster-Name", a.Options.ClusterName)
	headers.Set("X-Remotedialer-ID", a.Options.ClusterName)

	url := fmt.Sprintf("%s/connect", a.Options.TunnelURL)
	if strings.HasPrefix(url, "https") {
		url = strings.Replace(url, "https", "wss", 1)
	} else if strings.HasPrefix(url, "http") {
		url = strings.Replace(url, "http", "ws", 1)
	}

	log.Log.Info("Starting tunnel", "url", url)

	agentmetrics.SetTunnelConnected(false)

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	for {
		select {
		case <-ctx.Done():
			agentmetrics.SetTunnelConnected(false)
			return nil
		default:
			_, span := observability.Tracer().Start(ctx, "Agent.TunnelConnect",
				trace.WithAttributes(
					attribute.String("cluster.name", a.Options.ClusterName),
					attribute.String("tunnel.url", url),
				),
			)

			log.Log.Info("Starting connection attempt", "url", url)
			err := remotedialer.ClientConnect(ctx, url, headers, dialer, func(proto, address string) bool {
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
