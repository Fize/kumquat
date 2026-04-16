package handler

import (
	"errors"
	"strconv"

	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// CreateUserRequest represents create user request
// swagger:model
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"john_doe"`
	Email    string `json:"email" binding:"required,email" example:"john@example.com"`
	Password string `json:"password" binding:"required,min=6,max=32" example:"password123"`
	Nickname string `json:"nickname" example:"John"`
	RoleID   uint   `json:"role_id" binding:"required" example:"2"`
	ModuleID *uint  `json:"module_id" example:"1"`
}

// UpdateUserRequest represents update user request
// swagger:model
type UpdateUserRequest struct {
	Nickname string `json:"nickname" example:"John Updated"`
	RoleID   uint   `json:"role_id" example:"2"`
	ModuleID *uint  `json:"module_id" example:"1"`
}

// UserController implements RestController interface
type UserController struct {
	svc           *service.UserService
	rs            *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewUserController creates a new user controller
func NewUserController(userSvc *service.UserService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *UserController {
	return &UserController{svc: userSvc, rs: roleSvc, authMiddleware: authMiddleware}
}

func (c *UserController) Name() string { return "users" }
func (c *UserController) Version() string { return "v1" }

func (c *UserController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"GET"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.rs, "user", "read")},
		},
		{
			Methods:     []string{"POST", "DELETE", "PUT"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequireRole("admin")},
		},
	}
}

// List retrieves user list
// @Summary Get user list (paginated)
// @Description Get paginated user list, requires admin role or user:read permission
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param page query int false "page number" default(1)
// @Param size query int false "page size" default(10)
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{users},\"pagination\":{...}}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Router /users [get]
func (c *UserController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		page, size := utils.GetPageSize(ctx)
		users, total, err := c.svc.List(ctx.Request.Context(), page, size)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "list users failed", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}
		list := make([]map[string]interface{}, len(users))
		for i, u := range users {
			list[i] = u.ToResponse()
		}
		utils.PageSuccess(ctx, total, page, size, list)
	}, nil
}

// Get retrieves a single user
// @Summary Get user information by ID
// @Description Get user details by specified ID
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "user ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid user ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"user not found\"}"
// @Router /users/{id} [get]
func (c *UserController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		user, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get user failed", "id", id, "err", err)
			utils.NotFound(ctx, "user not found")
			return
		}
		utils.Success(ctx, user.ToResponse())
	}, nil
}

// Create creates a user
// @Summary Create new user
// @Description Create new user account, only admin role can perform
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateUserRequest true "create user request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid request parameters\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 409 {object} map[string]interface{} "{\"code\":409,\"message\":\"username or email already exists\"}"
// @Router /users [post]
func (c *UserController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required,min=3,max=32"`
			Email    string `json:"email" binding:"required,email"`
			Password string `json:"password" binding:"required,min=6,max=32"`
			Nickname string `json:"nickname"`
			RoleID   uint   `json:"role_id" binding:"required"`
			ModuleID *uint  `json:"module_id"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create user request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		user, err := c.svc.Create(ctx.Request.Context(), req.Username, req.Email, req.Password, req.Nickname, req.RoleID, req.ModuleID)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "create user failed", "username", req.Username, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "user created", "user_id", user.ID, "username", user.Username)
		utils.Success(ctx, user.ToResponse())
	}, nil
}

// Update updates user
// @Summary Update user information
// @Description Update specified user information, only admin role can perform
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "user ID"
// @Param request body UpdateUserRequest true "update user request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid request parameters\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"user not found\"}"
// @Router /users/{id} [put]
func (c *UserController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		var req struct {
			Nickname string `json:"nickname"`
			RoleID   uint   `json:"role_id"`
			ModuleID *uint  `json:"module_id"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "update user request validation failed", "id", id, "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		user, err := c.svc.Update(ctx.Request.Context(), uint(id), req.Nickname, req.RoleID, req.ModuleID)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update user failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "user updated", "user_id", user.ID)
		utils.Success(ctx, user.ToResponse())
	}, nil
}

// Delete deletes user
// @Summary Delete user
// @Description Delete specified user, only admin role can perform. Cannot delete last admin user.
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "user ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"deleted\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"cannot delete last admin user\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"user not found\"}"
// @Router /users/{id} [delete]
func (c *UserController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
			log.WarnContext(ctx.Request.Context(), "delete user failed", "id", id, "err", err)
			if errors.Is(err, errors.New("cannot delete last admin")) {
				utils.Forbidden(ctx, err.Error())
			} else {
				utils.NotFound(ctx, err.Error())
			}
			return
		}
		log.InfoContext(ctx.Request.Context(), "user deleted", "user_id", id)
		utils.SuccessWithMessage(ctx, "deleted", nil)
	}, nil
}

func (c *UserController) Patch() (gin.HandlerFunc, error) {
	return nil, errors.New("patch not implemented")
}
