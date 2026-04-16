package handler

import (
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

// WorkspaceController handles workspace management requests
type WorkspaceController struct {
	svc            *service.WorkspaceService
	roleSvc        *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewWorkspaceController creates a new workspace controller
func NewWorkspaceController(svc *service.WorkspaceService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *WorkspaceController {
	return &WorkspaceController{
		svc:            svc,
		roleSvc:        roleSvc,
		authMiddleware: authMiddleware,
	}
}

// Name returns the controller name
func (c *WorkspaceController) Name() string {
	return "workspaces"
}

// Version returns the API version
func (c *WorkspaceController) Version() string {
	return "v1"
}

// Middlewares returns the middlewares for this controller
// Workspace management: Admin/Member can read/write, Guest read-only
func (c *WorkspaceController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods: []string{"GET"},
			Middlewares: []gin.HandlerFunc{
				c.authMiddleware.Auth(),
				middleware.RequirePermission(c.roleSvc, model.ResourceWorkspace, model.ActionRead),
			},
		},
		{
			Methods: []string{"POST", "PUT", "DELETE"},
			Middlewares: []gin.HandlerFunc{
				c.authMiddleware.Auth(),
				middleware.RequirePermission(c.roleSvc, model.ResourceWorkspace, model.ActionWrite),
			},
		},
	}
}

// ListWorkspacesRequest represents workspace list query parameters
type ListWorkspacesRequest struct {
	Cluster  string `form:"cluster"`
	Limit    int64  `form:"limit"`     // page size
	Continue string `form:"continue"`  // pagination cursor
}

// List GET /workspaces
func (c *WorkspaceController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req ListWorkspacesRequest
		if err := ctx.ShouldBindQuery(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		result, err := c.svc.List(ctx.Request.Context(), &service.ListWorkspacesRequest{
			Cluster:  req.Cluster,
			Limit:    req.Limit,
			Continue: req.Continue,
		})
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to list workspaces", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}

		utils.Success(ctx, result)
	}, nil
}

// Get GET /workspaces/:id
func (c *WorkspaceController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "workspace name is required")
			return
		}

		workspace, err := c.svc.Get(ctx.Request.Context(), name)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to get workspace", "workspace", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.Success(ctx, workspace)
	}, nil
}

// Create POST /workspaces
func (c *WorkspaceController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var workspace workspacev1alpha1.Workspace
		if err := ctx.ShouldBindJSON(&workspace); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		if err := c.svc.Create(ctx.Request.Context(), &service.CreateWorkspaceRequest{
			Workspace: &workspace,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to create workspace", "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "workspace created successfully", nil)
	}, nil
}

// Update PUT /workspaces/:id
func (c *WorkspaceController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "workspace name is required")
			return
		}

		var workspace workspacev1alpha1.Workspace
		if err := ctx.ShouldBindJSON(&workspace); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		// Ensure the name matches the URL parameter
		workspace.Name = name

		// ResourceVersion is required for optimistic concurrency control
		if workspace.ResourceVersion == "" {
			utils.BadRequest(ctx, "resourceVersion is required for update operations")
			return
		}

		if err := c.svc.Update(ctx.Request.Context(), &service.UpdateWorkspaceRequest{
			Workspace: &workspace,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to update workspace", "workspace", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "workspace updated successfully", nil)
	}, nil
}

// Delete DELETE /workspaces/:id
func (c *WorkspaceController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "workspace name is required")
			return
		}

		if err := c.svc.Delete(ctx.Request.Context(), name); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to delete workspace", "workspace", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "workspace deleted successfully", nil)
	}, nil
}

// GetClusters GET /workspaces/:id/clusters
func (c *WorkspaceController) GetClusters() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		name := ctx.Param("id")
		if name == "" {
			utils.BadRequest(ctx, "workspace name is required")
			return
		}

		appliedClusters, failedClusters, err := c.svc.GetClustersByWorkspace(ctx.Request.Context(), name)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to get workspace clusters", "workspace", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.Success(ctx, gin.H{
			"appliedClusters": appliedClusters,
			"failedClusters":  failedClusters,
		})
	}, nil
}

// Patch implements partial update (not supported for workspace)
func (c *WorkspaceController) Patch() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		utils.BadRequest(ctx, "patch not supported for workspaces")
	}, nil
}
