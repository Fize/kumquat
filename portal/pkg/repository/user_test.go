package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return db, mock
}

func TestUserRepository_GetByUsername(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "username", "email", "password", "nickname", "role_id"}).
		AddRow(1, time.Now(), time.Now(), "testuser", "test@example.com", "$2a$10$hash", "Test", 2)

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE username = \\? AND `users`\\.\\`deleted_at\\` IS NULL ORDER BY").
		WithArgs("testuser", 1).
		WillReturnRows(rows)

	user, err := repo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_GetByUsername_NotFound(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE username = \\? AND `users`\\.\\`deleted_at\\` IS NULL ORDER BY").
		WithArgs("nonexistent", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	user, err := repo.GetByUsername(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, user)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ExistsByUsername(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users` WHERE username = ?")).
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))

	exists, err := repo.ExistsByUsername(ctx, "testuser")
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ExistsByUsername_NotExists(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users` WHERE username = ?")).
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))

	exists, err := repo.ExistsByUsername(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ExistsByEmail(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users` WHERE email = ?")).
		WithArgs("test@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))

	exists, err := repo.ExistsByEmail(ctx, "test@example.com")
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &model.User{
		Username: "newuser",
		Email:    "new@example.com",
		Password: "$2a$10$hashed",
		RoleID:   2,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), user.Username, user.Email, user.Password, user.Nickname, user.RoleID, user.ModuleID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.Create(ctx, user)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_CountByRoleID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `users` WHERE role_id = ?")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(3))

	count, err := repo.CountByRoleID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	require.NoError(t, mock.ExpectationsWereMet())
}
