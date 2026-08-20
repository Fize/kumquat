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

func TestModuleRepository_List(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewModuleRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "parent_id", "level", "sort"}).
		AddRow(1, time.Now(), time.Now(), "root1", nil, 1, 0).
		AddRow(2, time.Now(), time.Now(), "root2", nil, 1, 1)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `modules` WHERE `modules`.`deleted_at` IS NULL")).
		WillReturnRows(rows)

	modules, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, modules, 2)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModuleRepository_GetChildren(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewModuleRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "parent_id", "level", "sort"}).
		AddRow(2, time.Now(), time.Now(), "child1", 1, 2, 0).
		AddRow(3, time.Now(), time.Now(), "child2", 1, 2, 1)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `modules` WHERE parent_id = ? AND `modules`.`deleted_at` IS NULL")).
		WithArgs(1).
		WillReturnRows(rows)

	children, err := repo.GetChildren(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, children, 2)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModuleRepository_Delete(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewModuleRepository(db)
	ctx := context.Background()

	// GORM soft delete uses UPDATE SET deleted_at
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `modules` SET `deleted_at`=? WHERE `modules`.`id` = ? AND `modules`.`deleted_at` IS NULL")).
		WithArgs(sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.Delete(ctx, 1)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
