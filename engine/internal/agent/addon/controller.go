package addon

import (
	"context"
	"errors"
	"time"

	"github.com/fize/kumquat/engine/internal/addon"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AddonReconciler reconciles Addons on ManagedCluster from the Agent side
type AddonReconciler struct {
	HubClient   client.Client
	LocalClient client.Client
	Scheme      *runtime.Scheme
	ClusterName string
	Controllers map[string]addon.AddonController
	// Registry allows injecting a custom addon registry for testing
	Registry addon.AddonRegistry
}

// getRegistry returns the addon registry, using global one if not set
func (r *AddonReconciler) getRegistry() addon.AddonRegistry {
	if r.Registry != nil {
		return r.Registry
	}
	return addon.GetRegistry()
}

// Reconcile handles the addon reconciliation from the Agent side
func (r *AddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Only reconcile if this is our cluster
	if req.Name != r.ClusterName {
		return ctrl.Result{}, nil
	}

	var cluster storagev1alpha1.ManagedCluster
	if err := r.HubClient.Get(ctx, req.NamespacedName, &cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling addons for cluster", "cluster", cluster.Name)

	// Track addon statuses to write back to Hub
	statusUpdated := false
	addonStatuses := cluster.Status.AddonStatus

	// Iterate over all registered addons
	for _, a := range r.getRegistry().List() {
		addonName := a.Name()

		// Check if enabled in cluster spec
		enabled := false
		var config map[string]string

		for _, ca := range cluster.Spec.Addons {
			if ca.Name == addonName {
				enabled = ca.Enabled
				config = ca.Config
				break
			}
		}

		if !enabled {
			// Update status to Disabled
			addonStatuses = updateAddonStatus(addonStatuses, addonName, "Disabled", "Addon is disabled")
			statusUpdated = true
			continue
		}

		controller, ok := r.Controllers[addonName]
		if !ok || controller == nil {
			logger.V(1).Info("No AgentController for addon, skipping", "addon", addonName)
			addonStatuses = updateAddonStatus(addonStatuses, addonName, "Pending", "Controller not registered")
			statusUpdated = true
			continue
		}

		addonConfig := addon.AddonConfig{
			ClusterName: cluster.Name,
			Config:      config,
			Client:      r.LocalClient,
		}

		logger.Info("Calling AgentController.Reconcile", "addon", addonName)
		if err := controller.Reconcile(ctx, addonConfig); err != nil {
			if errors.Is(err, addon.ErrPrerequisitesNotMet) {
				// Prerequisites not met (e.g., missing URL config) - mark as Pending
				logger.V(1).Info("Addon prerequisites not met, will retry", "addon", addonName, "error", err)
				addonStatuses = updateAddonStatus(addonStatuses, addonName, "Pending", err.Error())
			} else {
				// Real error - mark as Failed
				logger.Error(err, "Failed to reconcile addon", "addon", addonName)
				addonStatuses = updateAddonStatus(addonStatuses, addonName, "Failed", err.Error())
			}
			statusUpdated = true
			continue
		}
		logger.Info("Successfully reconciled addon", "addon", addonName)
		addonStatuses = updateAddonStatus(addonStatuses, addonName, "Applied", "Addon successfully applied")
		statusUpdated = true
	}

	// Write addon statuses back to Hub
	if statusUpdated {
		var latestCluster storagev1alpha1.ManagedCluster
		if err := r.HubClient.Get(ctx, req.NamespacedName, &latestCluster); err != nil {
			return ctrl.Result{}, err
		}
		latestCluster.Status.AddonStatus = addonStatuses
		if err := r.HubClient.Status().Update(ctx, &latestCluster); err != nil {
			logger.Error(err, "Failed to update addon status on Hub")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// updateAddonStatus updates or appends an addon status entry
func updateAddonStatus(statuses []storagev1alpha1.AddonStatus, name, state, message string) []storagev1alpha1.AddonStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			statuses[i].State = state
			statuses[i].Message = message
			return statuses
		}
	}
	return append(statuses, storagev1alpha1.AddonStatus{
		Name:    name,
		State:   state,
		Message: message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Controllers = make(map[string]addon.AddonController)

	for _, a := range r.getRegistry().List() {
		c, err := a.AgentController(mgr)
		if err != nil {
			return err
		}
		if c != nil {
			r.Controllers[a.Name()] = c
		}
	}

	// Register as a Runnable so mgr.Start will invoke Start() periodically.
	return mgr.Add(r)
}

// Start implements the manager.Runnable interface for periodic reconciliation.
// Since Sub clusters typically do not have the ManagedCluster CRD installed,
// we cannot rely on controller-runtime watches. Instead we poll the Hub
// every 30 seconds and reconcile addons locally.
func (r *AddonReconciler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting periodic addon reconciliation", "cluster", r.ClusterName)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: r.ClusterName}}

	// Initial reconciliation
	if _, err := r.Reconcile(ctx, req); err != nil {
		logger.Error(err, "Initial addon reconciliation failed")
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := r.Reconcile(ctx, req); err != nil {
				logger.Error(err, "Periodic addon reconciliation failed")
			}
		}
	}
}
