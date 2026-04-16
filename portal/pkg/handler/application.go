package handler

import (
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplicationController handles application management requests
type ApplicationController struct {
	svc            *service.ApplicationService
	roleSvc        *service.RoleService
	authMiddleware *middleware.AuthMiddleware
}

// NewApplicationController creates a new application controller
func NewApplicationController(svc *service.ApplicationService, roleSvc *service.RoleService, authMiddleware *middleware.AuthMiddleware) *ApplicationController {
	return &ApplicationController{
		svc:            svc,
		roleSvc:        roleSvc,
		authMiddleware: authMiddleware,
	}
}

// Name returns the controller name
func (c *ApplicationController) Name() string {
	return "applications"
}

// Version returns the API version
func (c *ApplicationController) Version() string {
	return "v1"
}

// Middlewares returns the middlewares for this controller
// Application management: Admin/Member can read/write, Guest read-only
func (c *ApplicationController) Middlewares() []ginserver.MiddlewaresObject {
	return []ginserver.MiddlewaresObject{
		{
			Methods: []string{"GET"},
			Middlewares: []gin.HandlerFunc{
				c.authMiddleware.Auth(),
				middleware.RequirePermission(c.roleSvc, model.ResourceApplication, model.ActionRead),
			},
		},
		{
			Methods: []string{"POST", "PUT", "DELETE", "PATCH"},
			Middlewares: []gin.HandlerFunc{
				c.authMiddleware.Auth(),
				middleware.RequirePermission(c.roleSvc, model.ResourceApplication, model.ActionWrite),
			},
		},
	}
}

// ListApplicationsRequest represents application list query parameters
type ListApplicationsRequest struct {
	Namespace       string `form:"namespace"`
	SchedulingPhase string `form:"schedulingPhase"` // Pending, Scheduling, Scheduled, Descheduling, Failed
	HealthPhase     string `form:"healthPhase"`     // Healthy, Progressing, Degraded, Unknown
	Limit           int64  `form:"limit"`            // page size
	Continue        string `form:"continue"`         // pagination cursor
}

// List GET /applications
func (c *ApplicationController) List() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var req ListApplicationsRequest
		if err := ctx.ShouldBindQuery(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		result, err := c.svc.List(ctx.Request.Context(), &service.ListApplicationsRequest{
			Namespace:       req.Namespace,
			SchedulingPhase: req.SchedulingPhase,
			HealthPhase:     req.HealthPhase,
			Limit:           req.Limit,
			Continue:        req.Continue,
		})
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to list applications", "err", err)
			utils.InternalError(ctx, err.Error())
			return
		}

		utils.Success(ctx, result)
	}, nil
}

// Get GET /applications/:namespace/:name
func (c *ApplicationController) Get() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		app, err := c.svc.Get(ctx.Request.Context(), name, namespace)
		if err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to get application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.Success(ctx, app)
	}, nil
}

// Create POST /applications
func (c *ApplicationController) Create() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		var app appsv1alpha1.Application
		if err := ctx.ShouldBindJSON(&app); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		if err := c.svc.Create(ctx.Request.Context(), &service.CreateApplicationRequest{
			Application: &app,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to create application", "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "application created successfully", nil)
	}, nil
}

// Update PUT /applications/:namespace/:name
func (c *ApplicationController) Update() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		var app appsv1alpha1.Application
		if err := ctx.ShouldBindJSON(&app); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		// Ensure that name and namespace match URL parameters
		app.Name = name
		app.Namespace = namespace

		// ResourceVersion is required for optimistic concurrency control
		if app.ResourceVersion == "" {
			utils.BadRequest(ctx, "resourceVersion is required for update operations")
			return
		}

		if err := c.svc.Update(ctx.Request.Context(), &service.UpdateApplicationRequest{
			Application: &app,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to update application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "application updated successfully", nil)
	}, nil
}

// Delete DELETE /applications/:namespace/:name
func (c *ApplicationController) Delete() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		if err := c.svc.Delete(ctx.Request.Context(), name, namespace); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to delete application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "application deleted successfully", nil)
	}, nil
}

// SuspendRequest represents suspend/resume application request
type SuspendRequest struct {
	Suspend bool `json:"suspend"`
}

// Suspend PATCH /applications/:namespace/:name/suspend
func (c *ApplicationController) Suspend() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		var req SuspendRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		if err := c.svc.Suspend(ctx.Request.Context(), &service.SuspendApplicationRequest{
			Name:      name,
			Namespace: namespace,
			Suspend:   req.Suspend,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to suspend/resume application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		action := "resumed"
		if req.Suspend {
			action = "suspended"
		}
		utils.SuccessWithMessage(ctx, "application "+action+" successfully", nil)
	}, nil
}

// ScaleRequest represents scale application request
type ScaleRequest struct {
	Replicas int32 `json:"replicas" binding:"required,min=0"`
}

// Scale POST /applications/:namespace/:name/scale
func (c *ApplicationController) Scale() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		var req ScaleRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(ctx, err.Error())
			return
		}

		if err := c.svc.Scale(ctx.Request.Context(), &service.ScaleApplicationRequest{
			Name:      name,
			Namespace: namespace,
			Replicas:  req.Replicas,
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to scale application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "application scaled successfully", nil)
	}, nil
}

// ensure ObjectReference implements client.Object
var _ client.Object = &appsv1alpha1.Application{}

// Patch uses patch method to update application (supports partial update)
// PATCH /applications/:namespace/:name
func (c *ApplicationController) Patch() (gin.HandlerFunc, error) {
	return func(ctx *gin.Context) {
		namespace := ctx.Param("namespace")
		name := ctx.Param("name")
		if namespace == "" || name == "" {
			utils.BadRequest(ctx, "namespace and name are required")
			return
		}

		// Get patch data
		patchData, err := ctx.GetRawData()
		if err != nil {
			utils.BadRequest(ctx, "invalid patch data")
			return
		}

		// Execute patch
		if err := c.svc.Patch(ctx.Request.Context(), &service.PatchApplicationRequest{
			Name:      name,
			Namespace: namespace,
			Patch:     client.RawPatch(types.MergePatchType, patchData),
		}); err != nil {
			log.ErrorContext(ctx.Request.Context(), "failed to patch application", "namespace", namespace, "name", name, "err", err)
			utils.ErrorFromErr(ctx, err)
			return
		}

		utils.SuccessWithMessage(ctx, "application patched successfully", nil)
	}, nil
}
