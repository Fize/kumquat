package service

import (
	"errors"

	"github.com/casbin/casbin/v2"
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
		return nil, errors.New("role not found")
	}

	policies, err := s.enforcer.GetPermissionsForUser(role.Name)
	if err != nil {
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
					return err
				}
			} else {
				return err
			}
		}

		// 添加 Casbin 策略
		for _, perm := range perms {
			parts := splitPermission(perm)
			if len(parts) == 2 {
				s.enforcer.AddPolicy(roleName, parts[0], parts[1])
			}
		}
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
