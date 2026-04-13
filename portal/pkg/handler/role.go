package handler

import (
	"strconv"

	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// RoleHandler 角色处理器
type RoleHandler struct {
	roleService *service.RoleService
}

// NewRoleHandler 创建角色处理器
func NewRoleHandler(roleService *service.RoleService) *RoleHandler {
	return &RoleHandler{roleService: roleService}
}

// SetupRoutes 注册路由
func (h *RoleHandler) SetupRoutes(api *gin.RouterGroup) {
	roles := api.Group("/roles")
	roles.Use(middleware.Auth(), middleware.RequireRole("admin"))
	{
		roles.GET("", h.List)
		roles.GET("/:id", h.Get)
		roles.GET("/:id/permissions", h.GetPermissions)
	}
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roleService.List()
	if err != nil {
		log.ErrorContext(c.Request.Context(), "list roles failed", "err", err)
		utils.InternalError(c, err.Error())
		return
	}
	list := make([]map[string]interface{}, len(roles))
	for i, r := range roles {
		list[i] = r.ToResponse()
	}
	utils.Success(c, list)
}

func (h *RoleHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	role, err := h.roleService.GetByID(uint(id))
	if err != nil {
		log.WarnContext(c.Request.Context(), "get role failed", "id", id, "err", err)
		utils.NotFound(c, "role not found")
		return
	}
	utils.Success(c, role.ToResponse())
}

func (h *RoleHandler) GetPermissions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	perms, err := h.roleService.GetPermissions(uint(id))
	if err != nil {
		log.WarnContext(c.Request.Context(), "get role permissions failed", "id", id, "err", err)
		utils.NotFound(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"permissions": perms})
}
