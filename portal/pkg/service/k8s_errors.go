package service

import (
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// wrapK8sError wraps K8s API errors to AppError, preserving original error for chain unwrapping
func wrapK8sError(err error, message string) error {
	if err == nil {
		return nil
	}
	switch {
	case apierrors.IsNotFound(err):
		return apperr.Wrap(apperr.CodeNotFound, message, err)
	case apierrors.IsConflict(err):
		return apperr.Wrap(apperr.CodeConflict, message, err)
	case apierrors.IsAlreadyExists(err):
		return apperr.Wrap(apperr.CodeConflict, message, err)
	case apierrors.IsBadRequest(err):
		return apperr.Wrap(apperr.CodeBadRequest, message, err)
	case apierrors.IsForbidden(err):
		return apperr.Wrap(apperr.CodeForbidden, message, err)
	case apierrors.IsUnauthorized(err):
		return apperr.Wrap(apperr.CodeUnauthorized, message, err)
	default:
		return apperr.Wrap(apperr.CodeInternal, message, err)
	}
}
