package service

import (
	"context"
	"errors"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// RoleService 角色服务
type RoleService struct {
	db *gorm.DB
}

// NewRoleService 创建角色服务
func NewRoleService(db *gorm.DB) *RoleService {
	return &RoleService{db: db}
}

// List 获取角色列表
func (s *RoleService) List(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := s.db.Find(&roles).Error; err != nil {
		log.ErrorContext(ctx, "list roles failed", "err", err)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return roles, nil
}

// GetByID 根据ID获取角色
func (s *RoleService) GetByID(ctx context.Context, id uint) (*model.Role, error) {
	var role model.Role
	if err := s.db.First(&role, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeRoleNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return &role, nil
}

// GetPermissions 获取角色权限
func (s *RoleService) GetPermissions(ctx context.Context, roleID uint) ([]model.Permission, error) {
	role, err := s.GetByID(ctx, roleID)
	if err != nil {
		log.WarnContext(ctx, "get permissions failed: role not found", "role_id", roleID)
		return nil, err
	}

	var perms []model.Permission
	if err := s.db.Where("role_id = ?", role.ID).Find(&perms).Error; err != nil {
		log.ErrorContext(ctx, "get permissions failed: db error", "err", err, "role_id", role.ID)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return perms, nil
}

// InitRoles 初始化预定义角色和权限
func (s *RoleService) InitRoles() error {
	for roleName, permissions := range model.PredefinedPermissions {
		var role model.Role
		if err := s.db.Where("name = ?", roleName).First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				role = model.Role{Name: roleName}
				if err := s.db.Create(&role).Error; err != nil {
					log.Error("init roles failed: create role error", "err", err, "role", roleName)
					return apperr.WrapCode(apperr.CodeInternal, err)
				}
				log.Info("role created", "role_id", role.ID, "role", roleName)
			} else {
				log.Error("init roles failed: query role error", "err", err, "role", roleName)
				return apperr.WrapCode(apperr.CodeInternal, err)
			}
		}

		// 初始化权限：仅当角色无权限记录时才写入预定义权限
		var count int64
		s.db.Model(&model.Permission{}).Where("role_id = ?", role.ID).Count(&count)
		if count == 0 {
			for _, p := range permissions {
				perm := model.Permission{
					RoleID:   role.ID,
					Resource: p.Resource,
					Action:   p.Action,
					Effect:   p.Effect,
				}
				if err := s.db.Create(&perm).Error; err != nil {
					log.Error("init permissions failed", "err", err, "role", roleName, "resource", p.Resource, "action", p.Action)
					return apperr.WrapCode(apperr.CodeInternal, err)
				}
			}
			log.Info("role permissions initialized", "role", roleName, "count", len(permissions))
		}
	}
	return nil
}

// CheckPermission 检查角色权限
// 逻辑：查询该角色的所有权限规则，逐条匹配，deny 优先于 allow
func (s *RoleService) CheckPermission(ctx context.Context, roleID uint, resource, action string) (bool, error) {
	var perms []model.Permission
	if err := s.db.Where("role_id = ?", roleID).Find(&perms).Error; err != nil {
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
