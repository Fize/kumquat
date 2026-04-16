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

// CreateModuleRequest 创建模块请求
// swagger:model
type CreateModuleRequest struct {
	Name     string `json:"name" binding:"required" example:"infrastructure"`
	ParentID *uint  `json:"parent_id" example:"1"`
	Sort     int    `json:"sort" example:"0"`
}

// UpdateModuleRequest 更新模块请求
// swagger:model
type UpdateModuleRequest struct {
	Name string `json:"name" example:"infrastructure-updated"`
	Sort int    `json:"sort" example:"1"`
}

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

// List 获取模块列表
// @Summary 获取模块列表（树形结构）
// @Description 获取所有模块的树形结构列表，需要 module:read 权限
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":[{module_tree}]}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Router /modules [get]
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

// Get 获取单个模块
// @Summary 根据 ID 获取模块信息
// @Description 获取指定 ID 的模块详情
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"无效的模块ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"模块不存在\"}"
// @Router /modules/{id} [get]
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

// Create 创建模块
// @Summary 创建新模块
// @Description 创建新模块，仅 admin 角色可操作。支持最多 5 级层级。
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateModuleRequest true "创建模块请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"请求参数错误或层级超限\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
// @Router /modules [post]
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

// Update 更新模块
// @Summary 更新模块信息
// @Description 更新指定模块的信息，仅 admin 角色可操作
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "模块ID"
// @Param request body UpdateModuleRequest true "更新模块请求"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"无效的模块ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"模块不存在\"}"
// @Router /modules/{id} [put]
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

// Delete 删除模块
// @Summary 删除模块
// @Description 删除指定模块及其所有子模块，仅 admin 角色可操作
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"deleted\"}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"无效的模块ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"未授权\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"无权限\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"模块不存在\"}"
// @Router /modules/{id} [delete]
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
