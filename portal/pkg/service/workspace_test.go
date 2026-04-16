package service

import (
	"context"
	"testing"

	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupWorkspaceScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	return scheme
}

func newWorkspaceFakeClient(objects ...client.Object) *fake.ClientBuilder {
	scheme := setupWorkspaceScheme()
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...)
}

func TestWorkspaceService_List_Empty(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	workspaces, err := svc.List(context.Background(), &ListWorkspacesRequest{})
	require.NoError(t, err)
	assert.Empty(t, workspaces.Items)
}

func TestWorkspaceService_List_WithWorkspaces(t *testing.T) {
	workspaces := []client.Object{
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		},
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
		},
	}

	c := newWorkspaceFakeClient(workspaces...).Build()
	svc := NewWorkspaceService(c)

	allWorkspaces, err := svc.List(context.Background(), &ListWorkspacesRequest{})
	require.NoError(t, err)
	assert.Len(t, allWorkspaces.Items, 2)
}

func TestWorkspaceService_List_FilterByCluster(t *testing.T) {
	workspaces := []client.Object{
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-cluster-a"},
			Status: workspacev1alpha1.WorkspaceStatus{
				AppliedClusters: []string{"cluster-a", "cluster-b"},
			},
		},
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-cluster-b-only"},
			Status: workspacev1alpha1.WorkspaceStatus{
				AppliedClusters: []string{"cluster-b"},
			},
		},
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-no-clusters"},
			Status: workspacev1alpha1.WorkspaceStatus{
				AppliedClusters: []string{},
			},
		},
	}

	c := newWorkspaceFakeClient(workspaces...).Build()
	svc := NewWorkspaceService(c)

	// Filter by cluster-a
	clusterAWorkspaces, err := svc.List(context.Background(), &ListWorkspacesRequest{Cluster: "cluster-a"})
	require.NoError(t, err)
	assert.Len(t, clusterAWorkspaces.Items, 1)
	assert.Equal(t, "ws-cluster-a", clusterAWorkspaces.Items[0].Name)

	// Filter by cluster-b
	clusterBWorkspaces, err := svc.List(context.Background(), &ListWorkspacesRequest{Cluster: "cluster-b"})
	require.NoError(t, err)
	assert.Len(t, clusterBWorkspaces.Items, 2) // both ws-cluster-a and ws-cluster-b-only

	// Filter by non-existent cluster
	emptyWorkspaces, err := svc.List(context.Background(), &ListWorkspacesRequest{Cluster: "cluster-z"})
	require.NoError(t, err)
	assert.Len(t, emptyWorkspaces.Items, 0)
}

func TestWorkspaceService_Get_Success(t *testing.T) {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ws"},
		Status: workspacev1alpha1.WorkspaceStatus{
			AppliedClusters: []string{"cluster-1"},
		},
	}

	c := newWorkspaceFakeClient(ws).Build()
	svc := NewWorkspaceService(c)

	result, err := svc.Get(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.Equal(t, "test-ws", result.Name)
}

func TestWorkspaceService_Get_NotFound(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestWorkspaceService_Create_Success(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "new-ws"},
	}

	err := svc.Create(context.Background(), &CreateWorkspaceRequest{Workspace: ws})
	require.NoError(t, err)

	// Verify created
	result, err := svc.Get(context.Background(), "new-ws")
	require.NoError(t, err)
	assert.Equal(t, "new-ws", result.Name)
}

func TestWorkspaceService_Update_Success(t *testing.T) {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "update-test",
			ResourceVersion: "1",
		},
		Status: workspacev1alpha1.WorkspaceStatus{
			AppliedClusters: []string{"cluster-1"},
		},
	}

	c := newWorkspaceFakeClient(ws).Build()
	svc := NewWorkspaceService(c)

	// Update workspace
	ws.Status.AppliedClusters = []string{"cluster-1", "cluster-2"}
	err := svc.Update(context.Background(), &UpdateWorkspaceRequest{Workspace: ws})
	require.NoError(t, err)

	// Verify updated
	result, err := svc.Get(context.Background(), "update-test")
	require.NoError(t, err)
	assert.Len(t, result.Status.AppliedClusters, 2)
}

func TestWorkspaceService_Delete_Success(t *testing.T) {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "to-delete"},
	}

	c := newWorkspaceFakeClient(ws).Build()
	svc := NewWorkspaceService(c)

	err := svc.Delete(context.Background(), "to-delete")
	require.NoError(t, err)

	// Verify deleted
	_, err = svc.Get(context.Background(), "to-delete")
	assert.Error(t, err)
}

func TestWorkspaceService_Delete_NotFound(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	err := svc.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestWorkspaceService_GetClustersByWorkspace_Success(t *testing.T) {
	appliedClusters := []string{"cluster-a", "cluster-b"}
	failedClusters := []workspacev1alpha1.ClusterError{
		{Name: "cluster-c", Message: "connection timeout"},
	}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "clusters-test"},
		Status: workspacev1alpha1.WorkspaceStatus{
			AppliedClusters: appliedClusters,
			FailedClusters:  failedClusters,
		},
	}

	c := newWorkspaceFakeClient(ws).Build()
	svc := NewWorkspaceService(c)

	applied, failed, err := svc.GetClustersByWorkspace(context.Background(), "clusters-test")
	require.NoError(t, err)
	assert.Equal(t, appliedClusters, applied)
	assert.Len(t, failed, 1)
	assert.Equal(t, "cluster-c", failed[0].Name)
}

func TestWorkspaceService_GetClustersByWorkspace_NoAppliedClusters(t *testing.T) {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "no-clusters-ws"},
		Status: workspacev1alpha1.WorkspaceStatus{
			AppliedClusters: []string{},
			FailedClusters:  []workspacev1alpha1.ClusterError{},
		},
	}

	c := newWorkspaceFakeClient(ws).Build()
	svc := NewWorkspaceService(c)

	applied, failed, err := svc.GetClustersByWorkspace(context.Background(), "no-clusters-ws")
	require.NoError(t, err)
	assert.Empty(t, applied)
	assert.Empty(t, failed)
}

func TestWorkspaceService_GetClustersByWorkspace_NotFound(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	_, _, err := svc.GetClustersByWorkspace(context.Background(), "nonexistent")
	assert.Error(t, err)
}

// ===== Error path tests =====

func TestWorkspaceService_List_K8sAPIError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.listErr = newK8sInternalError("api server down")
	svc := NewWorkspaceService(c)

	_, err := svc.List(context.Background(), &ListWorkspacesRequest{})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestWorkspaceService_List_WithPagination(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	svc := NewWorkspaceService(c)

	// Verify pagination params pass through without error
	_, err := svc.List(context.Background(), &ListWorkspacesRequest{Limit: 5, Continue: "token"})
	assert.NoError(t, err)
}

func TestWorkspaceService_List_EmptyWithClusterFilter(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewWorkspaceService(c)

	result, err := svc.List(context.Background(), &ListWorkspacesRequest{Cluster: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

func TestWorkspaceService_Get_NotFound_K8sError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.getErr = newK8sNotFound("workspaces", "nonexistent")
	svc := NewWorkspaceService(c)

	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestWorkspaceService_Create_AlreadyExists(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.createErr = newK8sAlreadyExists("workspaces", "existing-ws")
	svc := NewWorkspaceService(c)

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-ws"},
	}
	err := svc.Create(context.Background(), &CreateWorkspaceRequest{Workspace: ws})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeConflict))
}

func TestWorkspaceService_Create_K8sAPIError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.createErr = newK8sInternalError("api server down")
	svc := NewWorkspaceService(c)

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "new-ws"},
	}
	err := svc.Create(context.Background(), &CreateWorkspaceRequest{Workspace: ws})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestWorkspaceService_Update_Conflict(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.updateErr = newK8sConflict("workspaces", "ws-1")
	svc := NewWorkspaceService(c)

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
	}
	err := svc.Update(context.Background(), &UpdateWorkspaceRequest{Workspace: ws})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeConflict))
}

func TestWorkspaceService_Update_K8sAPIError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.updateErr = newK8sInternalError("api server down")
	svc := NewWorkspaceService(c)

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
	}
	err := svc.Update(context.Background(), &UpdateWorkspaceRequest{Workspace: ws})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestWorkspaceService_Delete_K8sAPIError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.deleteErr = newK8sNotFound("workspaces", "nonexistent")
	svc := NewWorkspaceService(c)

	err := svc.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestWorkspaceService_GetClustersByWorkspace_K8sAPIError(t *testing.T) {
	scheme := setupWorkspaceScheme()
	c := newFailingClient(scheme)
	c.getErr = newK8sNotFound("workspaces", "nonexistent")
	svc := NewWorkspaceService(c)

	_, _, err := svc.GetClustersByWorkspace(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestWorkspaceService_List_FilterCluster_NoMatch(t *testing.T) {
	// Workspace with applied clusters, but filter doesn't match any
	workspaces := []client.Object{
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
			Status: workspacev1alpha1.WorkspaceStatus{
				AppliedClusters: []string{"cluster-a"},
			},
		},
	}

	c := newWorkspaceFakeClient(workspaces...).Build()
	svc := NewWorkspaceService(c)

	result, err := svc.List(context.Background(), &ListWorkspacesRequest{Cluster: "cluster-z"})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

func TestWorkspaceService_List_ContinueToken(t *testing.T) {
	workspaces := []client.Object{
		&workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		},
	}

	c := newWorkspaceFakeClient(workspaces...).Build()
	svc := NewWorkspaceService(c)

	result, err := svc.List(context.Background(), &ListWorkspacesRequest{Continue: "some-token"})
	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
}