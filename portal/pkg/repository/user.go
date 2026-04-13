package repository

import (
	"context"

	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// UserRepository 用户 Repository 接口
type UserRepository interface {
	GetByID(ctx context.Context, id uint) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	List(ctx context.Context, page, size int) ([]model.User, int64, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	CountByRoleID(ctx context.Context, roleID uint) (int64, error)
	Search(ctx context.Context, keyword string, page, size int) ([]model.User, int64, error)
}

// userRepository 用户 Repository 实现
type userRepository struct {
	*BaseRepository[model.User]
	db *gorm.DB
}

// NewUserRepository 创建用户 Repository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository[model.User](db),
		db:             db,
	}
}

// GetByUsername 根据用户名获取
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail 根据邮箱获取
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByID 重写以支持 Preload
func (r *userRepository) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Preload("Role").Preload("Module").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// List 重写以支持 Preload
func (r *userRepository) List(ctx context.Context, page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).Preload("Role").Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// CountByRoleID 统计指定角色的用户数
func (r *userRepository) CountByRoleID(ctx context.Context, roleID uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Search 搜索用户
func (r *userRepository) Search(ctx context.Context, keyword string, page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := r.db.WithContext(ctx).Model(&model.User{}).
		Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Role").Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
