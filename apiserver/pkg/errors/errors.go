package errors

import (
	"errors"
	"fmt"
)

// AppError application error
type AppError struct {
	Code    int
	Message string
	Err     error
}

// New creates application error
func New(code int, message string) *AppError {
	if message == "" {
		message = GetMessage(code)
	}
	return &AppError{Code: code, Message: message}
}

// Newf creates application error (formatted message)
func Newf(code int, format string, args ...interface{}) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap wraps error
func Wrap(code int, message string, err error) *AppError {
	if message == "" {
		message = GetMessage(code)
	}
	return &AppError{Code: code, Message: message, Err: err}
}

// WrapCode wraps error with error code
func WrapCode(code int, err error) *AppError {
	return &AppError{Code: code, Message: GetMessage(code), Err: err}
}

// Error implements error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap implements error unwrapping
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is comparison
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// AsAppError attempts to convert to AppError
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// IsCode checks if error is specified error code
func IsCode(err error, code int) bool {
	appErr, ok := AsAppError(err)
	if !ok {
		return false
	}
	return appErr.Code == code
}

// HTTPStatus gets HTTP status code
func (e *AppError) HTTPStatus() int {
	switch {
	case e.Code >= 1000:
		// Business error codes map to 400
		return 400
	case e.Code >= 500:
		return e.Code
	case e.Code >= 400:
		return e.Code
	default:
		return 500
	}
}

// Convenience constructors

// NotFoundError creates 404 error
func NotFoundError(message string) *AppError {
	return New(CodeNotFound, message)
}

// BadRequestError creates 400 error
func BadRequestError(message string) *AppError {
	return New(CodeBadRequest, message)
}

// UnauthorizedError creates 401 error
func UnauthorizedError(message string) *AppError {
	return New(CodeUnauthorized, message)
}

// ForbiddenError creates 403 error
func ForbiddenError(message string) *AppError {
	return New(CodeForbidden, message)
}

// ConflictError creates 409 error
func ConflictError(message string) *AppError {
	return New(CodeConflict, message)
}

// InternalError creates 500 error
func InternalError(message string) *AppError {
	return New(CodeInternal, message)
}
