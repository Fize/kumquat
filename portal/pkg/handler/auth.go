package handler

import (
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// LoginRequest represents login request
// swagger:model
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"admin123"`
}

// RegisterRequest represents registration request
// swagger:model
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"john_doe"`
	Email    string `json:"email" binding:"required,email" example:"john@example.com"`
	Password string `json:"password" binding:"required,min=6,max=32" example:"password123"`
	Nickname string `json:"nickname" example:"John"`
}

// ChangePasswordRequest represents change password request
// swagger:model
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required" example:"oldpass123"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=32" example:"newpass123"`
}

// AuthController handles authentication (manual routing, not using RestfulAPI)
type AuthController struct {
	authService   *service.AuthService
	authMiddleware *middleware.AuthMiddleware
}

// NewAuthController creates a new auth controller
func NewAuthController(authService *service.AuthService, authMiddleware *middleware.AuthMiddleware) *AuthController {
	return &AuthController{authService: authService, authMiddleware: authMiddleware}
}

// SetupRoutes registers auth routes (manual, not using RestfulAPI)
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

// Login handles user login
// @Summary User login
// @Description Get JWT token with username and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "login request"
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

// DoRegister handles user registration
// @Summary User registration
// @Description Register a new user account with default role guest
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "registration request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"...\"}"
// @Failure 409 {object} map[string]interface{} "{\"code\":409,\"message\":\"username or email already exists\"}"
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

// Me retrieves current user info
// @Summary Get current logged-in user information
// @Description Get user info for current JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
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

// ChangePassword handles password change
// @Summary Change current user password
// @Description Change password for current logged-in user
// @Tags auth
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ChangePasswordRequest true "change password request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"password changed successfully\"}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"...\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"old password is incorrect\"}"
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
