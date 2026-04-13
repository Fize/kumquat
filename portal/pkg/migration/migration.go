package migration

import (
	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.Module{},
		&model.Project{},
	)
}
