package service

import (
	"context"
	"testing"

	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthService_Login_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "testuser", "test@test.com", "password123", 1)

	svc := newAuthServiceFromDB(db)
	token, user, err := svc.Login(context.Background(), "testuser", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, "testuser", user.Username)
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newAuthServiceFromDB(db)

	_, _, err := svc.Login(context.Background(), "nonexistent", "password")
	assertAppErrCode(t, err, 1003) // CodeInvalidPassword
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "testuser", "test@test.com", "password123", 1)

	svc := newAuthServiceFromDB(db)
	_, _, err := svc.Login(context.Background(), "testuser", "wrongpassword")
	assertAppErrCode(t, err, 1003) // CodeInvalidPassword
}

func TestAuthService_Register_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newAuthServiceFromDB(db)

	user, err := svc.Register(context.Background(), "newuser", "new@test.com", "password", "Nick")
	require.NoError(t, err)
	assert.Equal(t, "newuser", user.Username)
	assert.Equal(t, "new@test.com", user.Email)
	assert.Equal(t, model.RoleGuest, user.Role.Name) // Default role
}

func TestAuthService_Register_UsernameExists(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "existing", "e@test.com", "pass", 1)

	svc := newAuthServiceFromDB(db)
	_, err := svc.Register(context.Background(), "existing", "other@test.com", "pass", "Nick")
	assertAppErrCode(t, err, 1004) // CodeUsernameExists
}

func TestAuthService_Register_EmailExists(t *testing.T) {
	db := setupTestDBWithRoles(t)
	createTestUser(t, db, "user1", "dup@test.com", "pass", 1)

	svc := newAuthServiceFromDB(db)
	_, err := svc.Register(context.Background(), "user2", "dup@test.com", "pass", "Nick")
	assertAppErrCode(t, err, 1005) // CodeEmailExists
}

func TestAuthService_Register_DefaultRoleNotFound(t *testing.T) {
	// DB without guest role
	db := setupTestDB(t)
	svc := newAuthServiceFromDB(db)

	_, err := svc.Register(context.Background(), "user", "u@test.com", "pass", "Nick")
	assert.Error(t, err) // Should fail because guest role doesn't exist
}

func TestAuthService_ChangePassword_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "testuser", "test@test.com", "oldpass", 1)

	svc := newAuthServiceFromDB(db)
	err := svc.ChangePassword(context.Background(), u.ID, "oldpass", "newpass")
	require.NoError(t, err)

	// Verify can login with new password
	_, _, err = svc.Login(context.Background(), "testuser", "newpass")
	require.NoError(t, err)
}

func TestAuthService_ChangePassword_UserNotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newAuthServiceFromDB(db)

	err := svc.ChangePassword(context.Background(), 999, "old", "new")
	assertAppErrCode(t, err, 1001) // CodeUserNotFound
}

func TestAuthService_ChangePassword_WrongOldPassword(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "testuser", "test@test.com", "correctpass", 1)

	svc := newAuthServiceFromDB(db)
	err := svc.ChangePassword(context.Background(), u.ID, "wrongpass", "newpass")
	assertAppErrCode(t, err, 1003) // CodeInvalidPassword
}

func TestAuthService_GetUserByID_Success(t *testing.T) {
	db := setupTestDBWithRoles(t)
	u := createTestUser(t, db, "testuser", "test@test.com", "pass", 1)

	svc := newAuthServiceFromDB(db)
	result, err := svc.GetUserByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
}

func TestAuthService_GetUserByID_NotFound(t *testing.T) {
	db := setupTestDBWithRoles(t)
	svc := newAuthServiceFromDB(db)

	_, err := svc.GetUserByID(context.Background(), 999)
	assertAppErrCode(t, err, 1001)
}
