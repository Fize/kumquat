package client

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fize/kumquat/engine/pkg/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sClient wraps the controller-runtime client
type K8sClient struct {
	client client.Client
	config *rest.Config
}

// Config holds configuration for Kubernetes client
type Config struct {
	KubeconfigPath string
	MasterURL      string
}

// NewK8sClient creates a new Kubernetes client
// Priority: 1. In-cluster config 2. Kubeconfig file 3. Default kubeconfig location
func NewK8sClient(cfg *Config) (*K8sClient, error) {
	restConfig, err := buildConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	c, err := client.New(restConfig, client.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &K8sClient{
		client: c,
		config: restConfig,
	}, nil
}

// GetClient returns the underlying controller-runtime client
func (k *K8sClient) GetClient() client.Client {
	return k.client
}

// GetConfig returns the REST config
func (k *K8sClient) GetConfig() *rest.Config {
	return k.config
}

// buildConfig builds the REST config from various sources
func buildConfig(cfg *Config) (*rest.Config, error) {
	// 1. Try in-cluster config first
	if restConfig, err := rest.InClusterConfig(); err == nil {
		return restConfig, nil
	}

	// 2. Use provided kubeconfig path
	kubeconfigPath := cfg.KubeconfigPath
	if kubeconfigPath == "" {
		// 3. Try environment variable
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		// 4. Try default location
		home := homedir.HomeDir()
		if home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	if kubeconfigPath == "" {
		return nil, fmt.Errorf("unable to find kubeconfig")
	}

	// Build from kubeconfig
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{ClusterInfo: api.Cluster{Server: cfg.MasterURL}},
	).ClientConfig()
	if err != nil {
		return nil, err
	}

	return config, nil
}
