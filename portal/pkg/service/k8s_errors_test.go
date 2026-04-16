package service

import (
	"fmt"
	"testing"

	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWrapK8sError_Nil(t *testing.T) {
	result := wrapK8sError(nil, "test")
	assert.Nil(t, result)
}

func TestWrapK8sError_NotFound(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "clusters"}, "test-cluster")
	result := wrapK8sError(err, "cluster not found")
	assert.True(t, apperr.IsCode(result, apperr.CodeNotFound))
}

func TestWrapK8sError_Conflict(t *testing.T) {
	err := apierrors.NewConflict(schema.GroupResource{Group: "", Resource: "clusters"}, "test-cluster", nil)
	result := wrapK8sError(err, "cluster conflict")
	assert.True(t, apperr.IsCode(result, apperr.CodeConflict))
}

func TestWrapK8sError_AlreadyExists(t *testing.T) {
	err := apierrors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: "clusters"}, "test-cluster")
	result := wrapK8sError(err, "cluster already exists")
	assert.True(t, apperr.IsCode(result, apperr.CodeConflict))
}

func TestWrapK8sError_BadRequest(t *testing.T) {
	err := apierrors.NewBadRequest("invalid request")
	result := wrapK8sError(err, "bad request")
	assert.True(t, apperr.IsCode(result, apperr.CodeBadRequest))
}

func TestWrapK8sError_Forbidden(t *testing.T) {
	err := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "clusters"}, "test-cluster", nil)
	result := wrapK8sError(err, "forbidden")
	assert.True(t, apperr.IsCode(result, apperr.CodeForbidden))
}

func TestWrapK8sError_Unauthorized(t *testing.T) {
	err := apierrors.NewUnauthorized("unauthorized")
	result := wrapK8sError(err, "unauthorized access")
	assert.True(t, apperr.IsCode(result, apperr.CodeUnauthorized))
}

func TestWrapK8sError_Internal(t *testing.T) {
	err := apierrors.NewInternalError(fmt.Errorf("something broke"))
	result := wrapK8sError(err, "internal error")
	assert.True(t, apperr.IsCode(result, apperr.CodeInternal))
}

func TestWrapK8sError_ServerTimeout(t *testing.T) {
	err := apierrors.NewServerTimeout(schema.GroupResource{Group: "", Resource: "clusters"}, "list", 0)
	result := wrapK8sError(err, "server timeout")
	assert.True(t, apperr.IsCode(result, apperr.CodeInternal))
}

func TestWrapK8sError_ServiceUnavailable(t *testing.T) {
	err := apierrors.NewServiceUnavailable("service unavailable")
	result := wrapK8sError(err, "service unavailable")
	assert.True(t, apperr.IsCode(result, apperr.CodeInternal))
}

func TestWrapK8sError_WrapsOriginal(t *testing.T) {
	inner := apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "clusters"}, "test")
	result := wrapK8sError(inner, "wrapped message")
	assert.NotNil(t, result)

	appErr, ok := apperr.AsAppError(result)
	assert.True(t, ok)
	assert.Equal(t, apperr.CodeNotFound, appErr.Code)
	assert.Contains(t, appErr.Error(), "wrapped message")
	assert.NotNil(t, appErr.Err) // original error preserved
}

func TestWrapK8sError_StatusReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   metav1.StatusReason
		expected int
	}{
		{"NotFound", metav1.StatusReasonNotFound, apperr.CodeNotFound},
		{"AlreadyExists", metav1.StatusReasonAlreadyExists, apperr.CodeConflict},
		{"Conflict", metav1.StatusReasonConflict, apperr.CodeConflict},
		{"Invalid", metav1.StatusReasonInvalid, apperr.CodeInternal}, // IsInvalid not handled in wrapK8sError, falls to default
		{"Forbidden", metav1.StatusReasonForbidden, apperr.CodeForbidden},
		{"Unauthorized", metav1.StatusReasonUnauthorized, apperr.CodeUnauthorized},
		{"InternalError", metav1.StatusReasonInternalError, apperr.CodeInternal},
		{"ServiceUnavailable", metav1.StatusReasonServiceUnavailable, apperr.CodeInternal},
		{"Timeout", metav1.StatusReasonTimeout, apperr.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apierrors.FromObject(&metav1.Status{
				Reason: tt.reason,
				Code:   500,
				Message: tt.name,
			})
			result := wrapK8sError(err, "test")
			assert.True(t, apperr.IsCode(result, tt.expected), "expected code %d for reason %s", tt.expected, tt.reason)
		})
	}
}
