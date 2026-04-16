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

// ListClustersRequest cluster list query parameters
type ListClustersRequest struct {
	State          string // Pending, Ready, Offline, Rejected
	ConnectionMode string // Hub, Edge
	Limit          int64  // page size, 0 means no limit
	Continue       string // pagination cursor
}

// ListClustersResponse cluster list response
type ListClustersResponse struct {
	Items    []clusterv1alpha1.Cluster `json:"items"`
	Continue string                    `json:"continue,omitempty"` // next page cursor
}

// List lists all clusters
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

	// Filter
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

// Get gets single cluster details
func (s *ClusterService) Get(ctx context.Context, name string) (*clusterv1alpha1.Cluster, error) {
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, cluster); err != nil {
		return nil, wrapK8sError(err, fmt.Sprintf("failed to get cluster %s", name))
	}
	return cluster, nil
}

// ApproveClusterRequest approve cluster request
type ApproveClusterRequest struct {
	Name string
}

// Approve approves cluster (Pending -> Ready)
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

// RejectClusterRequest reject cluster request
type RejectClusterRequest struct {
	Name string
}

// Reject rejects cluster
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

// Delete deletes cluster
func (s *ClusterService) Delete(ctx context.Context, name string) error {
	cluster := &clusterv1alpha1.Cluster{}
	cluster.Name = name
	if err := s.k8sClient.Delete(ctx, cluster); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to delete cluster %s", name))
	}
	return nil
}

// UpdateClusterAddonsRequest update cluster addons request
type UpdateClusterAddonsRequest struct {
	Name   string
	Addons []clusterv1alpha1.ClusterAddon
}

// UpdateAddons updates cluster addons configuration
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

// GetClusterAddons gets cluster addons configuration
func (s *ClusterService) GetClusterAddons(ctx context.Context, name string) ([]clusterv1alpha1.ClusterAddon, []clusterv1alpha1.AddonStatus, error) {
	cluster, err := s.Get(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	return cluster.Spec.Addons, cluster.Status.AddonStatus, nil
}
