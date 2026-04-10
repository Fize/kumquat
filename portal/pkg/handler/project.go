package handler

import (
	"strconv"

	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ProjectHandler 项目处理器
type ProjectHandler struct {
	projectService *service.ProjectService
}

// NewProjectHandler 创建项目处理器
func NewProjectHandler(projectService *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService}
}

// Register 注册路由
func (h *ProjectHandler) Register(api *gin.RouterGroup) {
	projects := api.Group("/projects")
	projects.Use(middleware.Auth())
	{
		projects.GET("", h.List)
		projects.GET("/:id", h.Get)
		projects.GET("/module/:moduleId", h.ListByModule)
		projects.POST("", middleware.RequireRole("admin"), h.Create)
		projects.PUT("/:id", middleware.RequireRole("admin"), h.Update)
		projects.DELETE("/:id", middleware.RequireRole("admin"), h.Delete)
	}
}

func (h *ProjectHandler) List(c *gin.Context) {
	page, size := utils.GetPageSize(c)
	projects, total, err := h.projectService.List(page, size)
	if err != nil {
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
		ModuleID uint            `json:"module_id" binding:"required"`
		Config   model.JSONConfig `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	project, err := h.projectService.Create(req.Name, req.ModuleID, req.Config)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
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
		utils.BadRequest(c, err.Error())
		return
	}
	project, err := h.projectService.Update(uint(id), req.Name, req.Config)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, project.ToResponse())
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.projectService.Delete(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "deleted", nil)
}
