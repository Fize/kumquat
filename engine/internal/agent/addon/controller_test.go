package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/fize/kumquat/engine/internal/addon"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestUpdateAddonStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []storagev1alpha1.AddonStatus
		addon    string
		state    string
		message  string
		wantLen  int
		wantIdx  int
	}{
		{
			name:    "empty statuses - add new",
			statuses: []storagev1alpha1.AddonStatus{},
			addon:   "test-addon",
			state:   "Applied",
			message: "success",
			wantLen: 1,
			wantIdx: 0,
		},
		{
			name: "existing status - update",
			statuses: []storagev1alpha1.AddonStatus{
				{Name: "other-addon", State: "Pending", Message: "waiting"},
				{Name: "test-addon", State: "Pending", Message: "waiting"},
			},
			addon:   "test-addon",
			state:   "Applied",
			message: "success",
			wantLen: 2,
			wantIdx: 1,
		},
		{
			name: "multiple statuses - update correct one",
			statuses: []storagev1alpha1.AddonStatus{
				{Name: "addon-a", State: "Applied", Message: ""},
				{Name: "addon-b", State: "Pending", Message: ""},
				{Name: "addon-c", State: "Applied", Message: ""},
			},
			addon:   "addon-b",
			state:   "Failed",
			message: "error occurred",
			wantLen: 3,
			wantIdx: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateAddonStatus(tt.statuses, tt.addon, tt.state, tt.message)

			if len(got) != tt.wantLen {
				t.Errorf("updateAddonStatus() len = %d, want %d", len(got), tt.wantLen)
			}

			if got[tt.wantIdx].Name != tt.addon {
				t.Errorf("updateAddonStatus()[%d].Name = %s, want %s", tt.wantIdx, got[tt.wantIdx].Name, tt.addon)
			}
			if got[tt.wantIdx].State != tt.state {
				t.Errorf("updateAddonStatus()[%d].State = %s, want %s", tt.wantIdx, got[tt.wantIdx].State, tt.state)
			}
			if got[tt.wantIdx].Message != tt.message {
				t.Errorf("updateAddonStatus()[%d].Message = %s, want %s", tt.wantIdx, got[tt.wantIdx].Message, tt.message)
			}
		})
	}
}

// mockAddonController is a mock implementation of addon.AddonController
type mockAddonController struct {
	reconcileErr error
}

func (m *mockAddonController) Reconcile(ctx context.Context, config addon.AddonConfig) error {
	return m.reconcileErr
}

// mockAddon is a mock implementation of addon.Addon for testing
type mockAddon struct {
	name       string
	controller addon.AddonController
}

func (m *mockAddon) Name() string {
	return m.name
}

func (m *mockAddon) ManagerController(mgr ctrl.Manager) (addon.AddonController, error) {
	return nil, nil
}

func (m *mockAddon) AgentController(mgr ctrl.Manager) (addon.AddonController, error) {
	return m.controller, nil
}

func (m *mockAddon) Manifests() []runtime.Object {
	return nil
}

// mockAddonRegistry is a mock implementation of addon.AddonRegistry
type mockAddonRegistry struct {
	addons []addon.Addon
}

func (m *mockAddonRegistry) Register(c addon.Addon) {
	m.addons = append(m.addons, c)
}

func (m *mockAddonRegistry) List() []addon.Addon {
	return m.addons
}

func (m *mockAddonRegistry) Get(name string) addon.Addon {
	for _, a := range m.addons {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

func setupTestReconciler(t *testing.T, cluster *storagev1alpha1.ManagedCluster, addonName string, reconcileErr error) (*AddonReconciler, client.Client) {
	t.Helper()

	scheme := runtime.NewScheme()
	storagev1alpha1.AddToScheme(scheme)

	objects := []client.Object{cluster}
	fakeHubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(&storagev1alpha1.ManagedCluster{}).
		Build()

	mockCtrl := &mockAddonController{
		reconcileErr: reconcileErr,
	}

	mockAddon := &mockAddon{
		name:       addonName,
		controller: mockCtrl,
	}

	registry := &mockAddonRegistry{
		addons: []addon.Addon{mockAddon},
	}

	r := &AddonReconciler{
		HubClient:   fakeHubClient,
		LocalClient: fakeHubClient,
		ClusterName: cluster.Name,
		Registry:    registry,
		Controllers: map[string]addon.AddonController{
			addonName: mockCtrl,
		},
	}

	return r, fakeHubClient
}

func TestAddonReconciler_Reconcile_ErrPrerequisitesNotMet(t *testing.T) {
	cluster := &storagev1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: storagev1alpha1.ManagedClusterSpec{
			Addons: []storagev1alpha1.ClusterAddon{
				{Name: "test-addon", Enabled: true},
			},
		},
		Status: storagev1alpha1.ManagedClusterStatus{},
	}

	r, fakeClient := setupTestReconciler(t, cluster, "test-addon", addon.ErrPrerequisitesNotMet)

	// Verify r.HubClient can get the cluster before calling Reconcile
	verifyCluster := &storagev1alpha1.ManagedCluster{}
	if err := r.HubClient.Get(context.Background(), client.ObjectKey{Name: "test-cluster"}, verifyCluster); err != nil {
		t.Fatalf("r.HubClient.Get() failed before Reconcile: %v", err)
	}
	t.Logf("r.HubClient.Get() succeeded before Reconcile, cluster name: %s", verifyCluster.Name)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "test-cluster"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify status was updated to Pending
	updated := &storagev1alpha1.ManagedCluster{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-cluster"}, updated); err != nil {
		t.Fatalf("Failed to get updated cluster: %v", err)
	}

	found := false
	for _, s := range updated.Status.AddonStatus {
		if s.Name == "test-addon" && s.State == "Pending" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Addon status not updated to Pending for ErrPrerequisitesNotMet. Status: %+v", updated.Status.AddonStatus)
	}
}

func TestAddonReconciler_Reconcile_RealError(t *testing.T) {
	cluster := &storagev1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: storagev1alpha1.ManagedClusterSpec{
			Addons: []storagev1alpha1.ClusterAddon{
				{Name: "test-addon", Enabled: true},
			},
		},
		Status: storagev1alpha1.ManagedClusterStatus{},
	}

	r, fakeClient := setupTestReconciler(t, cluster, "test-addon", errors.New("some real error"))

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "test-cluster"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	updated := &storagev1alpha1.ManagedCluster{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-cluster"}, updated); err != nil {
		t.Fatalf("Failed to get updated cluster: %v", err)
	}

	found := false
	for _, s := range updated.Status.AddonStatus {
		if s.Name == "test-addon" && s.State == "Failed" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Addon status not updated to Failed for real error. Status: %+v", updated.Status.AddonStatus)
	}
}

func TestAddonReconciler_Reconcile_Success(t *testing.T) {
	cluster := &storagev1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: storagev1alpha1.ManagedClusterSpec{
			Addons: []storagev1alpha1.ClusterAddon{
				{Name: "test-addon", Enabled: true},
			},
		},
		Status: storagev1alpha1.ManagedClusterStatus{},
	}

	r, fakeClient := setupTestReconciler(t, cluster, "test-addon", nil)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "test-cluster"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	updated := &storagev1alpha1.ManagedCluster{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-cluster"}, updated); err != nil {
		t.Fatalf("Failed to get updated cluster: %v", err)
	}

	found := false
	for _, s := range updated.Status.AddonStatus {
		if s.Name == "test-addon" && s.State == "Applied" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Addon status not updated to Applied for success. Status: %+v", updated.Status.AddonStatus)
	}
}

func TestAddonReconciler_Reconcile_Disabled(t *testing.T) {
	cluster := &storagev1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: storagev1alpha1.ManagedClusterSpec{
			Addons: []storagev1alpha1.ClusterAddon{
				{Name: "test-addon", Enabled: false},
			},
		},
		Status: storagev1alpha1.ManagedClusterStatus{},
	}

	// For disabled addon, we don't need a controller
	scheme := runtime.NewScheme()
	storagev1alpha1.AddToScheme(scheme)

	fakeHubClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(&storagev1alpha1.ManagedCluster{}).
		Build()

	// Create registry with addon but it won't be used since addon is disabled
	mockCtrl := &mockAddonController{
		reconcileErr: nil,
	}

	mockAddon := &mockAddon{
		name:       "test-addon",
		controller: mockCtrl,
	}

	registry := &mockAddonRegistry{
		addons: []addon.Addon{mockAddon},
	}

	r := &AddonReconciler{
		HubClient:   fakeHubClient,
		LocalClient: fakeHubClient,
		ClusterName: "test-cluster",
		Registry:    registry,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "test-cluster"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	updated := &storagev1alpha1.ManagedCluster{}
	if err := fakeHubClient.Get(context.Background(), client.ObjectKey{Name: "test-cluster"}, updated); err != nil {
		t.Fatalf("Failed to get updated cluster: %v", err)
	}

	found := false
	for _, s := range updated.Status.AddonStatus {
		if s.Name == "test-addon" && s.State == "Disabled" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Addon status not updated to Disabled when addon is disabled. Status: %+v", updated.Status.AddonStatus)
	}
}
