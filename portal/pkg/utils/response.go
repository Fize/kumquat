package utils

import (
	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// 分页响应
type PageResult struct {
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
	List  interface{} `json:"list"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{Code: 0, Message: "success", Data: data})
}

// SuccessWithMessage 成功响应带消息
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(200, Response{Code: 0, Message: message, Data: data})
}

// Error 错误响应
func Error(c *gin.Context, httpCode int, code int, message string) {
	c.JSON(httpCode, Response{Code: code, Message: message})
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

// InternalError 500
func InternalError(c *gin.Context, message string) {
	if message == "" {
		message = "internal server error"
	}
	Error(c, 500, 500, message)
}

// PageSuccess 分页成功响应
func PageSuccess(c *gin.Context, total int64, page, size int, list interface{}) {
	Success(c, PageResult{Total: total, Page: page, Size: size, List: list})
}
