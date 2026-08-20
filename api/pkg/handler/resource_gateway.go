package handler

import (
	"errors"
	"net/http"

	"github.com/fize/kumquat/api/pkg/middleware"
	"github.com/fize/kumquat/api/pkg/model"
	"github.com/fize/kumquat/api/pkg/service"
	"github.com/fize/kumquat/api/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ResourceGatewayController exposes API-owned business DTOs. Engine API types
// intentionally do not cross this HTTP boundary.
type ResourceGatewayController struct{ svc *service.ResourceGateway }

func NewResourceGatewayController(svc *service.ResourceGateway) *ResourceGatewayController {
	return &ResourceGatewayController{svc: svc}
}

func (c *ResourceGatewayController) Register(api *gin.RouterGroup, auth gin.HandlerFunc, authorize func(string, string) gin.HandlerFunc) {
	for _, resource := range []struct{ path, kind, permission string }{{"workspaces", model.ResourceWorkspace, model.ResourceWorkspace}, {"applications", model.ResourceApplication, model.ResourceApplication}} {
		group := api.Group("/"+resource.path, auth, c.Principal())
		group.GET("", authorize(resource.permission, model.ActionRead), c.list(resource.kind))
		group.POST("", authorize(resource.permission, model.ActionWrite), c.create(resource.kind))
		group.GET("/:id", authorize(resource.permission, model.ActionRead), c.get(resource.kind))
		group.PUT("/:id", authorize(resource.permission, model.ActionWrite), c.update(resource.kind))
		group.DELETE("/:id", authorize(resource.permission, model.ActionWrite), c.delete(resource.kind))
		group.POST("/:id/adopt-existing", authorize(resource.permission, model.ActionWrite), c.adoptExisting(resource.kind))
	}
	clusters := api.Group("/clusters", auth, c.Principal())
	clusters.GET("", authorize(model.ResourceCluster, model.ActionRead), c.list(model.ResourceCluster))
	clusters.GET("/:id", authorize(model.ResourceCluster, model.ActionRead), c.get(model.ResourceCluster))
	clusters.POST("/:id/adopt", authorize(model.ResourceCluster, model.ActionWrite), c.adopt)
	api.GET("/operations/:id", auth, c.Principal(), c.operation)
}

func (c *ResourceGatewayController) adoptExisting(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req struct {
			ProjectID   string `json:"projectId"`
			WorkspaceID string `json:"workspaceId"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}
		in := service.ResourceInput{ProjectID: req.ProjectID, WorkspaceID: req.WorkspaceID}
		op, err := c.svc.AdoptExisting(ctx, ctx.Param("id"), kind, in, ctx.GetHeader("Idempotency-Key"))
		if err != nil {
			c.writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusAccepted, utils.Response{Code: 0, Message: "accepted", Data: op})
	}
}

// Principal derives the durable business scope exactly once per request.
func (c *ResourceGatewayController) Principal() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		p, err := c.svc.PrincipalForUser(ctx, middleware.GetUserID(ctx), middleware.GetRoleName(ctx))
		if err != nil {
			if errors.Is(err, service.ErrForbidden) {
				utils.Forbidden(ctx, err.Error())
			} else {
				utils.InternalError(ctx, "failed to resolve actor scope")
			}
			ctx.Abort()
			return
		}
		ctx.Request = ctx.Request.WithContext(service.ContextWithPrincipal(ctx.Request.Context(), p))
		ctx.Set(middleware.ContextKeyRoleID, p.RoleID)
		ctx.Set(middleware.ContextKeyRoleName, p.RoleName)
		ctx.Next()
	}
}

func (c *ResourceGatewayController) list(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		items, err := c.svc.List(ctx, kind)
		if err != nil {
			utils.InternalError(ctx, err.Error())
			return
		}
		utils.Success(ctx, gin.H{"items": items})
	}
}
func (c *ResourceGatewayController) get(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		item, err := c.svc.Get(ctx, ctx.Param("id"), kind)
		if err != nil {
			c.writeError(ctx, err)
			return
		}
		utils.Success(ctx, item)
	}
}
func (c *ResourceGatewayController) create(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var in service.ResourceInput
		if err := ctx.ShouldBindJSON(&in); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}
		op, err := c.svc.Create(ctx, kind, in, ctx.GetHeader("Idempotency-Key"))
		if err != nil {
			c.writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusAccepted, utils.Response{Code: 0, Message: "accepted", Data: op})
	}
}
func (c *ResourceGatewayController) update(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var in service.ResourceInput
		if err := ctx.ShouldBindJSON(&in); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}
		op, err := c.svc.Update(ctx, ctx.Param("id"), kind, in, ctx.GetHeader("Idempotency-Key"))
		if err != nil {
			c.writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusAccepted, utils.Response{Code: 0, Message: "accepted", Data: op})
	}
}
func (c *ResourceGatewayController) delete(kind string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		op, err := c.svc.Delete(ctx, ctx.Param("id"), kind, ctx.GetHeader("Idempotency-Key"))
		if err != nil {
			c.writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusAccepted, utils.Response{Code: 0, Message: "accepted", Data: op})
	}
}
func (c *ResourceGatewayController) adopt(ctx *gin.Context) {
	op, err := c.svc.AdoptCluster(ctx, ctx.Param("id"), ctx.GetHeader("Idempotency-Key"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, utils.Response{Code: 0, Message: "accepted", Data: op})
}
func (c *ResourceGatewayController) operation(ctx *gin.Context) {
	op, err := c.svc.Operation(ctx, ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	utils.Success(ctx, op)
}
func (c *ResourceGatewayController) writeError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrForbidden):
		utils.Forbidden(ctx, err.Error())
	case errors.Is(err, service.ErrIdempotencyConflict):
		utils.Conflict(ctx, err.Error())
	case errors.Is(err, service.ErrParentImmutable), errors.Is(err, service.ErrWorkspaceNamespaceInUse), errors.Is(err, service.ErrConcurrentModification), errors.Is(err, service.ErrParentUnavailable):
		utils.Conflict(ctx, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.NotFound(ctx, "resource not found")
	case contains(err.Error(), "immutable"), contains(err.Error(), "cannot delete"), contains(err.Error(), "awaiting adoption"), contains(err.Error(), "unsupported Engine"):
		utils.Conflict(ctx, err.Error())
	default:
		utils.BadRequest(ctx, err.Error())
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
