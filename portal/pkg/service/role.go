package service

import (
	"context"
	"errors"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
	"gorm.io/gorm"
)

// RoleService role service
type RoleService struct {
	repo repository.RoleRepository
	db   *gorm.DB // reserved for transaction
}

// NewRoleService creates role service
func NewRoleService(repo repository.RoleRepository, db *gorm.DB) *RoleService {
	return &RoleService{repo: repo, db: db}
}

// List gets role list
func (s *RoleService) List(ctx context.Context) ([]model.Role, error) {
	roles, err := s.repo.List(ctx)
	if err != nil {
		log.ErrorContext(ctx, "list roles failed", "err", err)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return roles, nil
}

// GetByID gets role by ID
func (s *RoleService) GetByID(ctx context.Context, id uint) (*model.Role, error) {
	role, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeRoleNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return role, nil
}

// GetPermissions gets role permissions
func (s *RoleService) GetPermissions(ctx context.Context, roleID uint) ([]model.Permission, error) {
	_, err := s.repo.GetByID(ctx, roleID)
	if err != nil {
		log.WarnContext(ctx, "get permissions failed: role not found", "role_id", roleID)
		return nil, err
	}

	perms, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		log.ErrorContext(ctx, "get permissions failed: db error", "err", err, "role_id", roleID)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return perms, nil
}

// InitRoles initializes predefined roles and permissions (using transaction)
func (s *RoleService) InitRoles() error {
	ctx := context.Background()

	return repository.WithTransaction(s.db, ctx, func(tx *gorm.DB) error {
		for roleName, permissions := range model.PredefinedPermissions {
			var role model.Role
			if err := tx.Where("name = ?", roleName).First(&role).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					role = model.Role{Name: roleName}
					if err := tx.Create(&role).Error; err != nil {
						log.Error("init roles failed: create role error", "err", err, "role", roleName)
						return err
					}
					log.Info("role created", "role_id", role.ID, "role", roleName)
				} else {
					log.Error("init roles failed: query role error", "err", err, "role", roleName)
					return err
				}
			}

			// Initialize permissions: only write predefined permissions when role has no permission records
			var count int64
			if err := tx.Model(&model.Permission{}).Where("role_id = ?", role.ID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				for _, p := range permissions {
					perm := model.Permission{
						RoleID:   role.ID,
						Resource: p.Resource,
						Action:   p.Action,
						Effect:   p.Effect,
					}
					if err := tx.Create(&perm).Error; err != nil {
						log.Error("init permissions failed", "err", err, "role", roleName, "resource", p.Resource, "action", p.Action)
						return err
					}
				}
				log.Info("role permissions initialized", "role", roleName, "count", len(permissions))
			}
		}
		return nil
	})
}

// CheckPermission checks role permission
// Logic: query all permission rules for the role, match each rule, deny takes priority over allow
func (s *RoleService) CheckPermission(ctx context.Context, roleID uint, resource, action string) (bool, error) {
	perms, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		log.ErrorContext(ctx, "check permission failed: db error", "err", err, "role_id", roleID)
		return false, apperr.WrapCode(apperr.CodeInternal, err)
	}

	allowed := false
	for _, p := range perms {
		if model.MatchPermission(p.Resource, p.Action, resource, action) {
			if p.Effect == model.EffectDeny {
				return false, nil
			}
			allowed = true
		}
	}
	return allowed, nil
}
