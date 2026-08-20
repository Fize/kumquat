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

func TestProjectRepository_ExistsModule(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewProjectRepository(db)
	ctx := context.Background()

	t.Run("module exists", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `modules` WHERE id = ?")).
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))

		exists, err := repo.ExistsModule(ctx, 1)
		require.NoError(t, err)
		assert.True(t, exists)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("module not exists", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `modules` WHERE id = ?")).
			WithArgs(999).
			WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))

		exists, err := repo.ExistsModule(ctx, 999)
		require.NoError(t, err)
		assert.False(t, exists)

		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProjectRepository_GetByID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := NewProjectRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "module_id", "config"}).
		AddRow(1, time.Now(), time.Now(), "project1", 1, nil)

	mock.ExpectQuery("SELECT \\* FROM `projects` WHERE `projects`\\.\\`id\\` = \\? AND `projects`\\.\\`deleted_at\\` IS NULL ORDER BY").
		WithArgs(1, 1).
		WillReturnRows(rows)

	mock.ExpectQuery("SELECT \\* FROM `modules` WHERE `modules`\\.\\`id\\` = \\? AND `modules`\\.\\`deleted_at\\` IS NULL").
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "parent_id", "level", "sort"}).
			AddRow(1, time.Now(), time.Now(), "module1", nil, 1, 0))

	project, err := repo.GetByID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "project1", project.Name)

	require.NoError(t, mock.ExpectationsWereMet())
}
