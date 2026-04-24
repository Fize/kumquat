package helm

import (
	"testing"

	"helm.sh/helm/v3/pkg/release"
)

// mockHelmClient is a mock implementation of HelmClient for testing.
type mockHelmClient struct {
	installOrUpgradeErr error
	uninstallErr        error
	lastReleaseName     string
	lastChartPath      string
	lastValues         map[string]interface{}
}

// InstallOrUpgrade mocks the Helm install or upgrade operation.
func (m *mockHelmClient) InstallOrUpgrade(releaseName string, chartPath string, values map[string]interface{}) (*release.Release, error) {
	m.lastReleaseName = releaseName
	m.lastChartPath = chartPath
	m.lastValues = values
	return &release.Release{
		Name:      releaseName,
		Namespace: "default",
	}, m.installOrUpgradeErr
}

// Uninstall mocks the Helm uninstall operation.
func (m *mockHelmClient) Uninstall(releaseName string) error {
	m.lastReleaseName = releaseName
	return m.uninstallErr
}

// TestHelmClientInterface verifies that mockHelmClient implements HelmClient.
func TestHelmClientInterface(t *testing.T) {
	var _ HelmClient = &mockHelmClient{}
	t.Log("mockHelmClient correctly implements HelmClient interface")
}

// TestMockHelmClient verifies that our mock works as expected.
func TestMockHelmClient(t *testing.T) {
	mock := &mockHelmClient{
		installOrUpgradeErr: nil,
		uninstallErr:        nil,
	}

	// Test InstallOrUpgrade
	rel, err := mock.InstallOrUpgrade("test-release", "test-chart", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if rel == nil {
		t.Errorf("Expected non-nil release")
	}
	if mock.lastReleaseName != "test-release" {
		t.Errorf("Expected lastReleaseName=test-release, got %s", mock.lastReleaseName)
	}

	// Test Uninstall
	err = mock.Uninstall("test-release")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if mock.lastReleaseName != "test-release" {
		t.Errorf("Expected lastReleaseName=test-release, got %s", mock.lastReleaseName)
	}
}

// Note: More comprehensive tests for InstallOrUpgrade would require:
// 1. Mocking Helm's action.Configuration
// 2. Creating test chart files
// 3. Setting up a test Kubernetes environment
// These are better suited for integration tests rather than unit tests.
