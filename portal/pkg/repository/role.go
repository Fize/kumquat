package repository

import (
	"context"

	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// RoleRepository role repository interface
type RoleRepository interface {
	GetByID(ctx context.Context, id uint) (*model.Role, error)
	GetByName(ctx context.Context, name string) (*model.Role, error)
	List(ctx context.Context) ([]model.Role, error)
	Create(ctx context.Context, role *model.Role) error
	GetPermissionsByRoleID(ctx context.Context, roleID uint) ([]model.Permission, error)
	CreatePermission(ctx context.Context, perm *model.Permission) error
	CountPermissionsByRoleID(ctx context.Context, roleID uint) (int64, error)
}

// roleRepository role repository implementation
type roleRepository struct {
	*BaseRepository[model.Role]
	db *gorm.DB
}

// NewRoleRepository creates role repository
func NewRoleRepository(db *gorm.DB) RoleRepository {
	return &roleRepository{
		BaseRepository: NewBaseRepository[model.Role](db),
		db:             db,
	}
}

// GetByName gets by name
func (r *roleRepository) GetByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// List gets all roles
func (r *roleRepository) List(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := r.db.WithContext(ctx).Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// GetPermissionsByRoleID gets role permissions
func (r *roleRepository) GetPermissionsByRoleID(ctx context.Context, roleID uint) ([]model.Permission, error) {
	var perms []model.Permission
	if err := r.db.WithContext(ctx).Where("role_id = ?", roleID).Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// CreatePermission creates permission
func (r *roleRepository) CreatePermission(ctx context.Context, perm *model.Permission) error {
	return r.db.WithContext(ctx).Create(perm).Error
}

// CountPermissionsByRoleID counts role permissions
func (r *roleRepository) CountPermissionsByRoleID(ctx context.Context, roleID uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Permission{}).Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
