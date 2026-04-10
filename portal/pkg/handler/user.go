package handler

import (
	"strconv"

	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// Register 注册路由
func (h *UserHandler) Register(api *gin.RouterGroup) {
	users := api.Group("/users")
	users.Use(middleware.Auth(), middleware.RequireRole("admin"))
	{
		users.GET("", h.List)
		users.GET("/:id", h.Get)
		users.POST("", h.Create)
		users.PUT("/:id", h.Update)
		users.DELETE("/:id", h.Delete)
	}
}

func (h *UserHandler) List(c *gin.Context) {
	page, size := utils.GetPageSize(c)
	users, total, err := h.userService.List(page, size)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	list := make([]map[string]interface{}, len(users))
	for i, u := range users {
		list[i] = u.ToResponse()
	}
	utils.PageSuccess(c, total, page, size, list)
}

func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	user, err := h.userService.GetByID(uint(id))
	if err != nil {
		utils.NotFound(c, "user not found")
		return
	}
	utils.Success(c, user.ToResponse())
}

func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=32"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6,max=32"`
		Nickname string `json:"nickname"`
		RoleID   uint   `json:"role_id" binding:"required"`
		ModuleID *uint  `json:"module_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.userService.Create(req.Username, req.Email, req.Password, req.Nickname, req.RoleID, req.ModuleID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, user.ToResponse())
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Nickname string `json:"nickname"`
		RoleID   uint   `json:"role_id"`
		ModuleID *uint  `json:"module_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.userService.Update(uint(id), req.Nickname, req.RoleID, req.ModuleID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, user.ToResponse())
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.userService.Delete(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "deleted", nil)
}
