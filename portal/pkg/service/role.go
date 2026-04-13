package service

import (
	"errors"

	"github.com/casbin/casbin/v2"
	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// PredefinedRolePermissions 预定义角色权限
var PredefinedRolePermissions = map[string][]string{
	model.RoleAdmin:  {"*:*"},
	model.RoleMember: {"module:*", "project:*"},
	model.RoleGuest:  {"*:read"},
}

// RoleService 角色服务
type RoleService struct {
	db       *gorm.DB
	enforcer *casbin.Enforcer
}

// NewRoleService 创建角色服务
func NewRoleService(db *gorm.DB, enforcer *casbin.Enforcer) *RoleService {
	return &RoleService{db: db, enforcer: enforcer}
}

// List 获取角色列表
func (s *RoleService) List() ([]model.Role, error) {
	var roles []model.Role
	if err := s.db.Find(&roles).Error; err != nil {
		log.Error("list roles failed", "err", err)
		return nil, err
	}
	return roles, nil
}

// GetByID 根据ID获取角色
func (s *RoleService) GetByID(id uint) (*model.Role, error) {
	var role model.Role
	if err := s.db.First(&role, id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// GetPermissions 获取角色权限
func (s *RoleService) GetPermissions(roleID uint) ([]string, error) {
	role, err := s.GetByID(roleID)
	if err != nil {
		log.Warn("get permissions failed: role not found", "role_id", roleID)
		return nil, errors.New("role not found")
	}

	policies, err := s.enforcer.GetPermissionsForUser(role.Name)
	if err != nil {
		log.Error("get permissions failed: casbin error", "err", err, "role", role.Name)
		return nil, err
	}

	var perms []string
	for _, p := range policies {
		if len(p) >= 3 {
			perms = append(perms, p[1]+":"+p[2])
		}
	}
	return perms, nil
}

// InitRoles 初始化预定义角色
func (s *RoleService) InitRoles() error {
	for roleName, perms := range PredefinedRolePermissions {
		var role model.Role
		if err := s.db.Where("name = ?", roleName).First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				role = model.Role{Name: roleName}
				if err := s.db.Create(&role).Error; err != nil {
					log.Error("init roles failed: create role error", "err", err, "role", roleName)
					return err
				}
				log.Info("role created", "role_id", role.ID, "role", roleName)
			} else {
				log.Error("init roles failed: query role error", "err", err, "role", roleName)
				return err
			}
		}

		for _, perm := range perms {
			parts := splitPermission(perm)
			if len(parts) == 2 {
				s.enforcer.AddPolicy(roleName, parts[0], parts[1])
			}
		}
		log.Info("role policies loaded", "role", roleName, "permissions", perms)
	}
	return nil
}

func splitPermission(perm string) []string {
	for i, c := range perm {
		if c == ':' {
			return []string{perm[:i], perm[i+1:]}
		}
	}
	return []string{perm}
}

// CheckPermission 检查角色权限
func (s *RoleService) CheckPermission(roleName, resource, action string) (bool, error) {
	return s.enforcer.Enforce(roleName, resource, action)
}
