package handler

import (
	"strconv"

	"github.com/fize/go-ext/log"
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

// SetupRoutes 注册路由
func (h *UserHandler) SetupRoutes(api *gin.RouterGroup) {
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
		log.ErrorContext(c.Request.Context(), "list users failed", "err", err)
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
		log.WarnContext(c.Request.Context(), "get user failed", "id", id, "err", err)
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
		log.WarnContext(c.Request.Context(), "create user request validation failed", "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.userService.Create(req.Username, req.Email, req.Password, req.Nickname, req.RoleID, req.ModuleID)
	if err != nil {
		log.WarnContext(c.Request.Context(), "create user failed", "username", req.Username, "err", err)
		utils.Conflict(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "user created", "user_id", user.ID, "username", user.Username)
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
		log.WarnContext(c.Request.Context(), "update user request validation failed", "id", id, "err", err)
		utils.BadRequest(c, err.Error())
		return
	}
	user, err := h.userService.Update(uint(id), req.Nickname, req.RoleID, req.ModuleID)
	if err != nil {
		log.WarnContext(c.Request.Context(), "update user failed", "id", id, "err", err)
		utils.NotFound(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "user updated", "user_id", user.ID)
	utils.Success(c, user.ToResponse())
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.userService.Delete(uint(id)); err != nil {
		log.WarnContext(c.Request.Context(), "delete user failed", "id", id, "err", err)
		utils.Forbidden(c, err.Error())
		return
	}
	log.InfoContext(c.Request.Context(), "user deleted", "user_id", id)
	utils.SuccessWithMessage(c, "deleted", nil)
}
