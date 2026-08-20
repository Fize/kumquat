package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		pageParam    string
		sizeParam    string
		expectPage   int
		expectSize   int
	}{
		{"default values", "", "", 1, 20},
		{"custom values", "2", "50", 2, 50},
		{"page less than 1", "0", "10", 1, 10},
		{"size less than 1", "1", "0", 1, 20},
		{"size over 100", "1", "200", 1, 20},
		{"invalid page", "abc", "10", 1, 10},
		{"invalid size", "1", "abc", 1, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			// Set query params
			if tt.pageParam != "" {
				c.Request.URL.RawQuery = "page=" + tt.pageParam + "&size=" + tt.sizeParam
			} else if tt.sizeParam != "" {
				c.Request.URL.RawQuery = "size=" + tt.sizeParam
			}

			page, size := GetPageSize(c)
			assert.Equal(t, tt.expectPage, page)
			assert.Equal(t, tt.expectSize, size)
		})
	}
}

func TestSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Success(c, gin.H{"key": "value"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"code":0`)
	assert.Contains(t, w.Body.String(), `"key":"value"`)
}

func TestBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	BadRequest(c, "invalid input")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `"code":400`)
	assert.Contains(t, w.Body.String(), "invalid input")
}

func TestUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Unauthorized(c, "missing token")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":401`)
}

func TestForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Forbidden(c, "insufficient permissions")

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":403`)
}

func TestNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NotFound(c, "resource not found")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `"code":404`)
}
