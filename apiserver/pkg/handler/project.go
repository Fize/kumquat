package handler

import (
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/apiserver/pkg/middleware"
	"github.com/fize/kumquat/apiserver/pkg/model"
	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/fize/kumquat/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
)

// CreateProjectRequest represents create project request
// swagger:model
type CreateProjectRequest struct {
	Name     string                 `json:"name" binding:"required" example:"my-project"`
	ModuleID uint                   `json:"module_id" binding:"required" example:"1"`
	Config   map[string]interface{} `json:"config" example:"{\"key\":\"value\"}"`
}

// UpdateProjectRequest represents update project request
// swagger:model
type UpdateProjectRequest struct {
	Name   string                 `json:"name" example:"my-project-updated"`
	Config map[string]interface{} `json:"config" example:"{\"key\":\"new-value\"}"`
}

// ProjectController implements RestController interface
type ProjectController struct {
	svc            *service.ProjectService
	rs             *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewProjectController creates a new project controller
func NewProjectController(projectSvc *service.ProjectService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ProjectController {
	return &ProjectController{svc: projectSvc, rs: roleSvc, authMiddleware: authMiddleware}
}

func (c *ProjectController) Name() string    { return "projects" }
func (c *ProjectController) Version() string { return "v1" }

func (c *ProjectController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"get", "list"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.rs, "project", "read")},
		},
		{
			Methods:     []string{"create", "update", "delete"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequireRole("admin")},
		},
	}
}

// List retrieves project list
// @Summary Get project list (paginated)
// @Description Get paginated project list, requires project:read permission
// @Tags projects
// @Accept json
// @Produce json
// @Security Bearer
// @Param page query int false "page number" default(1)
// @Param size query int false "page size" default(10)
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":[{project}],\"pagination\":{...}}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Router /projects [get]
func (c *ProjectController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		page, size := utils.GetPageSize(ctx)
		principal := businessPrincipal(ctx)
		var projects []model.Project
		var total int64
		var err error
		if principal.Admin {
			projects, total, err = c.svc.List(ctx.Request.Context(), page, size)
		} else {
			projects, total, err = c.svc.ListScoped(ctx.Request.Context(), principal.ModulePublicID, page, size)
		}
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "list projects failed", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}
		list := make([]map[string]interface{}, len(projects))
		for i, p := range projects {
			list[i] = p.ToResponse()
		}
		utils.PageSuccess(ctx, total, page, size, list)
	}, nil
}

// Get retrieves a single project
// @Summary Get project information by ID
// @Description Get project details by specified ID
// @Tags projects
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "project ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{project}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid project ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"project not found\"}"
// @Router /projects/{id} [get]
func (c *ProjectController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		principal := businessPrincipal(ctx)
		if !principal.Admin {
			allowed, scopeErr := c.svc.CanAccess(ctx.Request.Context(), principal.ModulePublicID, id)
			if !requireScopedAccess(ctx, allowed, scopeErr) {
				return
			}
		}
		project, err := c.svc.GetByPublicID(ctx.Request.Context(), id)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get project failed", "id", id, "err", err)
			utils.NotFound(ctx, "project not found")
			return
		}
		utils.Success(ctx, project.ToResponse())
	}, nil
}

// Create creates a project
// @Summary Create new project
// @Description Create new project, only admin role can perform
// @Tags projects
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateProjectRequest true "create project request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{project}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid request parameters\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Router /projects [post]
func (c *ProjectController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req struct {
			Name     string                 `json:"name" binding:"required"`
			ModuleID string                 `json:"moduleId"`
			Config   map[string]interface{} `json:"config"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create project request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		// Convert to model.JSONConfig type to ensure correct database serialization
		if req.ModuleID == "" {
			utils.BadRequest(ctx, "moduleId is required")
			return
		}
		config := model.JSONConfig(req.Config)
		project, err := c.svc.CreateWithModulePublicID(ctx.Request.Context(), req.Name, req.ModuleID, config)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "create project failed", "name", req.Name, "module_id", req.ModuleID, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project created", "project_id", project.ID, "name", project.Name)
		utils.Success(ctx, project.ToResponse())
	}, nil
}

// Update updates project
// @Summary Update project information
// @Description Update specified project information, only admin role can perform
// @Tags projects
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "project ID"
// @Param request body UpdateProjectRequest true "update project request"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"data\":{project}}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid project ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"project not found\"}"
// @Router /projects/{id} [put]
func (c *ProjectController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		var req struct {
			Name     string           `json:"name"`
			ModuleID string           `json:"moduleId"`
			Config   model.JSONConfig `json:"config"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "update project request validation failed", "id", id, "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		project, err := c.svc.UpdateByPublicID(ctx.Request.Context(), id, req.Name, req.ModuleID, req.Config)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update project failed", "id", id, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}
		log.InfoContext(ctx.Request.Context(), "project updated", "project_id", project.ID)
		utils.Success(ctx, project.ToResponse())
	}, nil
}

// Delete deletes project
// @Summary Delete project
// @Description Delete specified project, only admin role can perform
// @Tags projects
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "project ID"
// @Success 200 {object} map[string]interface{} "{\"code\":0,\"message\":\"deleted\"}"
// @Failure 400 {object} map[string]interface{} "{\"code\":400,\"message\":\"invalid project ID\"}"
// @Failure 401 {object} map[string]interface{} "{\"code\":401,\"message\":\"unauthorized\"}"
// @Failure 403 {object} map[string]interface{} "{\"code\":403,\"message\":\"no permission\"}"
// @Failure 404 {object} map[string]interface{} "{\"code\":404,\"message\":\"project not found\"}"
// @Router /projects/{id} [delete]
func (c *ProjectController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id := ctx.Param("id")
		if err := c.svc.DeleteByPublicID(ctx.Request.Context(), id); err != nil {
			log.WarnContext(ctx.Request.Context(), "delete project failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project deleted", "project_id", id)
		utils.SuccessWithMessage(ctx, "deleted", nil)
	}, nil
}

func (c *ProjectController) Patch() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) { utils.Error(ctx, 405, 405, "patch not supported for projects") }, nil
}
