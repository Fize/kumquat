package errors

import (
	"errors"
	"fmt"
)

// AppError 应用错误
type AppError struct {
	Code    int
	Message string
	Err     error
}

// New 创建应用错误
func New(code int, message string) *AppError {
	if message == "" {
		message = GetMessage(code)
	}
	return &AppError{Code: code, Message: message}
}

// Newf 创建应用错误（格式化消息）
func Newf(code int, format string, args ...interface{}) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap 包装错误
func Wrap(code int, message string, err error) *AppError {
	if message == "" {
		message = GetMessage(code)
	}
	return &AppError{Code: code, Message: message, Err: err}
}

// WrapCode 用错误码包装错误
func WrapCode(code int, err error) *AppError {
	return &AppError{Code: code, Message: GetMessage(code), Err: err}
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap 实现错误解包
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is 实现 errors.Is 比较
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// AsAppError 尝试转换为 AppError
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// IsCode 检查错误是否为指定错误码
func IsCode(err error, code int) bool {
	appErr, ok := AsAppError(err)
	if !ok {
		return false
	}
	return appErr.Code == code
}

// HTTPStatus 获取 HTTP 状态码
func (e *AppError) HTTPStatus() int {
	switch {
	case e.Code >= 1000:
		// 业务错误码映射到 400
		return 400
	case e.Code >= 500:
		return e.Code
	case e.Code >= 400:
		return e.Code
	default:
		return 500
	}
}

// 便捷构造函数

// NotFoundError 创建 404 错误
func NotFoundError(message string) *AppError {
	return New(CodeNotFound, message)
}

// BadRequestError 创建 400 错误
func BadRequestError(message string) *AppError {
	return New(CodeBadRequest, message)
}

// UnauthorizedError 创建 401 错误
func UnauthorizedError(message string) *AppError {
	return New(CodeUnauthorized, message)
}

// ForbiddenError 创建 403 错误
func ForbiddenError(message string) *AppError {
	return New(CodeForbidden, message)
}

// ConflictError 创建 409 错误
func ConflictError(message string) *AppError {
	return New(CodeConflict, message)
}

// InternalError 创建 500 错误
func InternalError(message string) *AppError {
	return New(CodeInternal, message)
}
