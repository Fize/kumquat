package service

import (
	"context"
	"encoding/json"
	"fmt"

	apperr "github.com/fize/kumquat/portal/pkg/errors"
	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/cluster/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterService provides operations for cluster management
type ClusterService struct {
	k8sClient client.Client
}

// NewClusterService creates a new cluster service
func NewClusterService(k8sClient client.Client) *ClusterService {
	return &ClusterService{k8sClient: k8sClient}
}

// ListClustersRequest 集群列表查询参数
type ListClustersRequest struct {
	State          string // Pending, Ready, Offline, Rejected
	ConnectionMode string // Hub, Edge
	Limit          int64  // 分页大小，0 表示不限制
	Continue       string // 分页游标
}

// ListClustersResponse 集群列表响应
type ListClustersResponse struct {
	Items    []clusterv1alpha1.Cluster `json:"items"`
	Continue string                    `json:"continue,omitempty"` // 下一页游标
}

// List 列出所有集群
func (s *ClusterService) List(ctx context.Context, req *ListClustersRequest) (*ListClustersResponse, error) {
	clusterList := &clusterv1alpha1.ClusterList{}
	opts := []client.ListOption{}
	if req.Limit > 0 {
		opts = append(opts, client.Limit(req.Limit))
	}
	if req.Continue != "" {
		opts = append(opts, client.Continue(req.Continue))
	}

	if err := s.k8sClient.List(ctx, clusterList, opts...); err != nil {
		return nil, wrapK8sError(err, "failed to list clusters")
	}

	// 过滤
	var filtered []clusterv1alpha1.Cluster
	for _, cluster := range clusterList.Items {
		if req.State != "" && string(cluster.Status.State) != req.State {
			continue
		}
		if req.ConnectionMode != "" && string(cluster.Spec.ConnectionMode) != req.ConnectionMode {
			continue
		}
		filtered = append(filtered, cluster)
	}

	return &ListClustersResponse{
		Items:    filtered,
		Continue: clusterList.Continue,
	}, nil
}

// Get 获取单个集群详情
func (s *ClusterService) Get(ctx context.Context, name string) (*clusterv1alpha1.Cluster, error) {
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, cluster); err != nil {
		return nil, wrapK8sError(err, fmt.Sprintf("failed to get cluster %s", name))
	}
	return cluster, nil
}

// ApproveClusterRequest 批准集群请求
type ApproveClusterRequest struct {
	Name string
}

// Approve 批准集群（Pending -> Ready）
func (s *ClusterService) Approve(ctx context.Context, req *ApproveClusterRequest) error {
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: req.Name}, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to get cluster %s", req.Name))
	}

	if cluster.Status.State != clusterv1alpha1.ClusterPending {
		return apperr.New(apperr.CodeBadRequest, fmt.Sprintf("cluster %s is not in Pending state, current: %s", req.Name, cluster.Status.State))
	}

	cluster.Status.State = clusterv1alpha1.ClusterReady
	if err := s.k8sClient.Status().Update(ctx, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to approve cluster %s", req.Name))
	}
	return nil
}

// RejectClusterRequest 拒绝集群请求
type RejectClusterRequest struct {
	Name string
}

// Reject 拒绝集群
func (s *ClusterService) Reject(ctx context.Context, req *RejectClusterRequest) error {
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: req.Name}, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to get cluster %s", req.Name))
	}

	if cluster.Status.State != clusterv1alpha1.ClusterPending {
		return apperr.New(apperr.CodeBadRequest, fmt.Sprintf("cluster %s is not in Pending state, current: %s", req.Name, cluster.Status.State))
	}

	cluster.Status.State = clusterv1alpha1.ClusterRejected
	if err := s.k8sClient.Status().Update(ctx, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to reject cluster %s", req.Name))
	}
	return nil
}

// Delete 删除集群
func (s *ClusterService) Delete(ctx context.Context, name string) error {
	cluster := &clusterv1alpha1.Cluster{}
	cluster.Name = name
	if err := s.k8sClient.Delete(ctx, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to delete cluster %s", name))
	}
	return nil
}

// UpdateClusterAddonsRequest 更新集群插件请求
type UpdateClusterAddonsRequest struct {
	Name   string
	Addons []clusterv1alpha1.ClusterAddon
}

// UpdateAddons 更新集群插件配置
func (s *ClusterService) UpdateAddons(ctx context.Context, req *UpdateClusterAddonsRequest) error {
	patchData, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"addons": req.Addons,
		},
	})
	patch := client.RawPatch(types.MergePatchType, patchData)

	cluster := &clusterv1alpha1.Cluster{}
	cluster.Name = req.Name

	if err := s.k8sClient.Patch(ctx, cluster, patch); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to update cluster addons %s", req.Name))
	}
	return nil
}

// GetClusterAddons 获取集群插件配置
func (s *ClusterService) GetClusterAddons(ctx context.Context, name string) ([]clusterv1alpha1.ClusterAddon, []clusterv1alpha1.AddonStatus, error) {
	cluster, err := s.Get(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	return cluster.Spec.Addons, cluster.Status.AddonStatus, nil
}
