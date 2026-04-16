package handler

import (
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/engine/pkg/apis/cluster/v1alpha1"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ClusterController handles cluster management requests
type ClusterController struct {
	svc            *service.ClusterService
	roleSvc        *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewClusterController creates a new cluster controller
func NewClusterController(svc *service.ClusterService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ClusterController {
	return &ClusterController{
		svc:            svc,
		roleSvc:        roleSvc,
		authMiddleware: authMiddleware,
	}
}

// Name returns the controller name
func (c *ClusterController) Name() string {
	return "clusters"
}

// Version returns the API version
func (c *ClusterController) Version() string {
	return "v1"
}

// Middlewares returns the middlewares for this controller
// Cluster management is only accessible to Admin
func (c *ClusterController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods: []string{"*"}, // All methods require Admin permission
			Middlewares: []gin.HandlerFunc{
				c.authMiddleware.Auth(),
				middleware.RequireRole(model.RoleAdmin),
			},
		},
	}
}

// ListClustersRequest represents cluster list query parameters
type ListClustersRequest struct {
	State          string `form:"state"`          // Pending, Ready, Offline, Rejected
	ConnectionMode string `form:"connectionMode"` // Hub, Edge
	Limit          int64  `form:"limit"`          // page size
	Continue       string `form:"continue"`       // pagination cursor
}

// List GET /clusters
func (c *ClusterController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req ListClustersRequest
		if err := ctx.ShouldBindQuery(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		result, err := c.svc.List(ctx.Request.Context(), &service.ListClustersRequest{
			State:          req.State,
			ConnectionMode: req.ConnectionMode,
			Limit:          req.Limit,
			Continue:       req.Continue,
		})
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to list clusters", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}

		utils.Success(ctx, result)
	}, nil
}

// Get GET /clusters/:id
func (c *ClusterController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		cluster, err := c.svc.Get(ctx.Request.Context(), name)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to get cluster", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.Success(ctx, cluster)
	}, nil
}

// Approve POST /clusters/:id/approve
func (c *ClusterController) Approve() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		if err := c.svc.Approve(ctx.Request.Context(), &service.ApproveClusterRequest{Name: name}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to approve cluster", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "cluster approved successfully", nil)
	}, nil
}

// Reject POST /clusters/:id/reject
func (c *ClusterController) Reject() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		if err := c.svc.Reject(ctx.Request.Context(), &service.RejectClusterRequest{Name: name}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to reject cluster", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "cluster rejected successfully", nil)
	}, nil
}

// Delete DELETE /clusters/:id
func (c *ClusterController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		if err := c.svc.Delete(ctx.Request.Context(), name); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to delete cluster", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "cluster deleted successfully", nil)
	}, nil
}

// UpdateAddonsRequest represents update cluster addons request
type UpdateAddonsRequest struct {
	Addons []v1alpha1.ClusterAddon `json:"addons" binding:"required"`
}

// UpdateAddons PUT /clusters/:id/addons
func (c *ClusterController) UpdateAddons() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		var req UpdateAddonsRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		if err := c.svc.UpdateAddons(ctx.Request.Context(), &service.UpdateClusterAddonsRequest{
			Name:   name,
			Addons: req.Addons,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to update cluster addons", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "cluster addons updated successfully", nil)
	}, nil
}

// GetAddons GET /clusters/:id/addons
func (c *ClusterController) GetAddons() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "cluster name is required")
			return
		}

		addons, statuses, err := c.svc.GetClusterAddons(ctx.Request.Context(), name)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to get cluster addons", "cluster", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.Success(ctx, gin.H{
			"addons":   addons,
			"statuses": statuses,
		})
	}, nil
}

// Create POST /clusters
// Cluster is automatically created by agent, portal only supports approve/reject, direct creation is not supported
func (c *ClusterController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		utils.BadRequest(ctx, "clusters are created by agents, please use approve/reject API")
	}, nil
}

// Update PUT /clusters/:id
func (c *ClusterController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		utils.BadRequest(ctx, "please use /clusters/:id/addons to update cluster configuration")
	}, nil
}

// Patch implements partial update (not supported for cluster)
func (c *ClusterController) Patch() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		utils.BadRequest(ctx, "patch not supported for clusters")
	}, nil
}
