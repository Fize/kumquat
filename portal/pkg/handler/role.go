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
