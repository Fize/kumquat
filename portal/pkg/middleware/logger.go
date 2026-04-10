package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/fize/go-ext/log"
)

// Logger 日志中间件（使用 go-ext/log）
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Info("http request",
			"method", method,
			"path", path,
			"status", status,
			"latency", latency.String(),
			"client_ip", c.ClientIP(),
		)
	}
}
