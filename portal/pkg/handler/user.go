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

// CreateUserRequest 创建用户请求
// swagger:model
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"john_doe"`
	Email    string `json:"email" binding:"required,email" example:"john@example.com"`
	Password string `json:"password" binding:"required,min=6,max=32" example:"password123"`
	Nickname string `json:"nickname" example:"John"`
	RoleID   uint   `json:"role_id" binding:"required" example:"2"`
	ModuleID *uint  `json:"module_id" example:"1"`
}

// UpdateUserRequest 更新用户请求
// swagger:model
type UpdateUserRequest struct {
	Nickname string `json:"nickname" example:"John Updated"`
	RoleID   uint   `json:"role_id" example:"2"`
	ModuleID *uint  `json:"module_id" example:"1"`
}

// UserController 用户控制器，实现 RestController 接口
type UserController struct {
	svc           *service.UserService
	rs            *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewUserController 创建用户控制器
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

// List 获取用户列表
// @Summary 获取用户列表（分页）
// @Description 获取所有用户的分页列表，需要 admin 角色或 user:read 权限
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param size query int false "每页数量" default(10)
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{users},\"pagination\":{...}}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
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

// Get 获取单个用户
// @Summary 根据 ID 获取用户信息
// @Description 获取指定 ID 的用户详情
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"无效的用户ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"用户不存在\"}"
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

// Create 创建用户
// @Summary 创建新用户
// @Description 创建新用户账号，仅 admin 角色可操作
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateUserRequest true "创建用户请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"请求参数错误\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
// @Failure 409 {object} map[string]interface{} "{\"code\":409,\"message\":\"用户名或邮箱已存在\"}"
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

// Update 更新用户
// @Summary 更新用户信息
// @Description 更新指定用户的信息，仅 admin 角色可操作
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "用户ID"
// @Param request body UpdateUserRequest true "更新用户请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{user}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"请求参数错误\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"用户不存在\"}"
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

// Delete 删除用户
// @Summary 删除用户
// @Description 删除指定用户，仅 admin 角色可操作。不能删除最后一个 admin 用户。
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"deleted\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"不能删除最后一个 admin 用户\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"用户不存在\"}"
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
			if errors.Is(err, errors.New("cannot delete the last admin")) {
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
