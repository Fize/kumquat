package utils

import (
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/gin-gonic/gin"
)

// Response unified response structure
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PageResult paginated response
type PageResult struct {
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
	List  interface{} `json:"list"`
}

// Success success response
func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{Code: 0, Message: "success", Data: data})
}

// SuccessWithMessage success response with message
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(200, Response{Code: 0, Message: message, Data: data})
}

// Error error response
func Error(c *gin.Context, httpCode int, code int, message string) {
	c.JSON(httpCode, Response{Code: code, Message: message})
}

// ErrorFromAppError generates error response from AppError
func ErrorFromAppError(c *gin.Context, err *apperr.AppError) {
	c.JSON(err.HTTPStatus(), Response{Code: err.Code, Message: err.Message})
}

// ErrorFromErr generates error response from error
func ErrorFromErr(c *gin.Context, err error) {
	if appErr, ok := apperr.AsAppError(err); ok {
		ErrorFromAppError(c, appErr)
		return
	}
	// Fallback: generic error
	msg := err.Error()
	switch {
	case containsAny(msg, "not found"):
		NotFound(c, msg)
	case containsAny(msg, "already exists"):
		Conflict(c, msg)
	case containsAny(msg, "cannot delete", "insufficient", "not allowed"):
		Forbidden(c, msg)
	case containsAny(msg, "invalid", "incorrect", "required"):
		BadRequest(c, msg)
	default:
		BadRequest(c, msg)
	}
}

// BadRequest 400
func BadRequest(c *gin.Context, message string) {
	Error(c, 400, 400, message)
}

// Unauthorized 401
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "unauthorized"
	}
	Error(c, 401, 401, message)
}

// Forbidden 403
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "forbidden"
	}
	Error(c, 403, 403, message)
}

// NotFound 404
func NotFound(c *gin.Context, message string) {
	if message == "" {
		message = "not found"
	}
	Error(c, 404, 404, message)
}

// Conflict 409
func Conflict(c *gin.Context, message string) {
	Error(c, 409, 409, message)
}

// InternalError 500
func InternalError(c *gin.Context, message string) {
	if message == "" {
		message = "internal server error"
	}
	Error(c, 500, 500, message)
}

// PageSuccess paginated success response
func PageSuccess(c *gin.Context, total int64, page, size int, list interface{}) {
	Success(c, PageResult{Total: total, Page: page, Size: size, List: list})
}

// ErrorFromMessage automatically selects appropriate HTTP status code based on error message
// Deprecated: use ErrorFromErr instead
func ErrorFromMessage(c *gin.Context, msg string) {
	switch {
	case containsAny(msg, "not found"):
		NotFound(c, msg)
	case containsAny(msg, "already exists"):
		Conflict(c, msg)
	case containsAny(msg, "cannot delete", "insufficient", "not allowed"):
		Forbidden(c, msg)
	case containsAny(msg, "invalid", "incorrect", "required"):
		BadRequest(c, msg)
	default:
		BadRequest(c, msg)
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, k := range keywords {
		if len(s) >= len(k) {
			for i := 0; i <= len(s)-len(k); i++ {
				if s[i:i+len(k)] == k {
					return true
				}
			}
		}
	}
	return false
}
