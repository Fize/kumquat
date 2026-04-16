package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Note: Project Create/Update tests that involve JSONConfig are limited because
// SQLite doesn't support model.JSONConfig type serialization.
// We use direct SQL insertion for test data setup.

// createTestProjectDirect bypasses GORM's type system to insert a project directly via SQL
func createTestProjectDirect(t *testing.T, db *gorm.DB, name string, moduleID uint) {
	t.Helper()
	err := db.Exec("INSERT INTO projects (name, module_id, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))", name, moduleID).Error
	require.NoError(t, err)
}

func TestProjectService_List_Empty(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	projects, total, err := svc.List(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Empty(t, projects)
	assert.Equal(t, int64(0), total)
}

func TestProjectService_Create_ModuleNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	_, err := svc.Create(context.Background(), "test-proj", 999, nil)
	assertAppErrCode(t, err, 1201) // CodeModuleNotFound
}

func TestProjectService_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	_, err := svc.GetByID(context.Background(), 999)
	assertAppErrCode(t, err, 1301) // CodeProjectNotFound
}

func TestProjectService_Update_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	_, err := svc.Update(context.Background(), 999, "name", nil)
	assertAppErrCode(t, err, 1301)
}

func TestProjectService_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	err := svc.Delete(context.Background(), 999)
	assertAppErrCode(t, err, 1301)
}

func TestProjectService_GetByID_Success(t *testing.T) {
	db := setupTestDB(t)
	mod := createTestModule(t, db, "mod", nil, 0)
	createTestProjectDirect(t, db, "proj-1", mod.ID)

	svc := newProjectServiceFromDB(db)
	result, err := svc.GetByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "proj-1", result.Name)
}

func TestProjectService_List_WithProjects(t *testing.T) {
	db := setupTestDB(t)
	mod := createTestModule(t, db, "mod", nil, 0)
	createTestProjectDirect(t, db, "proj-1", mod.ID)
	createTestProjectDirect(t, db, "proj-2", mod.ID)

	svc := newProjectServiceFromDB(db)
	projects, total, err := svc.List(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, projects, 2)
}

func TestProjectService_Delete_Success(t *testing.T) {
	db := setupTestDB(t)
	mod := createTestModule(t, db, "mod", nil, 0)
	createTestProjectDirect(t, db, "to-delete", mod.ID)

	svc := newProjectServiceFromDB(db)
	err := svc.Delete(context.Background(), 1)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), 1)
	assertAppErrCode(t, err, 1301)
}

func TestProjectService_ListByModule(t *testing.T) {
	db := setupTestDB(t)
	mod1 := createTestModule(t, db, "mod-1", nil, 0)
	mod2 := createTestModule(t, db, "mod-2", nil, 1)
	createTestProjectDirect(t, db, "proj-1", mod1.ID)
	createTestProjectDirect(t, db, "proj-2", mod1.ID)
	createTestProjectDirect(t, db, "proj-3", mod2.ID)

	svc := newProjectServiceFromDB(db)
	projects, total, err := svc.ListByModule(context.Background(), mod1.ID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, projects, 2)
}

func TestProjectService_ListByModule_Empty(t *testing.T) {
	db := setupTestDB(t)
	svc := newProjectServiceFromDB(db)

	projects, total, err := svc.ListByModule(context.Background(), 999, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, projects)
	assert.Equal(t, int64(0), total)
}
