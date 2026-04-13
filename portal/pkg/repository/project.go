package repository

import (
	"context"

	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// ProjectRepository 项目 Repository 接口
type ProjectRepository interface {
	GetByID(ctx context.Context, id uint) (*model.Project, error)
	List(ctx context.Context, page, size int) ([]model.Project, int64, error)
	Create(ctx context.Context, project *model.Project) error
	Update(ctx context.Context, project *model.Project, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	ListByModuleID(ctx context.Context, moduleID uint, page, size int) ([]model.Project, int64, error)
	ExistsModule(ctx context.Context, moduleID uint) (bool, error)
}

// projectRepository 项目 Repository 实现
type projectRepository struct {
	*BaseRepository[model.Project]
	db *gorm.DB
}

// NewProjectRepository 创建项目 Repository
func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepository{
		BaseRepository: NewBaseRepository[model.Project](db),
		db:             db,
	}
}

// GetByID 重写以支持 Preload
func (r *projectRepository) GetByID(ctx context.Context, id uint) (*model.Project, error) {
	var project model.Project
	if err := r.db.WithContext(ctx).Preload("Module").First(&project, id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

// List 重写以支持 Preload 和排序
func (r *projectRepository) List(ctx context.Context, page, size int) ([]model.Project, int64, error) {
	var projects []model.Project
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.Project{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Order("created_at desc").
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

// ListByModuleID 根据模块ID获取项目
func (r *projectRepository) ListByModuleID(ctx context.Context, moduleID uint, page, size int) ([]model.Project, int64, error) {
	var projects []model.Project
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Project{}).Where("module_id = ?", moduleID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Module").
		Offset((page-1)*size).Limit(size).
		Find(&projects).Error; err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

// ExistsModule 检查模块是否存在
func (r *projectRepository) ExistsModule(ctx context.Context, moduleID uint) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Module{}).Where("id = ?", moduleID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
