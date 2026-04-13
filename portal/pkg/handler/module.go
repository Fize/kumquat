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

// ModuleController 模块控制器，实现 RestController 接口
type ModuleController struct {
	svc           *service.ModuleService
	rs            *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewModuleController 创建模块控制器
func NewModuleController(moduleSvc *service.ModuleService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ModuleController {
	return &ModuleController{svc: moduleSvc, rs: roleSvc, authMiddleware: authMiddleware}
}

func (c *ModuleController) Name() string { return "modules" }
func (c *ModuleController) Version() string { return "v1" }

func (c *ModuleController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"GET"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.rs, "module", "read")},
		},
		{
			Methods:     []string{"POST", "PUT", "DELETE"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequireRole("admin")},
		},
	}
}

func (c *ModuleController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		modules, err := c.svc.List(ctx.Request.Context())
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "list modules failed", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}
		list := make([]map[string]interface{}, len(modules))
		for i, m := range modules {
			list[i] = m.ToResponse()
		}
		utils.Success(ctx, list)
	}, nil
}

func (c *ModuleController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		module, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get module failed", "id", id, "err", err)
			utils.NotFound(ctx, "module not found")
			return
		}
		utils.Success(ctx, module.ToResponse())
	}, nil
}

func (c *ModuleController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req struct {
			Name     string `json:"name" binding:"required"`
			ParentID *uint  `json:"parent_id"`
			Sort     int    `json:"sort"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create module request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		module, err := c.svc.Create(ctx.Request.Context(), req.Name, req.ParentID, req.Sort)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "create module failed", "name", req.Name, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "module created", "module_id", module.ID, "name", module.Name)
		utils.Success(ctx, module.ToResponse())
	}, nil
}

func (c *ModuleController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		var req struct {
			Name string `json:"name"`
			Sort int    `json:"sort"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "update module request validation failed", "id", id, "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		module, err := c.svc.Update(ctx.Request.Context(), uint(id), req.Name, req.Sort)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update module failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "module updated", "module_id", module.ID)
		utils.Success(ctx, module.ToResponse())
	}, nil
}

func (c *ModuleController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
			log.WarnContext(ctx.Request.Context(), "delete module failed", "id", id, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "module deleted", "module_id", id)
		utils.SuccessWithMessage(ctx, "deleted", nil)
	}, nil
}

func (c *ModuleController) Patch() (gin.HandlerFunc, error) {
	return nil, errors.New("patch not implemented")
}
