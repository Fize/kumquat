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

// setupModuleTestDB creates test database and dependencies
func setupModuleTestDB(t *testing.T) (*gin.Engine, *service.ModuleService, *service.RoleService, *middleware.AuthMiddleware, *gorm.DB, *service.JWTService) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{}, &model.Project{})
	require.NoError(t, err)

	// Create role
	adminRole := &model.Role{Name: model.RoleAdmin}
	err = db.Create(adminRole).Error
	require.NoError(t, err)

	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	authMiddleware := middleware.NewAuthMiddleware(jwtSvc)

	moduleRepo := repository.NewModuleRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	moduleSvc := service.NewModuleService(moduleRepo, db)
	roleSvc := service.NewRoleService(roleRepo, db)

	router := gin.New()
	return router, moduleSvc, roleSvc, authMiddleware, db, jwtSvc
}

func TestModuleController_List_Success(t *testing.T) {
	router, moduleSvc, roleSvc, authMiddleware, db, jwtSvc := setupModuleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewModuleController(moduleSvc, roleSvc, authMiddleware)
	handler, err := ctrl.List()
	require.NoError(t, err)

	router.GET("/api/v1/modules", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/modules", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestModuleController_Create_Success(t *testing.T) {
	router, moduleSvc, roleSvc, authMiddleware, db, jwtSvc := setupModuleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewModuleController(moduleSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)

	router.POST("/api/v1/modules", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	body := map[string]interface{}{
		"name":  "test-module",
		"sort":  1,
		"level": 1,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/modules", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestModuleController_Create_ValidationError(t *testing.T) {
	router, moduleSvc, roleSvc, authMiddleware, db, jwtSvc := setupModuleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewModuleController(moduleSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)

	router.POST("/api/v1/modules", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	// Missing name
	body := map[string]interface{}{"sort": 1}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/modules", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestModuleController_Delete_Success(t *testing.T) {
	router, moduleSvc, roleSvc, authMiddleware, db, jwtSvc := setupModuleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	// Create module
	module := &model.Module{Name: "test-module", Sort: 1}
	err = db.Create(module).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewModuleController(moduleSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Delete()
	require.NoError(t, err)

	router.DELETE("/api/v1/modules/:id", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/modules/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
