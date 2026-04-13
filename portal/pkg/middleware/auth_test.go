package middleware

import (
	"testing"
	"time"

	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
)

func TestAuthMiddleware_Auth_MissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	m := NewAuthMiddleware(jwtSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler := m.Auth()
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}

func TestAuthMiddleware_Auth_InvalidFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	m := NewAuthMiddleware(jwtSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "InvalidFormat")

	handler := m.Auth()
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}

func TestAuthMiddleware_Auth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	m := NewAuthMiddleware(jwtSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer invalid-token")

	handler := m.Auth()
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}

func TestAuthMiddleware_Auth_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	m := NewAuthMiddleware(jwtSvc)

	token, err := jwtSvc.GenerateToken(1, "testuser", 2, "admin")
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	handler := m.Auth()
	handler(c)

	assert.False(t, c.IsAborted())
	assert.Equal(t, uint(1), GetUserID(c))
	assert.Equal(t, "testuser", c.GetString(ContextKeyUsername))
	assert.Equal(t, uint(2), GetRoleID(c))
	assert.Equal(t, "admin", GetRoleName(c))
}

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("allowed role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyRoleName, "admin")

		called := false
		handler := RequireRole("admin")
		handler(c)
		// Manually call next handler since we can't use c.Handlers
		if !c.IsAborted() {
			called = true
		}

		assert.True(t, called)
		assert.False(t, c.IsAborted())
	})

	t.Run("denied role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(ContextKeyRoleName, "guest")

		handler := RequireRole("admin")
		handler(c)

		assert.True(t, c.IsAborted())
	})
}

// GetUsername is tested via the ValidToken test above
