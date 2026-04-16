package service

import (
	"context"
	"testing"

	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModuleService_List_Empty(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	modules, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, modules)
}

func TestModuleService_List_WithModules(t *testing.T) {
	db := setupTestDB(t)
	createTestModule(t, db, "mod-1", nil, 0)
	createTestModule(t, db, "mod-2", nil, 1)

	svc := newModuleServiceFromDB(db)
	modules, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, modules, 2)
}

func TestModuleService_List_TreeStructure(t *testing.T) {
	db := setupTestDB(t)
	parent := createTestModule(t, db, "parent", nil, 0)
	createTestModule(t, db, "child-1", &parent.ID, 0)
	createTestModule(t, db, "child-2", &parent.ID, 1)

	svc := newModuleServiceFromDB(db)
	modules, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, modules, 1) // 1 root
	assert.Len(t, modules[0].Children, 2) // 2 children
	assert.Equal(t, "child-1", modules[0].Children[0].Name)
	assert.Equal(t, "child-2", modules[0].Children[1].Name)
}

func TestModuleService_GetByID_Success(t *testing.T) {
	db := setupTestDB(t)
	m := createTestModule(t, db, "test-mod", nil, 0)

	svc := newModuleServiceFromDB(db)
	result, err := svc.GetByID(context.Background(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, "test-mod", result.Name)
}

func TestModuleService_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	_, err := svc.GetByID(context.Background(), 999)
	assertAppErrCode(t, err, 1201) // CodeModuleNotFound
}

func TestModuleService_Create_Success_NoParent(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	result, err := svc.Create(context.Background(), "new-mod", nil, 0)
	require.NoError(t, err)
	assert.Equal(t, "new-mod", result.Name)
	assert.Equal(t, 1, result.Level)
}

func TestModuleService_Create_Success_WithParent(t *testing.T) {
	db := setupTestDB(t)
	parent := createTestModule(t, db, "parent", nil, 0)

	svc := newModuleServiceFromDB(db)
	result, err := svc.Create(context.Background(), "child", &parent.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, "child", result.Name)
	assert.Equal(t, 2, result.Level)
}

func TestModuleService_Create_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	fakeID := uint(999)
	_, err := svc.Create(context.Background(), "child", &fakeID, 0)
	assertAppErrCode(t, err, 1201) // CodeModuleNotFound
}

func TestModuleService_Create_ParentAtMaxLevel(t *testing.T) {
	db := setupTestDB(t)
	// Create a module at max level, skipping BeforeCreate hook to preserve Level
	level5 := &model.Module{Name: "level5", Level: model.MaxModuleLevel, Sort: 0, Path: "/a/b/c/d/level5"}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(level5).Error)

	svc := newModuleServiceFromDB(db)
	_, err := svc.Create(context.Background(), "child", &level5.ID, 0)
	assertAppErrCode(t, err, 400) // CodeBadRequest
}

func TestModuleService_Update_Success(t *testing.T) {
	db := setupTestDB(t)
	m := createTestModule(t, db, "old-name", nil, 0)

	svc := newModuleServiceFromDB(db)
	result, err := svc.Update(context.Background(), m.ID, "new-name", 5)
	require.NoError(t, err)
	assert.Equal(t, "new-name", result.Name)
}

func TestModuleService_Update_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	_, err := svc.Update(context.Background(), 999, "name", 0)
	assertAppErrCode(t, err, 1201) // CodeModuleNotFound
}

func TestModuleService_Update_EmptyNameKeepsOld(t *testing.T) {
	db := setupTestDB(t)
	m := createTestModule(t, db, "original", nil, 0)

	svc := newModuleServiceFromDB(db)
	result, err := svc.Update(context.Background(), m.ID, "", 3)
	require.NoError(t, err)
	assert.Equal(t, "original", result.Name) // name not changed
}

func TestModuleService_Delete_Success(t *testing.T) {
	db := setupTestDB(t)
	m := createTestModule(t, db, "to-delete", nil, 0)

	svc := newModuleServiceFromDB(db)
	err := svc.Delete(context.Background(), m.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), m.ID)
	assertAppErrCode(t, err, 1201)
}

func TestModuleService_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := newModuleServiceFromDB(db)

	err := svc.Delete(context.Background(), 999)
	assertAppErrCode(t, err, 1201)
}

func TestModuleService_Delete_WithChildren(t *testing.T) {
	db := setupTestDB(t)
	parent := createTestModule(t, db, "parent", nil, 0)
	createTestModule(t, db, "child", &parent.ID, 0)

	svc := newModuleServiceFromDB(db)
	err := svc.Delete(context.Background(), parent.ID)
	require.NoError(t, err)

	// Both parent and child should be gone
	_, err = svc.GetByID(context.Background(), parent.ID)
	assertAppErrCode(t, err, 1201)
}

func TestModuleService_buildTree_SortOrder(t *testing.T) {
	db := setupTestDB(t)
	// Create modules with different sort orders
	createTestModule(t, db, "z-mod", nil, 2)
	createTestModule(t, db, "a-mod", nil, 0)
	createTestModule(t, db, "m-mod", nil, 1)

	svc := newModuleServiceFromDB(db)
	modules, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, modules, 3)
	assert.Equal(t, "a-mod", modules[0].Name)
	assert.Equal(t, "m-mod", modules[1].Name)
	assert.Equal(t, "z-mod", modules[2].Name)
}
