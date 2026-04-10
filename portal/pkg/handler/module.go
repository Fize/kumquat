package handler

import (
	"strconv"

	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ModuleHandler 模块处理器
type ModuleHandler struct {
	moduleService *service.ModuleService
}

// NewModuleHandler 创建模块处理器
func NewModuleHandler(moduleService *service.ModuleService) *ModuleHandler {
	return &ModuleHandler{moduleService: moduleService}
}

// Register 注册路由
func (h *ModuleHandler) Register(api *gin.RouterGroup) {
	modules := api.Group("/modules")
	modules.Use(middleware.Auth())
	{
		modules.GET("", h.List)
		modules.GET("/:id", h.Get)
		modules.GET("/:id/children", h.GetChildren)
		modules.POST("", middleware.RequireRole("admin"), h.Create)
		modules.PUT("/:id", middleware.RequireRole("admin"), h.Update)
		modules.DELETE("/:id", middleware.RequireRole("admin"), h.Delete)
	}
}

func (h *ModuleHandler) List(c *gin.Context) {
	modules, err := h.moduleService.List()
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	list := make([]map[string]interface{}, len(modules))
	for i, m := range modules {
		list[i] = m.ToResponse()
	}
	utils.Success(c, list)
}

func (h *ModuleHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	module, err := h.moduleService.GetByID(uint(id))
	if err != nil {
		utils.NotFound(c, "module not found")
		return
	}
	utils.Success(c, module.ToResponse())
}

func (h *ModuleHandler) GetChildren(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	module, err := h.moduleService.GetByID(uint(id))
	if err != nil {
		utils.NotFound(c, "module not found")
		return
	}
	utils.Success(c, module.Children)
}

func (h *ModuleHandler) Create(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		ParentID *uint  `json:"parent_id"`
		Sort     int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	module, err := h.moduleService.Create(req.Name, req.ParentID, req.Sort)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, module.ToResponse())
}

func (h *ModuleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	module, err := h.moduleService.Update(uint(id), req.Name, req.Sort)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, module.ToResponse())
}

func (h *ModuleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.moduleService.Delete(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "deleted", nil)
}
