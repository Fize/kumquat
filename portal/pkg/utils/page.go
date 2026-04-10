package utils

import (
	"github.com/gin-gonic/gin"
	"strconv"
)

// GetPageSize 解析分页参数
func GetPageSize(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return
}
