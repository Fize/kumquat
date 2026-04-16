package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuthTestDB creates test database and dependencies
func setupAuthTestDB(t *testing.T) (*gin.Engine, *service.AuthService, *middleware.AuthMiddleware, *gorm.DB, *service.JWTService) {
	gin.SetMode(gin.TestMode)

	// Create independent in-memory SQLite database (one per test)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate table structure
	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{})
	require.NoError(t, err)

	// Create default role
	role := &model.Role{Name: model.RoleGuest}
	err = db.Create(role).Error
	require.NoError(t, err)

	// Create JWT service
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	authMiddleware := middleware.NewAuthMiddleware(jwtSvc)

	// Create repository
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)

	// Create real service (using real DB)
	authSvc := service.NewAuthService(userRepo, roleRepo, jwtSvc, db)

	router := gin.New()
	return router, authSvc, authMiddleware, db, jwtSvc
}

func TestAuthController_Login_Success(t *testing.T) {
	router, authSvc, authMiddleware, db, _ := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	body := map[string]string{"username": "testuser", "password": "password123"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
	assert.NotNil(t, resp["data"])
}

func TestAuthController_Login_ValidationError(t *testing.T) {
	router, authSvc, authMiddleware, _, _ := setupAuthTestDB(t)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	// Missing password field
	body := map[string]string{"username": "testuser"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthController_Login_InvalidCredentials(t *testing.T) {
	router, authSvc, authMiddleware, db, _ := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	body := map[string]string{"username": "testuser", "password": "wrongpassword"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthController_DoRegister_Success(t *testing.T) {
	router, authSvc, authMiddleware, _, _ := setupAuthTestDB(t)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	body := map[string]string{
		"username": "newuser",
		"email":    "new@example.com",
		"password": "password123",
		"nickname": "New User",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestAuthController_DoRegister_ValidationError(t *testing.T) {
	router, authSvc, authMiddleware, _, _ := setupAuthTestDB(t)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	// Password too short (less than 6 characters)
	body := map[string]string{
		"username": "newuser",
		"email":    "new@example.com",
		"password": "123",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthController_Me_Success(t *testing.T) {
	router, authSvc, authMiddleware, db, jwtSvc := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	// Generate valid token
	token, err := jwtSvc.GenerateToken(user.ID, user.Username, user.RoleID, "guest")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestAuthController_Me_Unauthorized(t *testing.T) {
	router, authSvc, authMiddleware, _, _ := setupAuthTestDB(t)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	// No token provided
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthController_ChangePassword_Success(t *testing.T) {
	router, authSvc, authMiddleware, db, jwtSvc := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("oldpass123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	// Generate valid token
	token, err := jwtSvc.GenerateToken(user.ID, user.Username, user.RoleID, "guest")
	require.NoError(t, err)

	body := map[string]string{
		"oldPassword": "oldpass123",
		"newPassword": "newpass123",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/auth/change-password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestAuthController_ChangePassword_ValidationError(t *testing.T) {
	router, authSvc, authMiddleware, db, jwtSvc := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("oldpass123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	token, _ := jwtSvc.GenerateToken(user.ID, user.Username, user.RoleID, "guest")

	// Missing newPassword
	body := map[string]string{"oldPassword": "oldpass123"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/auth/change-password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthController_ChangePassword_WrongOldPassword(t *testing.T) {
	router, authSvc, authMiddleware, db, jwtSvc := setupAuthTestDB(t)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   1,
	}
	user.SetPassword("oldpass123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewAuthController(authSvc, authMiddleware)
	api := router.Group("/api/v1")
	ctrl.SetupRoutes(api)

	token, _ := jwtSvc.GenerateToken(user.ID, user.Username, user.RoleID, "guest")

	body := map[string]string{
		"oldPassword": "wrongpass",
		"newPassword": "newpass123",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/auth/change-password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
