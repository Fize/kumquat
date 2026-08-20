package handler

import (
	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/fize/kumquat/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
)

func businessPrincipal(ctx *gin.Context) service.Principal {
	return service.PrincipalFromContext(ctx.Request.Context())
}

func requireScopedAccess(ctx *gin.Context, allowed bool, err error) bool {
	if err != nil {
		utils.InternalError(ctx, "failed to evaluate business scope")
		return false
	}
	if !allowed {
		utils.Forbidden(ctx, service.ErrForbidden.Error())
		return false
	}
	return true
}
