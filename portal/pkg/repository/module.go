package repository

import (
	"context"

	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// ModuleRepository 模块 Repository 接口
type ModuleRepository interface {
	GetByID(ctx context.Context, id uint) (*model.Module, error)
	List(ctx context.Context) ([]model.Module, error)
	Create(ctx context.Context, module *model.Module) error
	Update(ctx context.Context, module *model.Module, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	GetChildren(ctx context.Context, parentID uint) ([]model.Module, error)
}

// moduleRepository 模块 Repository 实现
type moduleRepository struct {
	*BaseRepository[model.Module]
	db *gorm.DB
}

// NewModuleRepository 创建模块 Repository
func NewModuleRepository(db *gorm.DB) ModuleRepository {
	return &moduleRepository{
		BaseRepository: NewBaseRepository[model.Module](db),
		db:             db,
	}
}

// List 获取所有模块
func (r *moduleRepository) List(ctx context.Context) ([]model.Module, error) {
	var modules []model.Module
	if err := r.db.WithContext(ctx).Find(&modules).Error; err != nil {
		return nil, err
	}
	return modules, nil
}

// GetChildren 获取子模块
func (r *moduleRepository) GetChildren(ctx context.Context, parentID uint) ([]model.Module, error) {
	var children []model.Module
	if err := r.db.WithContext(ctx).Where("parent_id = ?", parentID).Find(&children).Error; err != nil {
		return nil, err
	}
	return children, nil
}
