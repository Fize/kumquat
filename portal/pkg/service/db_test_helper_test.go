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

// setupTestDB creates an in-memory SQLite database with auto-migration
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.Module{}, &model.Project{})
	require.NoError(t, err)
	return db
}

// setupTestDBWithRoles creates a test database with predefined roles
func setupTestDBWithRoles(t *testing.T) *gorm.DB {
	db := setupTestDB(t)

	// Create predefined roles
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

// setupTestJWTService creates a JWT service for testing
func setupTestJWTService() *JWTService {
	return NewJWTService("test-secret", time.Hour, 10*time.Minute)
}

// assertAppErrCode asserts error code
func assertAppErrCode(t *testing.T, err error, expectedCode int) {
	t.Helper()
	assert.True(t, apperr.IsCode(err, expectedCode), "expected error code %d, got: %v", expectedCode, err)
}

// createTestModule creates a test module in the database
func createTestModule(t *testing.T, db *gorm.DB, name string, parentID *uint, sort int) *model.Module {
	m := &model.Module{Name: name, ParentID: parentID, Sort: sort}
	// Manually set Level and Path, as BeforeCreate may not properly find parent in SQLite
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

// createTestUser creates a test user in the database
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

// newModuleServiceFromDB creates a ModuleService from the database
func newModuleServiceFromDB(db *gorm.DB) *ModuleService {
	return NewModuleService(repository.NewModuleRepository(db), db)
}

// newProjectServiceFromDB creates a ProjectService from the database
func newProjectServiceFromDB(db *gorm.DB) *ProjectService {
	return NewProjectService(repository.NewProjectRepository(db), db)
}

// newRoleServiceFromDB creates a RoleService from the database
func newRoleServiceFromDB(db *gorm.DB) *RoleService {
	return NewRoleService(repository.NewRoleRepository(db), db)
}

// newUserServiceFromDB creates a UserService from the database
func newUserServiceFromDB(db *gorm.DB) *UserService {
	return NewUserService(repository.NewUserRepository(db), repository.NewRoleRepository(db), db)
}

// newAuthServiceFromDB creates an AuthService from the database
func newAuthServiceFromDB(db *gorm.DB) *AuthService {
	return NewAuthService(repository.NewUserRepository(db), repository.NewRoleRepository(db), setupTestJWTService(), db)
}
