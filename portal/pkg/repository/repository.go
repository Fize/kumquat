package repository

import (
	"context"

	"gorm.io/gorm"
)

// Repository 泛型基础 Repository 接口
type Repository[T any] interface {
	GetByID(ctx context.Context, id uint) (*T, error)
	List(ctx context.Context, page, size int) ([]T, int64, error)
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
}

// BaseRepository 泛型基础实现
type BaseRepository[T any] struct {
	db *gorm.DB
}

// NewBaseRepository 创建基础 Repository
func NewBaseRepository[T any](db *gorm.DB) *BaseRepository[T] {
	return &BaseRepository[T]{db: db}
}

// GetByID 根据ID获取
func (r *BaseRepository[T]) GetByID(ctx context.Context, id uint) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// List 分页列表
func (r *BaseRepository[T]) List(ctx context.Context, page, size int) ([]T, int64, error) {
	var entities []T
	var total int64

	if err := r.db.WithContext(ctx).Model(new(T)).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Offset((page - 1) * size).Limit(size).
		Find(&entities).Error; err != nil {
		return nil, 0, err
	}

	return entities, total, nil
}

// Create 创建
func (r *BaseRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

// Update 更新
func (r *BaseRepository[T]) Update(ctx context.Context, entity *T, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(entity).Updates(updates).Error
}

// Delete 删除
func (r *BaseRepository[T]) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(new(T), id).Error
}

// DB 获取底层 DB（用于复杂查询）
func (r *BaseRepository[T]) DB() *gorm.DB {
	return r.db
}

// WithTransaction 在事务中执行函数
func WithTransaction(db *gorm.DB, ctx context.Context, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}
