package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserService_List_Empty(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	users, total, err := svc.List(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Empty(t, users)
	assert.Equal(t, int64(0), total)
}

func TestUserService_List_WithUsers(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "user1", "u1@test.com", "pass123", 1)
	createTestUser(t, db, "user2", "u2@test.com", "pass123", 2)

	svc := newUserServiceFromDB(db)
	users, total, err := svc.List(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, users, 2)
}

func TestUserService_GetByID_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "testuser", "test@test.com", "pass123", 1)

	svc := newUserServiceFromDB(db)
	result, err := svc.GetByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
}

func TestUserService_GetByID_NotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	_, err := svc.GetByID(context.Background(), 999)
	assertAppErrCode(t, err, 1001) // CodeUserNotFound
}

func TestUserService_Create_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	result, err := svc.Create(context.Background(), "newuser", "new@test.com", "password", "Nick", 3, nil)
	require.NoError(t, err)
	assert.Equal(t, "newuser", result.Username)
	assert.Equal(t, "new@test.com", result.Email)
}

func TestUserService_Create_UsernameExists(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "existing", "e@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	_, err := svc.Create(context.Background(), "existing", "other@test.com", "pass", "Nick", 1, nil)
	assertAppErrCode(t, err, 1004) // CodeUsernameExists
}

func TestUserService_Create_EmailExists(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "user1", "dup@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	_, err := svc.Create(context.Background(), "user2", "dup@test.com", "pass", "Nick", 1, nil)
	assertAppErrCode(t, err, 1005) // CodeEmailExists
}

func TestUserService_Create_RoleNotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	_, err := svc.Create(context.Background(), "user", "u@test.com", "pass", "Nick", 999, nil)
	assertAppErrCode(t, err, 1101) // CodeRoleNotFound
}

func TestUserService_Update_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "user1", "u@test.com", "pass", 3)

	svc := newUserServiceFromDB(db)
	result, err := svc.Update(context.Background(), u.ID, "NewNick", 2, nil)
	require.NoError(t, err)
	assert.Equal(t, "NewNick", result.Nickname)
}

func TestUserService_Update_NotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	_, err := svc.Update(context.Background(), 999, "nick", 1, nil)
	assertAppErrCode(t, err, 1001)
}

func TestUserService_Update_InvalidRole(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "user1", "u@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	_, err := svc.Update(context.Background(), u.ID, "", 999, nil)
	assertAppErrCode(t, err, 1101)
}

func TestUserService_Update_EmptyFieldsNoChange(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "user1", "u@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	result, err := svc.Update(context.Background(), u.ID, "", 0, nil)
	require.NoError(t, err)
	assert.Equal(t, "user1", result.Username) // username unchanged
}

func TestUserService_Delete_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "user1", "u@test.com", "pass", 3) // member

	svc := newUserServiceFromDB(db)
	err := svc.Delete(context.Background(), u.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), u.ID)
	assertAppErrCode(t, err, 1001)
}

func TestUserService_Delete_NotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newUserServiceFromDB(db)

	err := svc.Delete(context.Background(), 999)
	assertAppErrCode(t, err, 1001)
}

func TestUserService_Delete_LastAdmin(t *testing.T) {
	db := setupTestDBWithRoles(t)
	// Create the only admin (role_id=1)
	u := createTestUser(t, db, "admin", "admin@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	err := svc.Delete(context.Background(), u.ID)
	assertAppErrCode(t, err, 1006) // CodeLastAdmin
}

func TestUserService_Delete_MultipleAdmins(t *testing.T) {
	db := setupTestDBWithRoles(t)
	admin1 := createTestUser(t, db, "admin1", "a1@test.com", "pass", 1)
	createTestUser(t, db, "admin2", "a2@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	err := svc.Delete(context.Background(), admin1.ID)
	require.NoError(t, err) // Can delete when multiple admins exist
}

func TestUserService_Search_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "alice", "alice@test.com", "pass", 1)
	createTestUser(t, db, "bob", "bob@test.com", "pass", 2)

	svc := newUserServiceFromDB(db)
	users, total, err := svc.Search(context.Background(), "ali", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, users, 1)
	assert.Equal(t, "alice", users[0].Username)
}

func TestUserService_Search_NoMatch(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "alice", "alice@test.com", "pass", 1)

	svc := newUserServiceFromDB(db)
	users, total, err := svc.Search(context.Background(), "nonexistent", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, users)
}
