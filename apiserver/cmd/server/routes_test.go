package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fize/kumquat/apiserver/internal/testdb"
	"github.com/fize/kumquat/apiserver/pkg/middleware"
	"github.com/fize/kumquat/apiserver/pkg/migration"
	"github.com/fize/kumquat/apiserver/pkg/model"
	"github.com/fize/kumquat/apiserver/pkg/repository"
	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func TestRegisteredBusinessRoutesMatchStableAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	modules := repository.NewModuleRepository(db)
	projects := repository.NewProjectRepository(db)
	roleSvc := service.NewRoleService(roles, db)
	if err := roleSvc.InitRoles(); err != nil {
		t.Fatal(err)
	}
	jwt := service.NewJWTService("test-only-secret", time.Hour, time.Minute)
	auth := service.NewAuthService(users, roles, jwt, db)
	engine := gin.New()
	registerRoutes(engine, db, auth, service.NewUserService(users, roles, db), service.NewModuleService(modules, db), service.NewProjectService(projects, db), roleSvc, middleware.NewAuthMiddleware(jwt), service.NewResourceGateway(db, nil))
	want := map[string]bool{"POST /api/v1/applications": false, "GET /api/v1/applications/:id": false, "POST /api/v1/workspaces": false, "POST /api/v1/clusters/:id/adopt": false, "GET /api/v1/operations/:id": false}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if route.Path == "/api/v1/applications/:namespace/:name" {
			t.Fatal("legacy Engine-shaped application route must not be registered")
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
}

func TestLoadConfigAppliesSQLSecretEnvironmentOverrides(t *testing.T) {
	t.Setenv("KUMQUAT_API_JWT_SECRET", "test-jwt")
	t.Setenv("KUMQUAT_API_SQL_TYPE", "mysql")
	t.Setenv("KUMQUAT_API_SQL_HOST", "mysql.api.svc:3306")
	t.Setenv("KUMQUAT_API_SQL_USER", "kumquat")
	t.Setenv("KUMQUAT_API_SQL_PASSWORD", "secret")
	t.Setenv("KUMQUAT_API_SQL_DB", "kumquat")
	t.Setenv("KUMQUAT_API_SQL_MAX_IDLE_CONNS", "7")
	t.Setenv("KUMQUAT_API_SQL_MAX_OPEN_CONNS", "13")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SQL.Type != "mysql" || cfg.SQL.Host != "mysql.api.svc:3306" || cfg.SQL.User != "kumquat" || cfg.SQL.Password != "secret" || cfg.SQL.DB != "kumquat" {
		t.Fatalf("SQL environment overrides not applied: %#v", cfg.SQL)
	}
	if cfg.SQL.MaxIdleConns != 7 || cfg.SQL.MaxOpenConns != 13 {
		t.Fatalf("SQL pool overrides not applied: %#v", cfg.SQL)
	}
}

func TestNonAdminAdministrativeReadsAreDurablyModuleScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	roles := repository.NewRoleRepository(db)
	roleSvc := service.NewRoleService(roles, db)
	if err := roleSvc.InitRoles(); err != nil {
		t.Fatal(err)
	}
	var guestRole model.Role
	if err := db.Where("name = ?", model.RoleGuest).First(&guestRole).Error; err != nil {
		t.Fatal(err)
	}
	rootA := model.Module{Name: "team-a"}
	rootB := model.Module{Name: "team-b"}
	if err := db.Create(&rootA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&rootB).Error; err != nil {
		t.Fatal(err)
	}
	childA := model.Module{Name: "child-a", ParentID: &rootA.ID}
	if err := db.Create(&childA).Error; err != nil {
		t.Fatal(err)
	}
	projectA := model.Project{Name: "project-a", ModuleID: rootA.ID}
	projectB := model.Project{Name: "project-b", ModuleID: rootB.ID}
	if err := db.Omit("config").Create(&projectA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Omit("config").Create(&projectB).Error; err != nil {
		t.Fatal(err)
	}
	userA := model.User{Username: "guest-a", Email: "a@example.test", Password: "password", RoleID: guestRole.ID, ModuleID: &rootA.ID}
	userB := model.User{Username: "guest-b", Email: "b@example.test", Password: "password", RoleID: guestRole.ID, ModuleID: &rootB.ID}
	if err := db.Create(&userA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&userB).Error; err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	modules := repository.NewModuleRepository(db)
	projects := repository.NewProjectRepository(db)
	jwt := service.NewJWTService("scope-secret", time.Hour, time.Minute)
	token, err := jwt.GenerateToken(userA.ID, userA.Username, guestRole.ID, guestRole.Name)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	registerRoutes(engine, db, service.NewAuthService(users, roles, jwt, db), service.NewUserService(users, roles, db), service.NewModuleService(modules, db), service.NewProjectService(projects, db), roleSvc, middleware.NewAuthMiddleware(jwt), service.NewResourceGateway(db, nil))

	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	for _, path := range []string{"/api/v1/modules/" + rootB.PublicID, "/api/v1/modules/" + rootB.PublicID + "/children", "/api/v1/projects/" + projectB.PublicID, "/api/v1/projects/module/" + rootB.PublicID, "/api/v1/users/" + strconv.FormatUint(uint64(userB.ID), 10)} {
		if got := request(path); got.Code != http.StatusForbidden {
			t.Errorf("%s status=%d body=%s", path, got.Code, got.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/modules", "/api/v1/projects", "/api/v1/users"} {
		got := request(path)
		if got.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, got.Code, got.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(got.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		encoded := got.Body.String()
		if strings.Contains(encoded, rootB.PublicID) || strings.Contains(encoded, projectB.PublicID) || strings.Contains(encoded, userB.Username) {
			t.Errorf("%s leaked cross-module data: %s", path, encoded)
		}
	}
	if got := request("/api/v1/roles"); got.Code != http.StatusForbidden {
		t.Errorf("guest roles status=%d", got.Code)
	}

	var adminRole model.Role
	if err := db.Where("name = ?", model.RoleAdmin).First(&adminRole).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "admin", Email: "admin@example.test", Password: "password", RoleID: adminRole.ID}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	adminToken, err := jwt.GenerateToken(admin.ID, admin.Username, adminRole.ID, adminRole.Name)
	if err != nil {
		t.Fatal(err)
	}
	adminRequest := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	children := adminRequest("/api/v1/modules/" + rootA.PublicID + "/children")
	if children.Code != http.StatusOK {
		t.Fatalf("children status=%d body=%s", children.Code, children.Body.String())
	}
	var childEnvelope struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(children.Body.Bytes(), &childEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(childEnvelope.Data) != 1 || childEnvelope.Data[0]["parent_id"] != rootA.PublicID {
		t.Fatalf("children producer=%s", children.Body.String())
	}
	if _, numeric := childEnvelope.Data[0]["parent_id"].(float64); numeric {
		t.Fatalf("numeric parent_id leaked: %s", children.Body.String())
	}
	permissions := adminRequest("/api/v1/roles/" + strconv.FormatUint(uint64(guestRole.ID), 10) + "/permissions")
	if permissions.Code != http.StatusOK {
		t.Fatalf("permissions status=%d body=%s", permissions.Code, permissions.Body.String())
	}
	var permissionEnvelope struct {
		Data struct {
			Permissions []map[string]interface{} `json:"permissions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(permissions.Body.Bytes(), &permissionEnvelope); err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissionEnvelope.Data.Permissions {
		if len(permission) != 3 || permission["resource"] == nil || permission["action"] == nil || permission["effect"] == nil {
			t.Fatalf("permission producer violates PermissionDTO: %s", permissions.Body.String())
		}
		for _, forbidden := range []string{"id", "role_id", "created_at", "updated_at", "role"} {
			if _, ok := permission[forbidden]; ok {
				t.Fatalf("forbidden permission field %q: %s", forbidden, permissions.Body.String())
			}
		}
	}
}

func TestRegisteredAPIV1RoutesMatchOpenAPIContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	modules := repository.NewModuleRepository(db)
	projects := repository.NewProjectRepository(db)
	roleSvc := service.NewRoleService(roles, db)
	if err := roleSvc.InitRoles(); err != nil {
		t.Fatal(err)
	}
	jwt := service.NewJWTService("test-only-secret", time.Hour, time.Minute)
	auth := service.NewAuthService(users, roles, jwt, db)
	engine := gin.New()
	registerRoutes(engine, db, auth, service.NewUserService(users, roles, db), service.NewModuleService(modules, db), service.NewProjectService(projects, db), roleSvc, middleware.NewAuthMiddleware(jwt), service.NewResourceGateway(db, nil))

	runtimeRoutes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		path := strings.TrimPrefix(route.Path, "/api/v1")
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if strings.HasPrefix(part, ":") {
				parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
			}
		}
		runtimeRoutes[route.Method+" "+strings.Join(parts, "/")] = struct{}{}
	}

	contractData, err := os.ReadFile("../../openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		OpenAPI string                            `yaml:"openapi"`
		Paths   map[string]map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(contractData, &contract); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(contract.OpenAPI, "3.") {
		t.Fatalf("OpenAPI version = %q", contract.OpenAPI)
	}
	contractRoutes := make(map[string]struct{})
	for path, item := range contract.Paths {
		for method := range item {
			upper := strings.ToUpper(method)
			switch upper {
			case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
				contractRoutes[upper+" "+path] = struct{}{}
			}
		}
	}
	if missing, extra := routeDifference(contractRoutes, runtimeRoutes), routeDifference(runtimeRoutes, contractRoutes); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("runtime/OpenAPI route mismatch\nmissing at runtime: %v\nundeclared at runtime: %v", missing, extra)
	}
}

func routeDifference(left, right map[string]struct{}) []string {
	var difference []string
	for route := range left {
		if _, ok := right[route]; !ok {
			difference = append(difference, route)
		}
	}
	sort.Strings(difference)
	return difference
}
