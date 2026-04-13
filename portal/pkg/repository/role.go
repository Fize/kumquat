package repository

import (
	"context"

	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// RoleRepository 角色 Repository 接口
type RoleRepository interface {
	GetByID(ctx context.Context, id uint) (*model.Role, error)
	GetByName(ctx context.Context, name string) (*model.Role, error)
	List(ctx context.Context) ([]model.Role, error)
	Create(ctx context.Context, role *model.Role) error
	GetPermissionsByRoleID(ctx context.Context, roleID uint) ([]model.Permission, error)
	CreatePermission(ctx context.Context, perm *model.Permission) error
	CountPermissionsByRoleID(ctx context.Context, roleID uint) (int64, error)
}

// roleRepository 角色 Repository 实现
type roleRepository struct {
	*BaseRepository[model.Role]
	db *gorm.DB
}

// NewRoleRepository 创建角色 Repository
func NewRoleRepository(db *gorm.DB) RoleRepository {
	return &roleRepository{
		BaseRepository: NewBaseRepository[model.Role](db),
		db:             db,
	}
}

// GetByName 根据名称获取
func (r *roleRepository) GetByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// List 获取所有角色
func (r *roleRepository) List(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := r.db.WithContext(ctx).Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// GetPermissionsByRoleID 获取角色权限
func (r *roleRepository) GetPermissionsByRoleID(ctx context.Context, roleID uint) ([]model.Permission, error) {
	var perms []model.Permission
	if err := r.db.WithContext(ctx).Where("role_id = ?", roleID).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// CreatePermission 创建权限
func (r *roleRepository) CreatePermission(ctx context.Context, perm *model.Permission) error {
	return r.db.WithContext(ctx).Create(perm).Error
}

// CountPermissionsByRoleID 统计角色权限数
func (r *roleRepository) CountPermissionsByRoleID(ctx context.Context, roleID uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Permission{}).Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
