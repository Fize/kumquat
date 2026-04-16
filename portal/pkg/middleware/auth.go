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

// AuthMiddleware JWT authentication middleware
type AuthMiddleware struct {
	jwtService *service.JWTService
}

// NewAuthMiddleware creates authentication middleware
func NewAuthMiddleware(jwtService *service.JWTService) *AuthMiddleware {
	return &AuthMiddleware{jwtService: jwtService}
}

// Auth JWT authentication middleware
func (m *AuthMiddleware) Auth() gin.HandlerFunc {
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

		claims, err := m.jwtService.ParseToken(parts[1])
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

// GetUserID gets user ID
func GetUserID(c *gin.Context) uint {
	if v, exists := c.Get(ContextKeyUserID); exists {
		return v.(uint)
	}
	return 0
}

// GetRoleID gets role ID
func GetRoleID(c *gin.Context) uint {
	if v, exists := c.Get(ContextKeyRoleID); exists {
		return v.(uint)
	}
	return 0
}

// GetRoleName gets role name
func GetRoleName(c *gin.Context) string {
	if v, exists := c.Get(ContextKeyRoleName); exists {
		return v.(string)
	}
	return ""
}

// RequireRole requires specific role (coarse-grained authorization)
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

// RequirePermission requires specific permission (fine-grained authorization, based on Permission table)
func RequirePermission(roleService *service.RoleService, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID := GetRoleID(c)
		allowed, err := roleService.CheckPermission(c.Request.Context(), roleID, resource, action)
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
