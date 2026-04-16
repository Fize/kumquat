package service

import (
	"context"
	"testing"

	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleService_List_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newRoleServiceFromDB(db)

	roles, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, roles, 3)
}

func TestRoleService_GetByID_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newRoleServiceFromDB(db)

	role, err := svc.GetByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, model.RoleAdmin, role.Name)
}

func TestRoleService_GetByID_NotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newRoleServiceFromDB(db)

	_, err := svc.GetByID(context.Background(), 999)
	assertAppErrCode(t, err, 1101) // CodeRoleNotFound
}

func TestRoleService_GetPermissions_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newRoleServiceFromDB(db)

	// Create permissions for role
	perm := model.Permission{RoleID: 1, Resource: model.ResourceAll, Action: model.ActionAll, Effect: model.EffectAllow}
	require.NoError(t, db.Create(&perm).Error)

	perms, err := svc.GetPermissions(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, perms, 1)
}

func TestRoleService_GetPermissions_RoleNotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newRoleServiceFromDB(db)

	_, err := svc.GetPermissions(context.Background(), 999)
	assert.Error(t, err)
}

func TestRoleService_InitRoles_Success(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)

	err := svc.InitRoles()
	require.NoError(t, err)

	// Verify roles created
	roles, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, roles, 3)

	// Verify permissions created for admin
	perms, err := svc.GetPermissions(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, len(perms) > 0)
}

func TestRoleService_InitRoles_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)

	// Call twice - should not fail
	err := svc.InitRoles()
	require.NoError(t, err)
	err = svc.InitRoles()
	require.NoError(t, err)

	// Should still have 3 roles
	roles, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, roles, 3)
}

func TestRoleService_CheckPermission_Admin_AllowAll(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)
	require.NoError(t, svc.InitRoles())

	// Admin (role_id=1) should be allowed everything
	allowed, err := svc.CheckPermission(context.Background(), 1, "cluster", "read")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = svc.CheckPermission(context.Background(), 1, "application", "write")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = svc.CheckPermission(context.Background(), 1, "any-resource", "any-action")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRoleService_CheckPermission_Guest_ReadOnly(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)
	require.NoError(t, svc.InitRoles())

	// Guest (role_id=3) should only read
	allowed, err := svc.CheckPermission(context.Background(), 3, "application", "read")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = svc.CheckPermission(context.Background(), 3, "cluster", "write")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRoleService_CheckPermission_DenyOverrides(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)
	require.NoError(t, svc.InitRoles())

	// Add a deny rule for admin
	denyPerm := model.Permission{RoleID: 1, Resource: "cluster", Action: "delete", Effect: model.EffectDeny}
	require.NoError(t, db.Create(&denyPerm).Error)

	// Deny should override allow (admin has wildcard allow)
	allowed, err := svc.CheckPermission(context.Background(), 1, "cluster", "delete")
	require.NoError(t, err)
	assert.False(t, allowed)

	// But other actions on cluster should still be allowed
	allowed, err = svc.CheckPermission(context.Background(), 1, "cluster", "read")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRoleService_CheckPermission_NoPermissions(t *testing.T) {
	db := setupTestDB(t)
	svc := newRoleServiceFromDB(db)
	require.NoError(t, svc.InitRoles())

	// A role with no explicit permissions should deny everything
	// Create a new role with no permissions
	newRole := model.Role{Name: "empty-role"}
	require.NoError(t, db.Create(&newRole).Error)

	allowed, err := svc.CheckPermission(context.Background(), newRole.ID, "cluster", "read")
	require.NoError(t, err)
	assert.False(t, allowed)
}
