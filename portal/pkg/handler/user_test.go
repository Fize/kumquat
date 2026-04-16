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

// setupUserTestDB creates test database and dependencies
func setupUserTestDB(t *testing.T) (*gin.Engine, *service.UserService, *service.RoleService, *middleware.AuthMiddleware, *gorm.DB, *service.JWTService) {
	gin.SetMode(gin.TestMode)

	// Create independent in-memory SQLite database
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate table structure
	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{}, &model.Project{})
	require.NoError(t, err)

	// Create roles
	adminRole := &model.Role{Name: model.RoleAdmin}
	memberRole := &model.Role{Name: model.RoleMember}
	guestRole := &model.Role{Name: model.RoleGuest}
	err = db.Create(adminRole).Error
	require.NoError(t, err)
	err = db.Create(memberRole).Error
	require.NoError(t, err)
	err = db.Create(guestRole).Error
	require.NoError(t, err)

	// Create JWT service
	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	authMiddleware := middleware.NewAuthMiddleware(jwtSvc)

	// Create repository
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)

	// Create service
	userSvc := service.NewUserService(userRepo, roleRepo, db)
	roleSvc := service.NewRoleService(roleRepo, db)

	router := gin.New()
	return router, userSvc, roleSvc, authMiddleware, db, jwtSvc
}

// createAdminUser creates admin user and returns token
func createAdminUser(t *testing.T, db *gorm.DB, jwtSvc *service.JWTService) (*model.User, string) {
	user := &model.User{
		Username: "admin",
		Email:    "admin@example.com",
		RoleID:   1, // admin
	}
	user.SetPassword("admin123")
	err := db.Create(user).Error
	require.NoError(t, err)

	token, err := jwtSvc.GenerateToken(user.ID, user.Username, user.RoleID, model.RoleAdmin)
	require.NoError(t, err)

	return user, token
}

func TestUserController_List_Success(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.List()
	require.NoError(t, err)

	router.GET("/api/v1/users", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users?page=1&size=10", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestUserController_List_Unauthorized(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, _, _ := setupUserTestDB(t)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.List()
	require.NoError(t, err)

	router.GET("/api/v1/users", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users", nil)
	// No token provided
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserController_Get_Success(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   2,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/2", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestUserController_Get_InvalidID(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/invalid", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserController_Get_NotFound(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserController_Create_Success(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)

	router.POST("/api/v1/users", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	body := map[string]interface{}{
		"username": "newuser",
		"email":    "new@example.com",
		"password": "password123",
		"nickname": "New User",
		"role_id":  2,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestUserController_Create_ValidationError(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)

	router.POST("/api/v1/users", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	// Missing required field
	body := map[string]string{
		"username": "newuser",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserController_Update_Success(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	// Create test user
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   2,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Update()
	require.NoError(t, err)

	router.PUT("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	body := map[string]interface{}{
		"nickname": "Updated Name",
		"role_id":  2,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/users/2", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestUserController_Update_NotFound(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Update()
	require.NoError(t, err)

	router.PUT("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	body := map[string]interface{}{
		"nickname": "Updated Name",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/users/999", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserController_Delete_Success(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	// Create test user (non-admin)
	user := &model.User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   2,
	}
	user.SetPassword("password123")
	err := db.Create(user).Error
	require.NoError(t, err)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Delete()
	require.NoError(t, err)

	router.DELETE("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/users/2", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUserController_Delete_NotFound(t *testing.T) {
	router, userSvc, roleSvc, authMiddleware, db, jwtSvc := setupUserTestDB(t)
	_, adminToken := createAdminUser(t, db, jwtSvc)

	ctrl := NewUserController(userSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Delete()
	require.NoError(t, err)

	router.DELETE("/api/v1/users/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/users/999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
