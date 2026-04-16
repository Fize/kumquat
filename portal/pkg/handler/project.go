package handler

import (
	"strconv"

	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
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
	svc           *service.ProjectService
	rs            *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewProjectController creates a new project controller
func NewProjectController(projectSvc *service.ProjectService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ProjectController {
	return &ProjectController{svc: projectSvc, rs: roleSvc, authMiddleware: authMiddleware}
}

func (c *ProjectController) Name() string { return "projects" }
func (c *ProjectController) Version() string { return "v1" }

func (c *ProjectController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"GET"},
			Middlewares: []gin.HandlerFunc{c.authMiddleware.Auth(), middleware.RequirePermission(c.rs, "project", "read")},
		},
		{
			Methods:     []string{"POST", "PUT", "DELETE"},
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
		projects, total, err := c.svc.List(ctx.Request.Context(), page, size)
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
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		project, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
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
			ModuleID uint                   `json:"module_id" binding:"required"`
			Config   map[string]interface{} `json:"config"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create project request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		// Convert to model.JSONConfig type to ensure correct database serialization
		config := model.JSONConfig(req.Config)
		project, err := c.svc.Create(ctx.Request.Context(), req.Name, req.ModuleID, config)
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
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		var req struct {
			Name   string           `json:"name"`
			Config model.JSONConfig `json:"config"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "update project request validation failed", "id", id, "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		project, err := c.svc.Update(ctx.Request.Context(), uint(id), req.Name, req.Config)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update project failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
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
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
			log.WarnContext(ctx.Request.Context(), "delete project failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project deleted", "project_id", id)
		utils.SuccessWithMessage(ctx, "deleted", nil)
	}, nil
}

func (c *ProjectController) Patch() (gin.HandlerFunc, error) { return nil, nil }
