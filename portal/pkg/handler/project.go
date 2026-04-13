package handler

import (
	"strconv"

	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ProjectHandler 项目处理器
type ProjectHandler struct {
	projectService *service.ProjectService
	roleService    *service.RoleService
}

// NewProjectHandler 创建项目处理器
func NewProjectHandler(projectService *service.ProjectService, roleService *service.RoleService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService, roleService: roleService}
}

// SetupRoutes 注册路由
func (h *ProjectHandler) SetupRoutes(api *gin.RouterGroup) {
	projects := api.Group("/projects")
	projects.Use(middleware.Auth())
	{
		projects.GET("", middleware.RequirePermission(h.roleService, "project", "read"), h.List)
		projects.GET("/:id", middleware.RequirePermission(h.roleService, "project", "read"), h.Get)
		projects.GET("/module/:moduleId", middleware.RequirePermission(h.roleService, "project", "read"), h.ListByModule)
		projects.POST("", middleware.RequireRole("admin"), h.Create)
		projects.PUT("/:id", middleware.RequireRole("admin"), h.Update)
		projects.DELETE("/:id", middleware.RequireRole("admin"), h.Delete)
	}
}

func (h *ProjectHandler) List(c *gin.Context) {
	page, size := utils.GetPageSize(c)
	projects, total, err := h.projectService.List(page, size)
	if err != nil {
		log.ErrorContext(c.Request.Context(), "list projects failed", "err", err)
		utils.InternalError(c, err.Error())
		return
	}
	list := make([]map[string]interface{}, len(projects))
	for i, p := range projects {
		list[i] = p.ToResponse()
	}
	utils.PageSuccess(c, total, page, size, list)
}

func (h *ProjectHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	project, err := h.projectService.GetByID(uint(id))
	if err != nil {
		log.WarnContext(c.Request.Context(), "get project failed", "id", id, "err", err)
		utils.NotFound(c, "project not found")
		return
	}
	utils.Success(c, project.ToResponse())
}

func (h *ProjectHandler) ListByModule(c *gin.Context) {
	moduleId, err := strconv.ParseUint(c.Param("moduleId"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid module id")
		return
	}
	page, size := utils.GetPageSize(c)
	projects, total, err := h.projectService.ListByModule(uint(moduleId), page, size)
	if err != nil {
		log.ErrorContext(c.Request.Context(), "list projects by module failed", "module_id", moduleId, "err", err)
		utils.InternalError(c, err.Error())
		return
	}
	list := make([]map[string]interface{}, len(projects))
	for i, p := range projects {
		list[i] = p.ToResponse()
	}
	utils.PageSuccess(c, total, page, size, list)
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var req struct {
		Name     string           `json:"name" binding:"required"`
		ModuleID uint             `json:"module_id" binding:"required"`
		Config   model.JSONConfig `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WarnContext(c.Request.Context(), "create project request validation failed", "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	project, err := h.projectService.Create(req.Name, req.ModuleID, req.Config)
	if err != nil {
		log.WarnContext(c.Request.Context(), "create project failed", "name", req.Name, "module_id", req.ModuleID, "err", err)
		utils.Conflict(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "project created", "project_id", project.ID, "name", project.Name)
	utils.Success(c, project.ToResponse())
}

func (h *ProjectHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Name   string           `json:"name"`
		Config model.JSONConfig `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WarnContext(c.Request.Context(), "update project request validation failed", "id", id, "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	project, err := h.projectService.Update(uint(id), req.Name, req.Config)
	if err != nil {
		log.WarnContext(c.Request.Context(), "update project failed", "id", id, "err", err)
		utils.NotFound(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "project updated", "project_id", project.ID)
	utils.Success(c, project.ToResponse())
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.projectService.Delete(uint(id)); err != nil {
		log.WarnContext(c.Request.Context(), "delete project failed", "id", id, "err", err)
		utils.NotFound(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "project deleted", "project_id", id)
	utils.SuccessWithMessage(c, "deleted", nil)
}
