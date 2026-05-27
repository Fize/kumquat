package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/fize/go-ext/log"
	"github.com/gin-gonic/gin"
)

// Recovery returns a gin middleware that recovers from panics,
// logs the panic with stack trace, and returns a 500 error.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := string(debug.Stack())
				log.ErrorContext(c.Request.Context(), "panic recovered",
					"panic", fmt.Sprintf("%v", err),
					"stack", stack,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":   "internal server error",
					"message": "an unexpected error occurred",
				})
			}
		}()
		c.Next()
	}
}
