package handler

import (
	"errors"

	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/apiserver/pkg/dto"
	"github.com/fize/kumquat/apiserver/pkg/middleware"
	"github.com/fize/kumquat/apiserver/pkg/model"
	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/fize/kumquat/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
)

// CreateModuleRequest represents create module request
// swagger:model
type CreateModuleRequest struct {
	Name     string `json:"name" binding:"required" example:"infrastructure"`
	ParentID *uint  `json:"parent_id" example:"1"`
	Sort     int    `json:"sort" example:"0"`
}

// UpdateModuleRequest represents update module request
// swagger:model
type UpdateModuleRequest struct {
	Name string `json:"name" example:"infrastructure-updated"`
	Sort int    `json:"sort" example:"1"`
}

// ModuleController implements RestController interface
type ModuleController struct {
	svc            *service.ModuleService
	rs             *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewModuleController creates a new module controller
func NewModuleController(moduleSvc *service.ModuleService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ModuleController {
	return &ModuleController{svc: moduleSvc, rs: roleSvc, authMiddleware: authMiddleware}
}

func (c *ModuleController) Name() string    { return "modules" }
func (c *ModuleController) Version() string { return "v1" }

func (c *ModuleController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"get", "list"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.rs, "module", "read")},
		},
		{
			Methods:     []string{"create", "update", "delete"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequireRole("admin")},
		},
	}
}

// List retrieves module list
// @Summary Get module list (tree structure)
// @Description Get tree-structured module list, requires module:read permission
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":[{module_tree}]}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Router /modules [get]
func (c *ModuleController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		principal := businessPrincipal(ctx)
		var modules []model.Module
		var err error
		if principal.Admin {
			modules, err = c.svc.List(ctx.Request.Context())
		} else {
			modules, err = c.svc.ListScoped(ctx.Request.Context(), principal.ModulePublicID)
		}
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "list modules failed", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}
		list := make([]dto.ModuleDTO, len(modules))
		for i, m := range modules {
			list[i] = dto.ModuleFromModel(m)
		}
		utils.Success(ctx, list)
	}, nil
}

// Get retrieves a single module
// @Summary Get module information by ID
// @Description Get module details by specified ID
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "module ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid module ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"module not found\"}"
// @Router /modules/{id} [get]
func (c *ModuleController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		principal := businessPrincipal(ctx)
		if !principal.Admin {
			allowed, scopeErr := c.svc.CanAccess(ctx.Request.Context(), principal.ModulePublicID, id)
			if !requireScopedAccess(ctx, allowed, scopeErr) {
				return
			}
		}
		module, err := c.svc.GetByPublicID(ctx.Request.Context(), id)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get module failed", "id", id, "err", err)
			utils.NotFound(ctx, "module not found")
			return
		}
		utils.Success(ctx, dto.ModuleFromModel(*module))
	}, nil
}

// Create creates a module
// @Summary Create new module
// @Description Create new module, only admin role can perform. Supports up to 5 levels of hierarchy.
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateModuleRequest true "create module request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid request parameters or hierarchy exceeded\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Router /modules [post]
func (c *ModuleController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req struct {
			Name     string  `json:"name" binding:"required"`
			ParentID *string `json:"parentId"`
			Sort     int     `json:"sort"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create module request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		module, err := c.svc.CreateWithParentPublicID(ctx.Request.Context(), req.Name, req.ParentID, req.Sort)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "create module failed", "name", req.Name, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "module created", "module_id", module.ID, "name", module.Name)
		utils.Success(ctx, module.ToResponse())
	}, nil
}

// Update updates module
// @Summary Update module information
// @Description Update specified module information, only admin role can perform
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "module ID"
// @Param request body UpdateModuleRequest true "update module request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{module}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid module ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"module not found\"}"
// @Router /modules/{id} [put]
func (c *ModuleController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		var req struct {
			Name     string  `json:"name"`
			ParentID *string `json:"parentId"`
			Sort     int     `json:"sort"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "update module request validation failed", "id", id, "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		if req.ParentID != nil {
			current, getErr := c.svc.GetByPublicID(ctx.Request.Context(), id)
			if getErr != nil {
				utils.ErrorFromErr(ctx, getErr)
				return
			}
			if current.Parent == nil || current.Parent.PublicID != *req.ParentID {
				utils.Conflict(ctx, "module parent association is immutable")
				return
			}
		}
		module, err := c.svc.UpdateByPublicID(ctx.Request.Context(), id, req.Name, req.Sort)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update module failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "module updated", "module_id", module.ID)
		utils.Success(ctx, module.ToResponse())
	}, nil
}

// Delete deletes module
// @Summary Delete module
// @Description Delete specified module and all its submodules, only admin role can perform
// @Tags modules
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "module ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"deleted\"}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid module ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"module not found\"}"
// @Router /modules/{id} [delete]
func (c *ModuleController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		if err := c.svc.DeleteByPublicID(ctx.Request.Context(), id); err != nil {
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
