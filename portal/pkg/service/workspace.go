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

// ListWorkspacesRequest 工作空间列表查询参数
type ListWorkspacesRequest struct {
	Cluster  string // 筛选特定集群上的工作空间
	Limit    int64  // 分页大小，0 表示不限制
	Continue string // 分页游标
}

// ListWorkspacesResponse 工作空间列表响应
type ListWorkspacesResponse struct {
	Items    []workspacev1alpha1.Workspace `json:"items"`
	Continue string                        `json:"continue,omitempty"` // 下一页游标
}

// List 列出所有工作空间
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

	// 过滤
	var filtered []workspacev1alpha1.Workspace
	for _, ws := range workspaceList.Items {
		if req.Cluster != "" {
			// 检查工作空间是否应用到指定集群
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

// Get 获取工作空间详情
func (s *WorkspaceService) Get(ctx context.Context, name string) (*workspacev1alpha1.Workspace, error) {
	workspace := &workspacev1alpha1.Workspace{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		return nil, wrapK8sError(err, fmt.Sprintf("failed to get workspace %s", name))
	}
	return workspace, nil
}

// CreateWorkspaceRequest 创建工作空间请求
type CreateWorkspaceRequest struct {
	Workspace *workspacev1alpha1.Workspace
}

// Create 创建工作空间
func (s *WorkspaceService) Create(ctx context.Context, req *CreateWorkspaceRequest) error {
	if err := s.k8sClient.Create(ctx, req.Workspace); err != nil {
		return wrapK8sError(err, "failed to create workspace")
	}
	return nil
}

// UpdateWorkspaceRequest 更新工作空间请求
type UpdateWorkspaceRequest struct {
	Workspace *workspacev1alpha1.Workspace
}

// Update 更新工作空间
func (s *WorkspaceService) Update(ctx context.Context, req *UpdateWorkspaceRequest) error {
	if err := s.k8sClient.Update(ctx, req.Workspace); err != nil {
		return wrapK8sError(err, "failed to update workspace")
	}
	return nil
}

// Delete 删除工作空间
func (s *WorkspaceService) Delete(ctx context.Context, name string) error {
	workspace := &workspacev1alpha1.Workspace{}
	workspace.Name = name
	if err := s.k8sClient.Delete(ctx, workspace); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to delete workspace %s", name))
	}
	return nil
}

// GetClustersByWorkspace 获取工作空间已应用的集群列表
func (s *WorkspaceService) GetClustersByWorkspace(ctx context.Context, name string) ([]string, []workspacev1alpha1.ClusterError, error) {
	workspace, err := s.Get(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	return workspace.Status.AppliedClusters, workspace.Status.FailedClusters, nil
}
