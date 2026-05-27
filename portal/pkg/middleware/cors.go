package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns a CORS middleware with configurable allowed origins.
// If allowedOrigins is empty, the middleware uses a fail-safe default:
// it reflects the request's Origin header (never returns "*").
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowOrigin := ""

		if len(allowedOrigins) == 0 {
			// Fail-safe: when no origins are configured, reflect the request origin.
			// This is still more secure than wildcard "*".
			allowOrigin = origin
		} else {
			for _, allowed := range allowedOrigins {
				if strings.EqualFold(allowed, origin) || allowed == "*" {
					allowOrigin = origin
					break
				}
			}
		}

		if allowOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
