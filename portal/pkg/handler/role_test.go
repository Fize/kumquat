package handler

import (
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

// setupRoleTestDB 创建测试数据库和依赖
func setupRoleTestDB(t *testing.T) (*gin.Engine, *service.RoleService, *middleware.AuthMiddleware, *gorm.DB, *service.JWTService) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{})
	require.NoError(t, err)

	// 创建角色
	adminRole := &model.Role{Name: model.RoleAdmin}
	err = db.Create(adminRole).Error
	require.NoError(t, err)

	// 创建权限
	perm := &model.Permission{RoleID: adminRole.ID, Resource: "*", Action: "*", Effect: "allow"}
	err = db.Create(perm).Error
	require.NoError(t, err)

	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	authMiddleware := middleware.NewAuthMiddleware(jwtSvc)

	roleRepo := repository.NewRoleRepository(db)
	roleSvc := service.NewRoleService(roleRepo, db)

	router := gin.New()
	return router, roleSvc, authMiddleware, db, jwtSvc
}

func TestRoleController_List_Success(t *testing.T) {
	router, roleSvc, authMiddleware, db, jwtSvc := setupRoleTestDB(t)

	// 创建 admin 用户
	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewRoleController(roleSvc, authMiddleware)
	handler, err := ctrl.List()
	require.NoError(t, err)

	router.GET("/api/v1/roles", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestRoleController_Get_Success(t *testing.T) {
	router, roleSvc, authMiddleware, db, jwtSvc := setupRoleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewRoleController(roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/roles/:id", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/roles/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestRoleController_Get_NotFound(t *testing.T) {
	router, roleSvc, authMiddleware, db, jwtSvc := setupRoleTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewRoleController(roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/roles/:id", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/roles/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
