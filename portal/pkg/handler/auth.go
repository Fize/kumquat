package handler

import (
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// LoginRequest 登录请求
// swagger:model
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"admin123"`
}

// RegisterRequest 注册请求
// swagger:model
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"john_doe"`
	Email    string `json:"email" binding:"required,email" example:"john@example.com"`
	Password string `json:"password" binding:"required,min=6,max=32" example:"password123"`
	Nickname string `json:"nickname" example:"John"`
}

// ChangePasswordRequest 修改密码请求
// swagger:model
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required" example:"oldpass123"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=32" example:"newpass123"`
}

// AuthController 认证控制器（手动路由注册，不使用 RestfulAPI）
type AuthController struct {
	authService   *service.AuthService
	authMiddleware *middleware.AuthMiddleware
}

// NewAuthController 创建认证控制器
func NewAuthController(authService *service.AuthService, authMiddleware *middleware.AuthMiddleware) *AuthController {
	return &AuthController{authService: authService, authMiddleware: authMiddleware}
}

// SetupRoutes 注册 auth 路由（手动，不走 RestfulAPI）
func (h *AuthController) SetupRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		auth.POST("/login", h.Login)
		auth.POST("/register", h.DoRegister)
		protected := auth.Group("")
		protected.Use(h.authMiddleware.Auth())
		{
			protected.GET("/me", h.Me)
			protected.PUT("/change-password", h.ChangePassword)
		}
	}
}

// Login 用户登录
// @Summary 用户登录
// @Description 使用用户名和密码获取 JWT Token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "登录请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{\"token\":\"...\",\"user\":{...}}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"...\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"...\"}"
// @Router /auth/login [post]
func (h *AuthController) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WarnContext(c.Request.Context(), "login request validation failed", "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	token, user, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		log.WarnContext(c.Request.Context(), "login failed", "username", req.Username, "err", err)
		utils.Unauthorized(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "user logged in", "user_id", user.ID, "username", user.Username)
	utils.Success(c, gin.H{"token": token, "user": user.ToResponse()})
}

// DoRegister 用户注册
// @Summary 用户注册
// @Description 注册新用户账号，默认角色为 guest
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "注册请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"...\"}"
// @Failure 409 {object} map[string]interface{} "{\"code\":409,\"message\":\"用户名或邮箱已存在\"}"
// @Router /auth/register [post]
func (h *AuthController) DoRegister(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=32"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6,max=32"`
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WarnContext(c.Request.Context(), "register request validation failed", "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.authService.Register(c.Request.Context(), req.Username, req.Email, req.Password, req.Nickname)
	if err != nil {
		log.WarnContext(c.Request.Context(), "register failed", "username", req.Username, "err", err)
		utils.Conflict(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "user registered", "user_id", user.ID, "username", user.Username)
	utils.Success(c, user.ToResponse())
}

// Me 获取当前用户信息
// @Summary 获取当前登录用户信息
// @Description 获取当前 JWT Token 对应的用户信息
// @Tags auth
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Router /auth/me [get]
func (h *AuthController) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		log.WarnContext(c.Request.Context(), "get current user failed", "user_id", userID, "err", err)
		utils.NotFound(c, "user not found")
		return
	}
	utils.Success(c, user.ToResponse())
}

// ChangePassword 修改密码
// @Summary 修改当前用户密码
// @Description 修改当前登录用户的密码
// @Tags auth
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"password changed successfully\"}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"...\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"旧密码错误\"}"
// @Router /auth/change-password [put]
func (h *AuthController) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6,max=32"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WarnContext(c.Request.Context(), "change password request validation failed", "user_id", userID, "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		log.WarnContext(c.Request.Context(), "change password failed", "user_id", userID, "err", err)
		utils.Forbidden(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "password changed", "user_id", userID)
	utils.SuccessWithMessage(c, "password changed successfully", nil)
}
