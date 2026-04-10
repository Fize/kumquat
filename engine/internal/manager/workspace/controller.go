/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workspace

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/fize/kumquat/engine/internal/manager/cluster"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	pkglabel "github.com/fize/kumquat/engine/pkg/util/labels"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *cluster.ClientManager
}

// +kubebuilder:rbac:groups=workspace.kumquat.io,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workspace.kumquat.io,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=storage.kumquat.io,resources=managedclusters,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var workspace workspacev1alpha1.Workspace
	if err := r.Get(ctx, req.NamespacedName, &workspace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nsName := workspace.Spec.Name
	if nsName == "" {
		nsName = workspace.Name
	}

	// 0. Ensure namespace exists in Hub cluster
	hubNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	// Create or Update the Namespace, ensuring the OwnerReference is set (Adoption)
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, hubNS, func() error {
		pkglabel.AddManagedBy(hubNS)

		// This block is executed for both Create (before) and Update (after fetching)
		// It ensures that even if the Namespace exists, we try to adopt it by setting the OwnerRef.
		return controllerutil.SetControllerReference(&workspace, hubNS, r.Scheme)
	}); err != nil {
		logger.Error(err, "Failed to reconcile hub namespace")
		return ctrl.Result{}, err
	}

	// 1. List all ManagedClusters
	var clusterList storagev1alpha1.ManagedClusterList
	if err := r.List(ctx, &clusterList); err != nil {
		logger.Error(err, "Failed to list managed clusters")
		return ctrl.Result{}, err
	}

	// 2. Filter clusters
	var targetClusters []storagev1alpha1.ManagedCluster
	selector, err := metav1.LabelSelectorAsSelector(workspace.Spec.ClusterSelector)
	if err != nil {
		logger.Error(err, "Invalid cluster selector")
		return ctrl.Result{}, nil
	}

	for _, c := range clusterList.Items {
		if selector.Matches(labels.Set(c.Labels)) {
			targetClusters = append(targetClusters, c)
		}
	}

	// 3. Propagate to each cluster
	var appliedClusters []string
	var failedClusters []workspacev1alpha1.ClusterError
	now := metav1.Now()

	for _, c := range targetClusters {
		if err := r.reconcileCluster(ctx, &workspace, c.Name, nsName); err != nil {
			logger.Error(err, "Failed to reconcile workspace in cluster", "cluster", c.Name)
			failedClusters = append(failedClusters, workspacev1alpha1.ClusterError{
				Name:               c.Name,
				Message:            err.Error(),
				LastTransitionTime: now,
			})
			continue
		}
		appliedClusters = append(appliedClusters, c.Name)
	}

	// 4. Update status
	workspace.Status.AppliedClusters = appliedClusters
	workspace.Status.FailedClusters = failedClusters

	// Set Ready condition
	readyCondition := metav1.Condition{
		Type:               "Ready",
		LastTransitionTime: now,
	}
	if len(failedClusters) == 0 && len(appliedClusters) > 0 {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "AllClustersReady"
		readyCondition.Message = fmt.Sprintf("Workspace successfully applied to %d cluster(s)", len(appliedClusters))
	} else if len(appliedClusters) > 0 {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "PartialFailure"
		readyCondition.Message = fmt.Sprintf("Workspace applied to %d cluster(s), failed on %d cluster(s)", len(appliedClusters), len(failedClusters))
	} else if len(targetClusters) > 0 {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "AllClustersFailed"
		readyCondition.Message = fmt.Sprintf("Workspace failed to apply to all %d target cluster(s)", len(targetClusters))
	} else {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "NoTargetClusters"
		readyCondition.Message = "No clusters match the selector"
	}

	// Update or add the condition
	found := false
	for i, cond := range workspace.Status.Conditions {
		if cond.Type == "Ready" {
			workspace.Status.Conditions[i] = readyCondition
			found = true
			break
		}
	}
	if !found {
		workspace.Status.Conditions = append(workspace.Status.Conditions, readyCondition)
	}

	if err := r.Status().Update(ctx, &workspace); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if there are failures to retry
	if len(failedClusters) > 0 {
		return ctrl.Result{RequeueAfter: 30}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileCluster reconciles the workspace resources in a single cluster
func (r *WorkspaceReconciler) reconcileCluster(ctx context.Context, workspace *workspacev1alpha1.Workspace, clusterName, nsName string) error {
	edgeClient, err := r.ClientManager.GetClient(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	if _, err := ctrl.CreateOrUpdate(ctx, edgeClient, ns, func() error {
		pkglabel.AddManagedBy(ns)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to reconcile namespace: %w", err)
	}

	// Create ResourceQuota if specified
	if workspace.Spec.ResourceConstraints != nil && workspace.Spec.ResourceConstraints.Quota != nil {
		quota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workspace-quota",
				Namespace: nsName,
			},
		}
		if _, err := ctrl.CreateOrUpdate(ctx, edgeClient, quota, func() error {
			pkglabel.AddManagedBy(quota)
			quota.Spec = *workspace.Spec.ResourceConstraints.Quota
			return nil
		}); err != nil {
			return fmt.Errorf("failed to reconcile quota: %w", err)
		}
	}

	// Create LimitRange if specified
	if workspace.Spec.ResourceConstraints != nil && workspace.Spec.ResourceConstraints.LimitRange != nil {
		limits := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workspace-limits",
				Namespace: nsName,
			},
		}
		if _, err := ctrl.CreateOrUpdate(ctx, edgeClient, limits, func() error {
			pkglabel.AddManagedBy(limits)
			limits.Spec = *workspace.Spec.ResourceConstraints.LimitRange
			return nil
		}); err != nil {
			return fmt.Errorf("failed to reconcile limitrange: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workspacev1alpha1.Workspace{}).
		Complete(r)
}
