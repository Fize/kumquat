package service

import (
	"context"
	"testing"

	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupAppScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	return scheme
}

func newAppFakeClient(objects ...client.Object) *fake.ClientBuilder {
	scheme := setupAppScheme()
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...)
}

func TestApplicationService_List_Empty(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	apps, err := svc.List(context.Background(), &ListApplicationsRequest{})
	require.NoError(t, err)
	assert.Empty(t, apps.Items)
}

func TestApplicationService_List_WithApplications(t *testing.T) {
	apps := []client.Object{
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "ns-1"},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "ns-2"},
		},
	}

	c := newAppFakeClient(apps...).Build()
	svc := NewApplicationService(c)

	allApps, err := svc.List(context.Background(), &ListApplicationsRequest{})
	require.NoError(t, err)
	assert.Len(t, allApps.Items, 2)
}

func TestApplicationService_List_FilterByNamespace(t *testing.T) {
	apps := []client.Object{
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "kube-system"},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-3", Namespace: "default"},
		},
	}

	c := newAppFakeClient(apps...).Build()
	svc := NewApplicationService(c)

	// Filter by namespace
	defaultApps, err := svc.List(context.Background(), &ListApplicationsRequest{Namespace: "default"})
	require.NoError(t, err)
	assert.Len(t, defaultApps.Items, 2)

	kubeApps, err := svc.List(context.Background(), &ListApplicationsRequest{Namespace: "kube-system"})
	require.NoError(t, err)
	assert.Len(t, kubeApps.Items, 1)
}

func TestApplicationService_List_FilterBySchedulingPhase(t *testing.T) {
	apps := []client.Object{
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pending", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Pending,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-scheduled", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Scheduled,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-failed", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Failed,
			},
		},
	}

	c := newAppFakeClient(apps...).Build()
	svc := NewApplicationService(c)

	// Filter by Pending
	pendingApps, err := svc.List(context.Background(), &ListApplicationsRequest{SchedulingPhase: "Pending"})
	require.NoError(t, err)
	assert.Len(t, pendingApps.Items, 1)
	assert.Equal(t, "app-pending", pendingApps.Items[0].Name)

	// Filter by Scheduled
	scheduledApps, err := svc.List(context.Background(), &ListApplicationsRequest{SchedulingPhase: "Scheduled"})
	require.NoError(t, err)
	assert.Len(t, scheduledApps.Items, 1)
	assert.Equal(t, "app-scheduled", scheduledApps.Items[0].Name)
}

func TestApplicationService_List_FilterByHealthPhase(t *testing.T) {
	apps := []client.Object{
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-healthy", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				HealthPhase: appsv1alpha1.Healthy,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-degraded", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				HealthPhase: appsv1alpha1.Degraded,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-unknown", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				HealthPhase: appsv1alpha1.Unknown,
			},
		},
	}

	c := newAppFakeClient(apps...).Build()
	svc := NewApplicationService(c)

	// Filter by Healthy
	healthyApps, err := svc.List(context.Background(), &ListApplicationsRequest{HealthPhase: "Healthy"})
	require.NoError(t, err)
	assert.Len(t, healthyApps.Items, 1)
	assert.Equal(t, "app-healthy", healthyApps.Items[0].Name)

	// Filter by Degraded
	degradedApps, err := svc.List(context.Background(), &ListApplicationsRequest{HealthPhase: "Degraded"})
	require.NoError(t, err)
	assert.Len(t, degradedApps.Items, 1)
	assert.Equal(t, "app-degraded", degradedApps.Items[0].Name)
}

func TestApplicationService_List_CombinedFilters(t *testing.T) {
	apps := []client.Object{
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Scheduled,
				HealthPhase:     appsv1alpha1.Healthy,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "default"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Scheduled,
				HealthPhase:     appsv1alpha1.Degraded,
			},
		},
		&appsv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app-3", Namespace: "kube-system"},
			Status: appsv1alpha1.ApplicationStatus{
				SchedulingPhase: appsv1alpha1.Scheduled,
				HealthPhase:     appsv1alpha1.Healthy,
			},
		},
	}

	c := newAppFakeClient(apps...).Build()
	svc := NewApplicationService(c)

	// Combined: namespace=default + health=Healthy
	filtered, err := svc.List(context.Background(), &ListApplicationsRequest{
		Namespace:  "default",
		HealthPhase: "Healthy",
	})
	require.NoError(t, err)
	assert.Len(t, filtered.Items, 1)
	assert.Equal(t, "app-1", filtered.Items[0].Name)
}

func TestApplicationService_Get_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "test-ns"},
		Status: appsv1alpha1.ApplicationStatus{
			HealthPhase: appsv1alpha1.Healthy,
		},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	result, err := svc.Get(context.Background(), "test-app", "test-ns")
	require.NoError(t, err)
	assert.Equal(t, "test-app", result.Name)
	assert.Equal(t, "test-ns", result.Namespace)
}

func TestApplicationService_Get_NotFound(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	_, err := svc.Get(context.Background(), "nonexistent", "default")
	assert.Error(t, err)
}

func TestApplicationService_Create_Success(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "new-app", Namespace: "default"},
	}

	err := svc.Create(context.Background(), &CreateApplicationRequest{Application: app})
	require.NoError(t, err)

	// Verify created
	result, err := svc.Get(context.Background(), "new-app", "default")
	require.NoError(t, err)
	assert.Equal(t, "new-app", result.Name)
}

func TestApplicationService_Update_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "update-test",
			Namespace:       "default",
			ResourceVersion: "1",
		},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Update app
	app.Spec.Replicas = ptrInt32(3)
	err := svc.Update(context.Background(), &UpdateApplicationRequest{Application: app})
	require.NoError(t, err)

	// Verify updated
	result, err := svc.Get(context.Background(), "update-test", "default")
	require.NoError(t, err)
	assert.Equal(t, int32(3), *result.Spec.Replicas)
}

func TestApplicationService_Delete_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "to-delete", Namespace: "default"},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	err := svc.Delete(context.Background(), "to-delete", "default")
	require.NoError(t, err)

	// Verify deleted
	_, err = svc.Get(context.Background(), "to-delete", "default")
	assert.Error(t, err)
}

func TestApplicationService_Delete_NotFound(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	err := svc.Delete(context.Background(), "nonexistent", "default")
	assert.Error(t, err)
}

func TestApplicationService_Suspend_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "suspend-test", Namespace: "default"},
		Spec:       appsv1alpha1.ApplicationSpec{},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Suspend
	err := svc.Suspend(context.Background(), &SuspendApplicationRequest{
		Name:      "suspend-test",
		Namespace: "default",
		Suspend:   true,
	})
	require.NoError(t, err)

	// Verify suspended
	result, err := svc.Get(context.Background(), "suspend-test", "default")
	require.NoError(t, err)
	assert.NotNil(t, result.Spec.Suspend)
	assert.True(t, *result.Spec.Suspend)
}

func TestApplicationService_Suspend_Resume(t *testing.T) {
	suspend := true
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "resume-test", Namespace: "default"},
		Spec: appsv1alpha1.ApplicationSpec{
			Suspend: &suspend,
		},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Resume
	err := svc.Suspend(context.Background(), &SuspendApplicationRequest{
		Name:      "resume-test",
		Namespace: "default",
		Suspend:   false,
	})
	require.NoError(t, err)

	// Verify resumed
	result, err := svc.Get(context.Background(), "resume-test", "default")
	require.NoError(t, err)
	assert.NotNil(t, result.Spec.Suspend)
	assert.False(t, *result.Spec.Suspend)
}

func TestApplicationService_Suspend_NotFound(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	err := svc.Suspend(context.Background(), &SuspendApplicationRequest{
		Name:      "nonexistent",
		Namespace: "default",
		Suspend:   true,
	})
	assert.Error(t, err)
}

func TestApplicationService_Scale_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "scale-test", Namespace: "default"},
		Spec:       appsv1alpha1.ApplicationSpec{},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Scale to 5 replicas
	err := svc.Scale(context.Background(), &ScaleApplicationRequest{
		Name:      "scale-test",
		Namespace: "default",
		Replicas:  5,
	})
	require.NoError(t, err)

	// Verify scaled
	result, err := svc.Get(context.Background(), "scale-test", "default")
	require.NoError(t, err)
	assert.Equal(t, int32(5), *result.Spec.Replicas)
}

func TestApplicationService_Scale_ToZero(t *testing.T) {
	replicas := int32(3)
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "scale-zero-test", Namespace: "default"},
		Spec: appsv1alpha1.ApplicationSpec{
			Replicas: &replicas,
		},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Scale to 0
	err := svc.Scale(context.Background(), &ScaleApplicationRequest{
		Name:      "scale-zero-test",
		Namespace: "default",
		Replicas:  0,
	})
	require.NoError(t, err)

	// Verify scaled to 0
	result, err := svc.Get(context.Background(), "scale-zero-test", "default")
	require.NoError(t, err)
	assert.Equal(t, int32(0), *result.Spec.Replicas)
}

func TestApplicationService_Scale_NotFound(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	err := svc.Scale(context.Background(), &ScaleApplicationRequest{
		Name:      "nonexistent",
		Namespace: "default",
		Replicas:  3,
	})
	assert.Error(t, err)
}

func TestApplicationService_Patch_Success(t *testing.T) {
	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "patch-test", Namespace: "default"},
		Spec:       appsv1alpha1.ApplicationSpec{},
	}

	c := newAppFakeClient(app).Build()
	svc := NewApplicationService(c)

	// Simple strategic merge patch - just update replicas
	patchData := []byte(`{"spec":{"replicas":3}}`)

	err := svc.Patch(context.Background(), &PatchApplicationRequest{
		Name:      "patch-test",
		Namespace: "default",
		Patch:     client.RawPatch(types.MergePatchType, patchData),
	})
	require.NoError(t, err)

	// Verify patched
	result, err := svc.Get(context.Background(), "patch-test", "default")
	require.NoError(t, err)
	assert.Equal(t, int32(3), *result.Spec.Replicas)
}

func TestApplicationService_Patch_NotFound(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	patchData := []byte(`{"spec":{"replicas":3}}`)

	err := svc.Patch(context.Background(), &PatchApplicationRequest{
		Name:      "nonexistent",
		Namespace: "default",
		Patch:     client.RawPatch(types.MergePatchType, patchData),
	})
	assert.Error(t, err)
}

// Helper function
func ptrInt32(i int32) *int32 {
	return &i
}

// ===== Error path tests =====

func TestApplicationService_List_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.listErr = newK8sInternalError("api server down")
	svc := NewApplicationService(c)

	_, err := svc.List(context.Background(), &ListApplicationsRequest{})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestApplicationService_List_WithPagination(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	svc := NewApplicationService(c)

	// Limit and Continue are passed through to K8s ListOption
	// fake client doesn't enforce pagination, but we verify no errors
	_, err := svc.List(context.Background(), &ListApplicationsRequest{Limit: 10, Continue: "token123"})
	assert.NoError(t, err)
}

func TestApplicationService_Create_AlreadyExists(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.createErr = newK8sAlreadyExists("applications", "existing-app")
	svc := NewApplicationService(c)

	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-app", Namespace: "default"},
	}
	err := svc.Create(context.Background(), &CreateApplicationRequest{Application: app})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeConflict))
}

func TestApplicationService_Create_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.createErr = newK8sInternalError("api server down")
	svc := NewApplicationService(c)

	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "new-app", Namespace: "default"},
	}
	err := svc.Create(context.Background(), &CreateApplicationRequest{Application: app})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestApplicationService_Update_Conflict(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.updateErr = newK8sConflict("applications", "app-1")
	svc := NewApplicationService(c)

	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
	}
	err := svc.Update(context.Background(), &UpdateApplicationRequest{Application: app})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeConflict))
}

func TestApplicationService_Update_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.updateErr = newK8sInternalError("api server down")
	svc := NewApplicationService(c)

	app := &appsv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
	}
	err := svc.Update(context.Background(), &UpdateApplicationRequest{Application: app})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestApplicationService_Delete_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.deleteErr = newK8sNotFound("applications", "nonexistent")
	svc := NewApplicationService(c)

	err := svc.Delete(context.Background(), "nonexistent", "default")
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestApplicationService_Get_NotFound_K8sError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.getErr = newK8sNotFound("applications", "nonexistent")
	svc := NewApplicationService(c)

	_, err := svc.Get(context.Background(), "nonexistent", "default")
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestApplicationService_Suspend_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.patchErr = newK8sNotFound("applications", "nonexistent")
	svc := NewApplicationService(c)

	err := svc.Suspend(context.Background(), &SuspendApplicationRequest{
		Name:      "nonexistent",
		Namespace: "default",
		Suspend:   true,
	})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeNotFound))
}

func TestApplicationService_Scale_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.patchErr = newK8sConflict("applications", "scale-app")
	svc := NewApplicationService(c)

	err := svc.Scale(context.Background(), &ScaleApplicationRequest{
		Name:      "scale-app",
		Namespace: "default",
		Replicas:  3,
	})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeConflict))
}

func TestApplicationService_Patch_K8sAPIError(t *testing.T) {
	scheme := setupAppScheme()
	c := newFailingClient(scheme)
	c.patchErr = newK8sInternalError("api server down")
	svc := NewApplicationService(c)

	patchData := []byte(`{"spec":{"replicas":3}}`)
	err := svc.Patch(context.Background(), &PatchApplicationRequest{
		Name:      "patch-app",
		Namespace: "default",
		Patch:     client.RawPatch(types.MergePatchType, patchData),
	})
	assert.Error(t, err)
	assert.True(t, apperr.IsCode(err, apperr.CodeInternal))
}

func TestApplicationService_List_EmptyWithAllFilters(t *testing.T) {
	scheme := setupAppScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := NewApplicationService(c)

	// All filter params set but no data
	result, err := svc.List(context.Background(), &ListApplicationsRequest{
		Namespace:       "default",
		SchedulingPhase: "Scheduled",
		HealthPhase:     "Healthy",
		Limit:           10,
	})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Empty(t, result.Continue)
}