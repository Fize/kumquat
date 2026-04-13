package middleware

import (
	"strings"

	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserID   = "userId"
	ContextKeyUsername = "username"
	ContextKeyRoleID   = "roleId"
	ContextKeyRoleName = "roleName"
)

// Auth JWT认证中间件
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			utils.Unauthorized(c, "invalid authorization header format")
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			utils.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Set(ContextKeyRoleID, claims.RoleID)
		c.Set(ContextKeyRoleName, claims.RoleName)

		c.Next()
	}
}

// GetUserID 获取用户ID
func GetUserID(c *gin.Context) uint {
	if v, exists := c.Get(ContextKeyUserID); exists {
		return v.(uint)
	}
	return 0
}

// GetRoleID 获取角色ID
func GetRoleID(c *gin.Context) uint {
	if v, exists := c.Get(ContextKeyRoleID); exists {
		return v.(uint)
	}
	return 0
}

// GetRoleName 获取角色名
func GetRoleName(c *gin.Context) string {
	if v, exists := c.Get(ContextKeyRoleName); exists {
		return v.(string)
	}
	return ""
}

// RequireRole 要求特定角色（粗粒度鉴权）
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleName := GetRoleName(c)
		for _, role := range roles {
			if roleName == role {
				c.Next()
				return
			}
		}
		utils.Forbidden(c, "insufficient permissions")
		c.Abort()
	}
}

// RequirePermission 要求特定权限（细粒度鉴权，基于 Permission 表）
func RequirePermission(roleService *service.RoleService, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID := GetRoleID(c)
		allowed, err := roleService.CheckPermission(roleID, resource, action)
		if err != nil {
			utils.InternalError(c, "permission check failed")
			c.Abort()
			return
		}
		if !allowed {
			utils.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}
		c.Next()
	}
}
