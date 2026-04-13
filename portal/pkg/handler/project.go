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

// ProjectController 项目控制器，实现 RestController 接口
type ProjectController struct {
	svc *service.ProjectService
	rs  *service.RoleService
}

// NewProjectController 创建项目控制器
func NewProjectController(projectSvc *service.ProjectService, roleSvc *service.RoleService) *ProjectController {
	return &ProjectController{svc: projectSvc, rs: roleSvc}
}

func (c *ProjectController) Name() string { return "projects" }
func (c *ProjectController) Version() string { return "v1" }

func (c *ProjectController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods:     []string{"GET"},
			Middlewares: []gin.HandlerFunc{middleware.Auth(), middleware.RequirePermission(c.rs, "project", "read")},
		},
		{
			Methods:     []string{"POST", "PUT", "DELETE"},
			Middlewares: []gin.HandlerFunc{middleware.Auth(), middleware.RequireRole("admin")},
		},
	}
}

func (c *ProjectController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		page, size := utils.GetPageSize(ctx)
		projects, total, err := c.svc.List(page, size)
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

func (c *ProjectController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		project, err := c.svc.GetByID(uint(id))
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "get project failed", "id", id, "err", err)
			utils.NotFound(ctx, "project not found")
			return
		}
		utils.Success(ctx, project.ToResponse())
	}, nil
}

func (c *ProjectController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req struct {
			Name     string           `json:"name" binding:"required"`
			ModuleID uint             `json:"module_id" binding:"required"`
			Config   model.JSONConfig `json:"config"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			log.WarnContext(ctx.Request.Context(), "create project request validation failed", "err", err)
			utils.BadRequest(ctx, err.Error())
			return
		}
		project, err := c.svc.Create(req.Name, req.ModuleID, req.Config)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "create project failed", "name", req.Name, "module_id", req.ModuleID, "err", err)
			utils.Conflict(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project created", "project_id", project.ID, "name", project.Name)
		utils.Success(ctx, project.ToResponse())
	}, nil
}

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
		project, err := c.svc.Update(uint(id), req.Name, req.Config)
		if err != nil {
			log.WarnContext(ctx.Request.Context(), "update project failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project updated", "project_id", project.ID)
		utils.Success(ctx, project.ToResponse())
	}, nil
}

func (c *ProjectController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if err != nil {
			utils.BadRequest(ctx, "invalid id")
			return
		}
		if err := c.svc.Delete(uint(id)); err != nil {
			log.WarnContext(ctx.Request.Context(), "delete project failed", "id", id, "err", err)
			utils.NotFound(ctx, err.Error())
			return
		}
		log.InfoContext(ctx.Request.Context(), "project deleted", "project_id", id)
		utils.SuccessWithMessage(ctx, "deleted", nil)
	}, nil
}

func (c *ProjectController) Patch() (gin.HandlerFunc, error) { return nil, nil }
