package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fize/kumquat/api/internal/testdb"
	"github.com/fize/kumquat/api/pkg/middleware"
	"github.com/fize/kumquat/api/pkg/model"
	"github.com/fize/kumquat/api/pkg/repository"
	"github.com/fize/kumquat/api/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupProjectTestDB creates test database and dependencies
func setupProjectTestDB(t *testing.T) (*gin.Engine, *service.ProjectService, *service.RoleService, *middleware.AuthMiddleware, *gorm.DB, *service.JWTService) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)

	err := db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{}, &model.Project{}, &model.ResourceRecord{})
	require.NoError(t, err)

	// Create role
	adminRole := &model.Role{Name: model.RoleAdmin}
	err = db.Create(adminRole).Error
	require.NoError(t, err)

	jwtSvc := service.NewJWTService("test-secret", time.Hour, 10*time.Minute)
	authMiddleware := middleware.NewAuthMiddleware(jwtSvc)

	projectRepo := repository.NewProjectRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	projectSvc := service.NewProjectService(projectRepo, db)
	roleSvc := service.NewRoleService(roleRepo, db)

	router := gin.New()
	return router, projectSvc, roleSvc, authMiddleware, db, jwtSvc
}

func TestProjectController_List_Success(t *testing.T) {
	router, projectSvc, roleSvc, authMiddleware, db, jwtSvc := setupProjectTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewProjectController(projectSvc, roleSvc, authMiddleware)
	handler, err := ctrl.List()
	require.NoError(t, err)

	router.GET("/api/v1/projects", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/projects?page=1&size=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
}

func TestProjectController_Create_Success(t *testing.T) {
	router, projectSvc, roleSvc, authMiddleware, db, jwtSvc := setupProjectTestDB(t)
	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	require.NoError(t, db.Create(admin).Error)
	module := &model.Module{Name: "test-module"}
	require.NoError(t, db.Create(module).Error)
	token, err := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)
	require.NoError(t, err)
	ctrl := NewProjectController(projectSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)
	router.POST("/api/v1/projects", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)
	body, err := json.Marshal(map[string]interface{}{"name": "test-project", "moduleId": module.PublicID})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestProjectController_Create_ValidationError(t *testing.T) {
	router, projectSvc, roleSvc, authMiddleware, db, jwtSvc := setupProjectTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewProjectController(projectSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Create()
	require.NoError(t, err)

	router.POST("/api/v1/projects", authMiddleware.Auth(), middleware.RequireRole("admin"), handler)

	// Missing required field
	body := map[string]interface{}{"name": "test-project"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProjectController_Get_NotFound(t *testing.T) {
	router, projectSvc, roleSvc, authMiddleware, db, jwtSvc := setupProjectTestDB(t)

	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	err := db.Create(admin).Error
	require.NoError(t, err)

	token, _ := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)

	ctrl := NewProjectController(projectSvc, roleSvc, authMiddleware)
	handler, err := ctrl.Get()
	require.NoError(t, err)

	router.GET("/api/v1/projects/:id", authMiddleware.Auth(), handler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/projects/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProjectController_Delete_Success(t *testing.T) {
	router, projectSvc, _, authMiddleware, db, jwtSvc := setupProjectTestDB(t)
	admin := &model.User{Username: "admin", Email: "admin@example.com", RoleID: 1}
	admin.SetPassword("admin123")
	require.NoError(t, db.Create(admin).Error)
	module := &model.Module{Name: "test-module"}
	require.NoError(t, db.Create(module).Error)
	project := &model.Project{Name: "test-project", ModuleID: module.ID}
	require.NoError(t, db.Omit("config").Create(project).Error)
	token, err := jwtSvc.GenerateToken(admin.ID, admin.Username, admin.RoleID, model.RoleAdmin)
	require.NoError(t, err)
	ctrl := NewProjectController(projectSvc, nil, authMiddleware)
	handler, err := ctrl.Delete()
	require.NoError(t, err)
	router.DELETE("/api/v1/projects/:id", authMiddleware.Auth(), handler)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/projects/"+project.PublicID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
