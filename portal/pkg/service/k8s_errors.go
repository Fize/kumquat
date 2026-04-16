package service

import (
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// wrapK8sError 将 K8s API 错误转换为 AppError，保留原始错误用于链式解包
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
