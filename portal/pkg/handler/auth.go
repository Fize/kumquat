package handler

import (
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// RegisterRoutes 注册路由
func (h *AuthHandler) RegisterRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		auth.POST("/login", h.Login)
		auth.POST("/register", h.DoRegister)  // 重命名避免冲突
		protected := auth.Group("")
		protected.Use(middleware.Auth())
		{
			protected.GET("/me", h.Me)
			protected.PUT("/change-password", h.ChangePassword)
		}
	}
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	token, user, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		utils.Unauthorized(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"token": token, "user": user.ToResponse()})
}

// DoRegister 用户注册
func (h *AuthHandler) DoRegister(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=32"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6,max=32"`
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.authService.Register(req.Username, req.Email, req.Password, req.Nickname)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, user.ToResponse())
}

// Me 获取当前用户
func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		utils.NotFound(c, "user not found")
		return
	}
	utils.Success(c, user.ToResponse())
}

// ChangePassword 修改密码
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6,max=32"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	if err := h.authService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "password changed successfully", nil)
}
