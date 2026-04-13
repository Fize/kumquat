package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRoleRepository_GetByName(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewRoleRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
		AddRow(1, time.Now(), time.Now(), "admin")

	mock.ExpectQuery("SELECT \\* FROM `roles` WHERE name = \\? AND `roles`\\.\\`deleted_at\\` IS NULL ORDER BY").
		WithArgs("admin", 1).
		WillReturnRows(rows)

	role, err := repo.GetByName(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", role.Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoleRepository_GetPermissionsByRoleID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewRoleRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "resource", "action", "effect"}).
		AddRow(1, time.Now(), time.Now(), 1, "*", "*", "allow").
		AddRow(2, time.Now(), time.Now(), 1, "module", "read", "deny")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `permissions` WHERE role_id = ?")).
		WithArgs(1).
		WillReturnRows(rows)

	perms, err := repo.GetPermissionsByRoleID(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, perms, 2)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoleRepository_CountPermissionsByRoleID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewRoleRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `permissions` WHERE role_id = ?")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(3))

	count, err := repo.CountPermissionsByRoleID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	require.NoError(t, mock.ExpectationsWereMet())
}
