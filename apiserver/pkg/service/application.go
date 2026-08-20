package service

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplicationService provides operations for application management
type ApplicationService struct {
	k8sClient client.Client
}

// NewApplicationService creates a new application service
func NewApplicationService(k8sClient client.Client) *ApplicationService {
	return &ApplicationService{k8sClient: k8sClient}
}

// ListApplicationsRequest application list query parameters
type ListApplicationsRequest struct {
	Namespace       string
	SchedulingPhase string // Pending, Scheduling, Scheduled, Descheduling, Failed
	HealthPhase     string // Healthy, Progressing, Degraded, Unknown
	Limit           int64  // page size, 0 means no limit
	Continue        string // pagination cursor
}

// ListApplicationsResponse application list response
type ListApplicationsResponse struct {
	Items    []appsv1alpha1.Application `json:"items"`
	Continue string                     `json:"continue,omitempty"` // next page cursor
}

// List lists applications
func (s *ApplicationService) List(ctx context.Context, req *ListApplicationsRequest) (*ListApplicationsResponse, error) {
	appList := &appsv1alpha1.ApplicationList{}
	opts := []client.ListOption{}
	if req.Namespace != "" {
		opts = append(opts, client.InNamespace(req.Namespace))
	}
	if req.Limit > 0 {
		opts = append(opts, client.Limit(req.Limit))
	}
	if req.Continue != "" {
		opts = append(opts, client.Continue(req.Continue))
	}

	if err := s.k8sClient.List(ctx, appList, opts...); err != nil {
		return nil, wrapK8sError(err, "failed to list applications")
	}

	// Filter
	var filtered []appsv1alpha1.Application
	for _, app := range appList.Items {
		if req.SchedulingPhase != "" && string(app.Status.SchedulingPhase) != req.SchedulingPhase {
			continue
		}
		if req.HealthPhase != "" && string(app.Status.HealthPhase) != req.HealthPhase {
			continue
		}
		filtered = append(filtered, app)
	}

	return &ListApplicationsResponse{
		Items:    filtered,
		Continue: appList.Continue,
	}, nil
}

// Get gets application details
func (s *ApplicationService) Get(ctx context.Context, name, namespace string) (*appsv1alpha1.Application, error) {
	app := &appsv1alpha1.Application{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := s.k8sClient.Get(ctx, key, app); err != nil {
		return nil, wrapK8sError(err, fmt.Sprintf("failed to get application %s/%s", namespace, name))
	}
	return app, nil
}

// CreateApplicationRequest create application request
type CreateApplicationRequest struct {
	Application *appsv1alpha1.Application
}

// Create creates an application
func (s *ApplicationService) Create(ctx context.Context, req *CreateApplicationRequest) error {
	if err := s.k8sClient.Create(ctx, req.Application); err != nil {
		return wrapK8sError(err, "failed to create application")
	}
	return nil
}

// UpdateApplicationRequest update application request
type UpdateApplicationRequest struct {
	Application *appsv1alpha1.Application
}

// Update updates an application
func (s *ApplicationService) Update(ctx context.Context, req *UpdateApplicationRequest) error {
	if err := s.k8sClient.Update(ctx, req.Application); err != nil {
		return wrapK8sError(err, "failed to update application")
	}
	return nil
}

// Delete deletes an application
func (s *ApplicationService) Delete(ctx context.Context, name, namespace string) error {
	app := &appsv1alpha1.Application{}
	app.Name = name
	app.Namespace = namespace
	if err := s.k8sClient.Delete(ctx, app); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to delete application %s/%s", namespace, name))
	}
	return nil
}

// SuspendApplicationRequest suspend/resume application request
type SuspendApplicationRequest struct {
	Name      string
	Namespace string
	Suspend   bool
}

// Suspend suspends or resumes an application
func (s *ApplicationService) Suspend(ctx context.Context, req *SuspendApplicationRequest) error {
	patchData, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"suspend": req.Suspend,
		},
	})
	patch := client.RawPatch(types.MergePatchType, patchData)

	app := &appsv1alpha1.Application{}
	app.Name = req.Name
	app.Namespace = req.Namespace

	if err := s.k8sClient.Patch(ctx, app, patch); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to suspend/resume application %s/%s", req.Namespace, req.Name))
	}
	return nil
}

// ScaleApplicationRequest scale application request
type ScaleApplicationRequest struct {
	Name      string
	Namespace string
	Replicas  int32
}

// Scale scales an application
func (s *ApplicationService) Scale(ctx context.Context, req *ScaleApplicationRequest) error {
	patchData, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": req.Replicas,
		},
	})
	patch := client.RawPatch(types.MergePatchType, patchData)

	app := &appsv1alpha1.Application{}
	app.Name = req.Name
	app.Namespace = req.Namespace

	if err := s.k8sClient.Patch(ctx, app, patch); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to scale application %s/%s", req.Namespace, req.Name))
	}
	return nil
}

// PatchApplicationRequest patch application request
type PatchApplicationRequest struct {
	Name      string
	Namespace string
	Patch     client.Patch
}

// Patch patches an application
func (s *ApplicationService) Patch(ctx context.Context, req *PatchApplicationRequest) error {
	app := &appsv1alpha1.Application{}
	app.Name = req.Name
	app.Namespace = req.Namespace

	if err := s.k8sClient.Patch(ctx, app, req.Patch); err != nil {
		return wrapK8sError(err, fmt.Sprintf("failed to patch application %s/%s", req.Namespace, req.Name))
	}
	return nil
}
