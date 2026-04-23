package helm

import (
	"context"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HelmClient defines the interface for Helm operations, enabling mock in tests
type HelmClient interface {
	// InstallOrUpgrade installs a new release or upgrades an existing one
	InstallOrUpgrade(releaseName string, chartPath string, values map[string]interface{}) (*release.Release, error)
	// Uninstall removes a release
	Uninstall(releaseName string) error
}

// Client implements HelmClient interface
type Client struct {
	settings *cli.EnvSettings
	cfg      *action.Configuration
	ns       string
}

// Ensure Client implements HelmClient
var _ HelmClient = (*Client)(nil)

func NewClient(namespace string) (*Client, error) {
	settings := cli.New()
	cfg := new(action.Configuration)

	// We use the in-cluster config or default kubeconfig
	if err := cfg.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		// Log callback
	}); err != nil {
		return nil, err
	}

	return &Client{
		settings: settings,
		cfg:      cfg,
		ns:       namespace,
	}, nil
}

// NewClientInCluster creates a helm client from a rest.Config
func NewClientInCluster(namespace string) (*Client, error) {
	return NewClient(namespace)
}

// EnsureNamespaceWithHelmLabels ensures the target namespace exists with Helm ownership labels.
// This prevents "cannot be imported into the current release: invalid ownership metadata" errors
// when a namespace was created outside of Helm (e.g., by a previous failed install or by the chart itself).
func EnsureNamespaceWithHelmLabels(ctx context.Context, k8sClient client.Client, namespace string) error {
	if k8sClient == nil {
		return nil
	}
	ns := &corev1.Namespace{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
	if err != nil {
		if errors.IsNotFound(err) {
			// Namespace does not exist; Helm CreateNamespace will handle it.
			return nil
		}
		return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}

	// Namespace exists; ensure it has Helm ownership labels.
	needsUpdate := false
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
		needsUpdate = true
	}
	if ns.Labels["app.kubernetes.io/managed-by"] != "Helm" {
		ns.Labels["app.kubernetes.io/managed-by"] = "Helm"
		needsUpdate = true
	}
	if ns.Annotations == nil {
		ns.Annotations = make(map[string]string)
		needsUpdate = true
	}
	// Use a generic release name annotation to satisfy Helm's ownership check.
	if ns.Annotations["meta.helm.sh/release-name"] == "" {
		ns.Annotations["meta.helm.sh/release-name"] = "kumquat-addon"
		needsUpdate = true
	}
	if ns.Annotations["meta.helm.sh/release-namespace"] == "" {
		ns.Annotations["meta.helm.sh/release-namespace"] = namespace
		needsUpdate = true
	}

	if needsUpdate {
		if err := k8sClient.Update(ctx, ns); err != nil {
			return fmt.Errorf("failed to update namespace %s with Helm labels: %w", namespace, err)
		}
	}
	return nil
}

func (c *Client) InstallOrUpgrade(releaseName string, chartPath string, values map[string]interface{}) (*release.Release, error) {
	resolvedChartPath, err := c.resolveChartPath(chartPath)
	if err != nil {
		return nil, err
	}

	histClient := action.NewHistory(c.cfg)
	histClient.Max = 1
	if _, err := histClient.Run(releaseName); err == nil {
		// Upgrade
		client := action.NewUpgrade(c.cfg)
		client.Namespace = c.ns
		client.ReuseValues = false
		client.Wait = true
		client.Timeout = 5 * time.Minute

		ch, err := loader.Load(resolvedChartPath)
		if err != nil {
			return nil, err
		}

		return client.Run(releaseName, ch, values)
	}

	// Install
	client := action.NewInstall(c.cfg)
	client.ReleaseName = releaseName
	client.Namespace = c.ns
	client.CreateNamespace = true
	client.Wait = true
	client.Timeout = 5 * time.Minute

	ch, err := loader.Load(resolvedChartPath)
	if err != nil {
		return nil, err
	}

	return client.Run(ch, values)
}

func (c *Client) resolveChartPath(chartPath string) (string, error) {
	chartPathOptions := action.ChartPathOptions{}
	return chartPathOptions.LocateChart(chartPath, c.settings)
}

func (c *Client) Uninstall(releaseName string) error {
	client := action.NewUninstall(c.cfg)
	_, err := client.Run(releaseName)
	return err
}
