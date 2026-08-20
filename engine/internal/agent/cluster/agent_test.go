package cluster

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fize/kumquat/engine/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
)

func useProjectedCredentials(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(tokenPath, []byte("projected-token"), 0o600))
	require.NoError(t, os.WriteFile(caPath, []byte("projected-ca"), 0o600))
	originalTokenPath, originalCAPath := serviceAccountTokenPath, serviceAccountCAPath
	serviceAccountTokenPath, serviceAccountCAPath = tokenPath, caPath
	t.Cleanup(func() { serviceAccountTokenPath, serviceAccountCAPath = originalTokenPath, originalCAPath })
}

func testCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestAgentLocalReporterRequiresExplicitLocalMarker(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "ambiguous-hub"},
		Spec:       clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeHub},
	}
	agent := &Agent{Options: AgentOptions{ClusterName: cluster.Name}, HubClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()}
	require.ErrorContains(t, agent.Register(context.Background()), "explicit local")
}

func TestAgentHeartbeatPublishesRotatedProjectedCredentials(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(tokenPath, []byte("token-b"), 0o600))
	require.NoError(t, os.WriteFile(caPath, []byte("ca-b"), 0o600))
	originalTokenPath, originalCAPath := serviceAccountTokenPath, serviceAccountCAPath
	serviceAccountTokenPath, serviceAccountCAPath = tokenPath, caPath
	t.Cleanup(func() { serviceAccountTokenPath, serviceAccountCAPath = originalTokenPath, originalCAPath })
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "edge", Annotations: map[string]string{constants.AnnotationCredentialsHash: credentialsHash(map[string]string{constants.AnnotationCredentialsToken: "token-a"})}},
		Spec:       clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge},
	}
	hubClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	agent := &Agent{Options: AgentOptions{ClusterName: cluster.Name, HubURL: "https://hub", BootstrapToken: "bootstrap"}, HubClient: hubClient}
	require.NoError(t, agent.sendHeartbeat(context.Background()))
	var got clusterv1alpha1.ManagedCluster
	require.NoError(t, hubClient.Get(context.Background(), client.ObjectKey{Name: cluster.Name}, &got))
	assert.Equal(t, "token-b", got.Annotations[constants.AnnotationCredentialsToken])
	assert.NotEqual(t, cluster.Annotations[constants.AnnotationCredentialsHash], got.Annotations[constants.AnnotationCredentialsHash])
}

func TestAgentHeartbeatDoesNotPublishIncompleteProjectedCredentials(t *testing.T) {
	dir := t.TempDir()
	originalTokenPath, originalCAPath := serviceAccountTokenPath, serviceAccountCAPath
	serviceAccountTokenPath = filepath.Join(dir, "missing-token")
	serviceAccountCAPath = filepath.Join(dir, "missing-ca")
	t.Cleanup(func() { serviceAccountTokenPath, serviceAccountCAPath = originalTokenPath, originalCAPath })

	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "edge", Annotations: map[string]string{
			constants.AnnotationCredentialsHash:  "sha256:old",
			constants.AnnotationCredentialsToken: "token-a",
			constants.AnnotationCredentialsCA:    "ca-a",
		}},
		Spec: clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge},
	}
	hubClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	agent := &Agent{Options: AgentOptions{ClusterName: cluster.Name, HubURL: "https://hub"}, HubClient: hubClient}
	require.ErrorContains(t, agent.sendHeartbeat(context.Background()), "read projected service account")

	var got clusterv1alpha1.ManagedCluster
	require.NoError(t, hubClient.Get(context.Background(), client.ObjectKey{Name: cluster.Name}, &got))
	assert.Equal(t, "token-a", got.Annotations[constants.AnnotationCredentialsToken])
	assert.Equal(t, "ca-a", got.Annotations[constants.AnnotationCredentialsCA])
	assert.Equal(t, "sha256:old", got.Annotations[constants.AnnotationCredentialsHash])
}

func TestNewAgent(t *testing.T) {
	opts := AgentOptions{
		HubURL:            "http://hub.kumquat.io",
		ClusterName:       "test-cluster",
		BootstrapToken:    "test-token",
		HeartbeatInterval: 1 * time.Minute,
	}

	agent := NewAgent(opts)

	if agent.Options.HubURL != opts.HubURL {
		t.Errorf("expected HubURL %s, got %s", opts.HubURL, agent.Options.HubURL)
	}
	if agent.Options.TunnelURL != "" {
		t.Errorf("expected tunnel to remain disabled, got %s", agent.Options.TunnelURL)
	}
	if agent.Options.ClusterName != opts.ClusterName {
		t.Errorf("expected ClusterName %s, got %s", opts.ClusterName, agent.Options.ClusterName)
	}
}

func TestInitHubClientUsesRotatingTokenFileAndVerifiedCA(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	caPath := filepath.Join(dir, "hub-ca.crt")
	require.NoError(t, os.WriteFile(tokenPath, []byte("abcdef.1234567890abcdef"), 0o600))
	require.NoError(t, os.WriteFile(caPath, testCAPEM(t), 0o600))
	agent := NewAgent(AgentOptions{HubURL: "https://hub.example.test", ClusterName: "edge-a", BootstrapTokenFile: tokenPath, HubCAFile: caPath})
	require.NoError(t, agent.InitHubClient())
	assert.Equal(t, tokenPath, agent.HubConfig.BearerTokenFile)
	assert.Equal(t, caPath, agent.HubConfig.CAFile)
	assert.Empty(t, agent.HubConfig.BearerToken)
	assert.False(t, agent.HubConfig.Insecure)
}

func TestTunnelTLSConfigUsesCAAndSNI(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "tunnel-ca.crt")
	require.NoError(t, os.WriteFile(caPath, testCAPEM(t), 0o600))
	agent := NewAgent(AgentOptions{TunnelURL: "https://tunnel.example.test:5443", TunnelCAFile: caPath})
	config, err := agent.tunnelTLSConfig()
	require.NoError(t, err)
	assert.False(t, config.InsecureSkipVerify)
	assert.Equal(t, "tunnel.example.test", config.ServerName)
	assert.NotNil(t, config.RootCAs)
}

func TestTunnelHeadersReloadRotatedTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("token-a\n"), 0o600))
	agent := NewAgent(AgentOptions{ClusterName: "edge-a", BootstrapTokenFile: tokenPath})
	headers, err := agent.tunnelHeaders()
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-a", headers.Get("Authorization"))
	require.NoError(t, os.WriteFile(tokenPath, []byte("token-b\n"), 0o600))
	headers, err = agent.tunnelHeaders()
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-b", headers.Get("Authorization"))
}

func TestAgent_Register(t *testing.T) {
	useProjectedCredentials(t)
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1alpha1.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	clusterName := "test-cluster"
	hubClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName:    clusterName,
			HubURL:         "https://hub.example.test",
			BootstrapToken: "test-token",
		},
		HubClient: hubClient,
	}

	// 1. Register new cluster
	err := agent.Register(context.Background())
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	cluster := &clusterv1alpha1.ManagedCluster{}
	err = hubClient.Get(context.Background(), client.ObjectKey{Name: clusterName}, cluster)
	if err != nil {
		t.Fatalf("failed to get cluster after registration: %v", err)
	}

	if cluster.Spec.ConnectionMode != clusterv1alpha1.ClusterConnectionModeEdge {
		t.Errorf("expected connection mode %s, got %s", clusterv1alpha1.ClusterConnectionModeEdge, cluster.Spec.ConnectionMode)
	}
	if cluster.Labels[constants.LabelRegistrationSource] != constants.RegistrationSourceAgent {
		t.Fatalf("registration source label = %q", cluster.Labels[constants.LabelRegistrationSource])
	}

	// 2. Register existing cluster (update)
	err = agent.Register(context.Background())
	if err != nil {
		t.Fatalf("Register (update) failed: %v", err)
	}
}

func TestAgentRegisterIncompleteProjectedCredentialsDoesNotCreate(t *testing.T) {
	dir := t.TempDir()
	originalTokenPath, originalCAPath := serviceAccountTokenPath, serviceAccountCAPath
	serviceAccountTokenPath = filepath.Join(dir, "missing-token")
	serviceAccountCAPath = filepath.Join(dir, "missing-ca")
	t.Cleanup(func() { serviceAccountTokenPath, serviceAccountCAPath = originalTokenPath, originalCAPath })
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	hubClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	agent := &Agent{Options: AgentOptions{ClusterName: "edge-incomplete", HubURL: "https://hub"}, HubClient: hubClient}
	require.ErrorContains(t, agent.Register(context.Background()), "read projected service account")
	var got clusterv1alpha1.ManagedCluster
	require.Error(t, hubClient.Get(context.Background(), client.ObjectKey{Name: "edge-incomplete"}, &got))
}

func TestAgent_SendHeartbeat(t *testing.T) {
	useProjectedCredentials(t)
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1alpha1.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	clusterName := "test-cluster"
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}

	hubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName:    clusterName,
			HubURL:         "https://hub.example.test",
			BootstrapToken: "test-token",
		},
		HubClient: hubClient,
	}

	err := agent.sendHeartbeat(context.Background())
	if err != nil {
		t.Fatalf("sendHeartbeat failed: %v", err)
	}

	updatedCluster := &clusterv1alpha1.ManagedCluster{}
	err = hubClient.Get(context.Background(), client.ObjectKey{Name: clusterName}, updatedCluster)
	if err != nil {
		t.Fatalf("failed to get cluster after heartbeat: %v", err)
	}

	if updatedCluster.Status.LastKeepAliveTime == nil {
		t.Error("expected LastKeepAliveTime to be set")
	}
}

func TestNewAgent_WithTunnelURL(t *testing.T) {
	opts := AgentOptions{
		HubURL:            "http://hub.kumquat.io",
		TunnelURL:         "http://tunnel.kumquat.io",
		ClusterName:       "test-cluster",
		BootstrapToken:    "test-token",
		HeartbeatInterval: 1 * time.Minute,
	}

	agent := NewAgent(opts)

	assert.Equal(t, "http://tunnel.kumquat.io", agent.Options.TunnelURL)
	assert.Equal(t, "http://hub.kumquat.io", agent.Options.HubURL)
}

func TestAgent_Register_NoHubClient(t *testing.T) {
	agent := &Agent{
		Options: AgentOptions{
			ClusterName: "test-cluster",
		},
		HubClient: nil,
	}

	err := agent.Register(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hub client not initialized")
}

func TestAgent_getClusterCredentials(t *testing.T) {
	agent := &Agent{
		Options: AgentOptions{
			ClusterName: "test-cluster",
		},
	}

	creds, err := agent.getClusterCredentials()
	assert.NoError(t, err)
	assert.NotNil(t, creds)

	// Should have at least the APIServer URL (defaults to kubernetes.default.svc if not in cluster)
	assert.Contains(t, creds, constants.AnnotationAPIServerURL)
}

func TestAgent_sendHeartbeat_ClusterNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))

	hubClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName: "non-existent-cluster",
		},
		HubClient: hubClient,
	}

	err := agent.sendHeartbeat(context.Background())
	assert.Error(t, err)
}

func TestAgentOptions_Fields(t *testing.T) {
	opts := AgentOptions{
		HubURL:            "https://hub.example.com",
		TunnelURL:         "https://tunnel.example.com",
		ClusterName:       "my-cluster",
		BootstrapToken:    "my-token",
		HeartbeatInterval: 30 * time.Second,
	}

	assert.Equal(t, "https://hub.example.com", opts.HubURL)
	assert.Equal(t, "https://tunnel.example.com", opts.TunnelURL)
	assert.Equal(t, "my-cluster", opts.ClusterName)
	assert.Equal(t, "my-token", opts.BootstrapToken)
	assert.Equal(t, 30*time.Second, opts.HeartbeatInterval)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "cluster.kumquat.io/credentials-ca", constants.AnnotationCredentialsCA)
	assert.Equal(t, "cluster.kumquat.io/credentials-token", constants.AnnotationCredentialsToken)
	assert.Equal(t, "cluster.kumquat.io/apiserver-url", constants.AnnotationAPIServerURL)
}

func TestAgent_InitHubClient_WithHubURLAndToken(t *testing.T) {
	agent := &Agent{
		Options: AgentOptions{
			HubURL:         "https://hub.example.com",
			BootstrapToken: "test-token",
			ClusterName:    "test-cluster",
		},
	}

	// This will fail because the Hub URL is not reachable
	// We just verify that the function can be called without panic
	_ = agent.InitHubClient()
	// The call may succeed or fail depending on network, we just verify it doesn't panic
}

func TestAgent_InitHubClient_EmptyHubURLAndToken(t *testing.T) {
	original := inClusterConfig
	t.Cleanup(func() { inClusterConfig = original })
	inClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
	}
	agent := &Agent{
		Options: AgentOptions{
			HubURL:         "",
			BootstrapToken: "",
			ClusterName:    "test-cluster",
		},
	}

	require.NoError(t, agent.InitHubClient())
	assert.Equal(t, "https://kubernetes.default.svc", agent.HubConfig.Host)
}

func TestAgent_InitHubClient_OnlyHubURL(t *testing.T) {
	agent := &Agent{
		Options: AgentOptions{
			HubURL:         "https://hub.example.com",
			BootstrapToken: "", // Empty token
			ClusterName:    "test-cluster",
		},
	}

	// When only HubURL is set (no token), it should try kubeconfig path
	err := agent.InitHubClient()
	_ = err // May fail, just verify no panic
}

func TestAgent_InitHubClient_OnlyToken(t *testing.T) {
	agent := &Agent{
		Options: AgentOptions{
			HubURL:         "", // Empty URL
			BootstrapToken: "some-token",
			ClusterName:    "test-cluster",
		},
	}

	// When only token is set (no HubURL), it should try kubeconfig path
	err := agent.InitHubClient()
	_ = err // May fail, just verify no panic
}

func TestAgent_Register_ExistingCluster_Update(t *testing.T) {
	useProjectedCredentials(t)
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1alpha1.AddToScheme(scheme))

	clusterName := "existing-cluster"
	existingCluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
			Annotations: map[string]string{
				"existing-key": "existing-value",
			},
		},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			ConnectionMode: clusterv1alpha1.ClusterConnectionModeHub,
		},
	}

	hubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCluster).
		Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName:    clusterName,
			HubURL:         "https://hub.example.test",
			BootstrapToken: "test-token",
		},
		HubClient: hubClient,
	}

	err := agent.Register(context.Background())
	assert.NoError(t, err)

	// Verify cluster was updated with new credentials
	updatedCluster := &clusterv1alpha1.ManagedCluster{}
	err = hubClient.Get(context.Background(), client.ObjectKey{Name: clusterName}, updatedCluster)
	assert.NoError(t, err)

	// Should preserve existing annotation and add new ones
	assert.Equal(t, "existing-value", updatedCluster.Annotations["existing-key"])
	assert.Contains(t, updatedCluster.Annotations, constants.AnnotationAPIServerURL)
}

func TestAgent_sendHeartbeat_StatusUpdateFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1alpha1.AddToScheme(scheme))

	clusterName := "test-cluster"
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}

	// Create client without status subresource support to trigger fallback
	hubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName:    clusterName,
			HubURL:         "https://hub.example.test",
			BootstrapToken: "test-token",
		},
		HubClient: hubClient,
	}

	err := agent.sendHeartbeat(context.Background())
	// May fail on status update but try regular update as fallback
	// Just verify no panic
	_ = err
}

func TestAgent_getClusterCredentials_EnvironmentVariables(t *testing.T) {
	// Set environment variables
	oldHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	oldPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	defer func() {
		os.Setenv("KUBERNETES_SERVICE_HOST", oldHost)
		os.Setenv("KUBERNETES_SERVICE_PORT", oldPort)
	}()

	os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")

	agent := &Agent{
		Options: AgentOptions{
			ClusterName: "test-cluster",
		},
	}

	creds, err := agent.getClusterCredentials()
	assert.NoError(t, err)
	assert.Equal(t, "https://10.0.0.1:443", creds[constants.AnnotationAPIServerURL])
}

func TestAgent_getClusterCredentials_DefaultAPIServer(t *testing.T) {
	// Unset environment variables
	oldHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	oldPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	defer func() {
		if oldHost != "" {
			os.Setenv("KUBERNETES_SERVICE_HOST", oldHost)
		}
		if oldPort != "" {
			os.Setenv("KUBERNETES_SERVICE_PORT", oldPort)
		}
	}()

	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_PORT")

	agent := &Agent{
		Options: AgentOptions{
			ClusterName: "test-cluster",
		},
	}

	creds, err := agent.getClusterCredentials()
	assert.NoError(t, err)
	assert.Equal(t, "https://kubernetes.default.svc:443", creds[constants.AnnotationAPIServerURL])
}

func TestNewAgent_DoesNotEnableTunnelImplicitly(t *testing.T) {
	opts := AgentOptions{
		HubURL:      "https://hub.example.com",
		TunnelURL:   "",
		ClusterName: "test-cluster",
	}

	agent := NewAgent(opts)
	assert.Empty(t, agent.Options.TunnelURL)
}

func TestNewAgent_CustomTunnelURL(t *testing.T) {
	opts := AgentOptions{
		HubURL:      "https://hub.example.com",
		TunnelURL:   "https://tunnel.example.com",
		ClusterName: "test-cluster",
	}

	agent := NewAgent(opts)
	assert.Equal(t, "https://tunnel.example.com", agent.Options.TunnelURL)
}

func TestAgent_Register_NilAnnotations(t *testing.T) {
	useProjectedCredentials(t)
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, clusterv1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1alpha1.AddToScheme(scheme))

	clusterName := "test-cluster-nil-annotations"
	existingCluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        clusterName,
			Annotations: nil, // Nil annotations
		},
	}

	hubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCluster).
		Build()

	agent := &Agent{
		Options: AgentOptions{
			ClusterName:    clusterName,
			HubURL:         "https://hub.example.test",
			BootstrapToken: "test-token",
		},
		HubClient: hubClient,
	}

	err := agent.Register(context.Background())
	assert.NoError(t, err)

	// Verify annotations were created
	updatedCluster := &clusterv1alpha1.ManagedCluster{}
	err = hubClient.Get(context.Background(), client.ObjectKey{Name: clusterName}, updatedCluster)
	assert.NoError(t, err)
	assert.NotNil(t, updatedCluster.Annotations)
}
