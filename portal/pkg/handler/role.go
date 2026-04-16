package handler

import (
	"strconv"

	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// RoleController 角色控制器，实现 RestController 接口
type RoleController struct {
	svc           *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewRoleController 创建角色控制器
func NewRoleController(roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *RoleController {
	return &RoleController{svc: roleSvc, authMiddleware: authMiddleware}
}

func (c *RoleController) Name() string { return "roles" }
func (c *RoleController) Version() string { return "v1" }

func (c *RoleController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"GET"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.svc, "role", "read")},
		},
	}
}

// List 获取角色列表
// @Summary 获取角色列表
// @Description 获取所有角色列表，需要 role:read 权限
// @Tags roles
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":[{role}]}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Router /roles [get]
func (c *RoleController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		roles, err := c.svc.List(ctx.Request.Context())
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "list roles failed", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}
		list := make([]map[string]interface{}, len(roles))
		for i, r := range roles {
			list[i] = r.ToResponse()
		}
		utils.Success(ctx, list)
	}, nil
}

// Get 获取单个角色
// @Summary 根据 ID 获取角色信息
// @Description 获取指定 ID 的角色详情
// @Tags roles
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "角色ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{role}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"无效的角色ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"角色不存在\"}"
// @Router /roles/{id} [get]
func (c *RoleController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		role, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get role failed", "id", id, "err", err)
			utils.NotFound(ctx, "role not found")
			return
		}
		utils.Success(ctx, role.ToResponse())
	}, nil
}

func (c *RoleController) Create() (gin.HandlerFunc, error) { return nil, nil }
func (c *RoleController) Update() (gin.HandlerFunc, error) { return nil, nil }
func (c *RoleController) Delete() (gin.HandlerFunc, error) { return nil, nil }
func (c *RoleController) Patch() (gin.HandlerFunc, error) { return nil, nil }
