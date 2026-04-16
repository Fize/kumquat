package service

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkspaceService provides operations for workspace management
type WorkspaceService struct {
	k8sClient client.Client
}

// NewWorkspaceService creates a new workspace service
func NewWorkspaceService(k8sClient client.Client) *WorkspaceService {
	return &WorkspaceService{k8sClient: k8sClient}
}

// ListWorkspacesRequest workspace list query parameters
type ListWorkspacesRequest struct {
	Cluster  string // filter workspaces on a specific cluster
	Limit    int64  // page size, 0 means no limit
	Continue string // pagination cursor
}

// ListWorkspacesResponse workspace list response
type ListWorkspacesResponse struct {
	Items    []workspacev1alpha1.Workspace `json:"items"`
	Continue string                        `json:"continue,omitempty"` // next page cursor
}

// List lists all workspaces
func (s *WorkspaceService) List(ctx context.Context, req *ListWorkspacesRequest) (*ListWorkspacesResponse, error) {
	workspaceList := &workspacev1alpha1.WorkspaceList{}
	opts := []client.ListOption{}
	if req.Limit > 0 {
		opts = append(opts, client.Limit(req.Limit))
	}
	if req.Continue != "" {
		opts = append(opts, client.Continue(req.Continue))
	}

	if err := s.k8sClient.List(ctx, workspaceList, opts...); err != nil {
		return nil, wrapK8sError(err, "failed to list workspaces")
	}

	// Filter
	var filtered []workspacev1alpha1.Workspace
	for _, ws := range workspaceList.Items {
		if req.Cluster != "" {
			// Check if workspace is applied to the specified cluster
			found := false
			for _, cluster := range ws.Status.AppliedClusters {
				if cluster == req.Cluster {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, ws)
	}

	return &ListWorkspacesResponse{
		Items:    filtered,
		Continue: workspaceList.Continue,
	}, nil
}

// Get gets workspace details
func (s *WorkspaceService) Get(ctx context.Context, name string) (*workspacev1alpha1.Workspace, error) {
	workspace := &workspacev1alpha1.Workspace{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		return nil, wrapK8sError(err, fmt.Sprintf("failed to get workspace %s", name))
	}
	return workspace, nil
}

// CreateWorkspaceRequest create workspace request
type CreateWorkspaceRequest struct {
	Workspace *workspacev1alpha1.Workspace
}

// Create creates a workspace
func (s *WorkspaceService) Create(ctx context.Context, req *CreateWorkspaceRequest) error {
	if err := s.k8sClient.Create(ctx, req.Workspace); err != nil {
		return wrapK8sError(err, "failed to create workspace")
	}
	return nil
}

// UpdateWorkspaceRequest update workspace request
type UpdateWorkspaceRequest struct {
	Workspace *workspacev1alpha1.Workspace
}

// Update updates a workspace
func (s *WorkspaceService) Update(ctx context.Context, req *UpdateWorkspaceRequest) error {
	if err := s.k8sClient.Update(ctx, req.Workspace); err != nil {
		return wrapK8sError(err, "failed to update workspace")
	}
	return nil
}

// Delete deletes a workspace
func (s *WorkspaceService) Delete(ctx context.Context, name string) error {
	workspace := &workspacev1alpha1.Workspace{}
	workspace.Name = name
	if err := s.k8sClient.Delete(ctx, workspace); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to delete workspace %s", name))
	}
	return nil
}

// GetClustersByWorkspace gets the list of clusters the workspace is applied to
func (s *WorkspaceService) GetClustersByWorkspace(ctx context.Context, name string) ([]string, []workspacev1alpha1.ClusterError, error) {
	workspace, err := s.Get(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	return workspace.Status.AppliedClusters, workspace.Status.FailedClusters, nil
}
