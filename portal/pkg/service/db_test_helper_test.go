package service

import (
	"testing"
	"time"

	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建内存 SQLite 数据库并自动迁移
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{}, &model.Project{})
	require.NoError(t, err)
	return db
}

// setupTestDBWithRoles 创建带预定义角色的测试数据库
func setupTestDBWithRoles(t *testing.T) *gorm.DB {
	db := setupTestDB(t)

	// 创建预定义角色
	roles := []model.Role{
		{Name: model.RoleAdmin},
		{Name: model.RoleMember},
		{Name: model.RoleGuest},
	}
	for _, r := range roles {
		err := db.Create(&r).Error
		require.NoError(t, err)
	}
	return db
}

// setupTestJWTService 创建测试用 JWT service
func setupTestJWTService() *JWTService {
	return NewJWTService("test-secret", time.Hour, 10*time.Minute)
}

// assertAppErrCode 断言错误码
func assertAppErrCode(t *testing.T, err error, expectedCode int) {
	t.Helper()
	assert.True(t, apperr.IsCode(err, expectedCode), "expected error code %d, got: %v", expectedCode, err)
}

// createTestModule 在 DB 中创建测试模块
func createTestModule(t *testing.T, db *gorm.DB, name string, parentID *uint, sort int) *model.Module {
	m := &model.Module{Name: name, ParentID: parentID, Sort: sort}
	// 手动设置 Level 和 Path，因为 BeforeCreate 可能无法在 SQLite 中正确查找 parent
	if parentID != nil {
		var parent model.Module
		require.NoError(t, db.First(&parent, *parentID).Error)
		m.Level = parent.Level + 1
		m.Path = parent.Path + "/" + m.Name
	} else {
		m.Level = 1
		m.Path = "/" + m.Name
	}
	// Skip BeforeCreate hook to preserve manually set Level/Path
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(m).Error)
	return m
}

// createTestUser 在 DB 中创建测试用户
func createTestUser(t *testing.T, db *gorm.DB, username, email, password string, roleID uint) *model.User {
	u := &model.User{
		Username: username,
		Email:    email,
		RoleID:   roleID,
	}
	u.SetPassword(password)
	require.NoError(t, db.Create(u).Error)
	return u
}

// newModuleServiceFromDB 从 DB 创建 ModuleService
func newModuleServiceFromDB(db *gorm.DB) *ModuleService {
	return NewModuleService(repository.NewModuleRepository(db), db)
}

// newProjectServiceFromDB 从 DB 创建 ProjectService
func newProjectServiceFromDB(db *gorm.DB) *ProjectService {
	return NewProjectService(repository.NewProjectRepository(db), db)
}

// newRoleServiceFromDB 从 DB 创建 RoleService
func newRoleServiceFromDB(db *gorm.DB) *RoleService {
	return NewRoleService(repository.NewRoleRepository(db), db)
}

// newUserServiceFromDB 从 DB 创建 UserService
func newUserServiceFromDB(db *gorm.DB) *UserService {
	return NewUserService(repository.NewUserRepository(db), repository.NewRoleRepository(db), db)
}

// newAuthServiceFromDB 从 DB 创建 AuthService
func newAuthServiceFromDB(db *gorm.DB) *AuthService {
	return NewAuthService(repository.NewUserRepository(db), repository.NewRoleRepository(db), setupTestJWTService(), db)
}
