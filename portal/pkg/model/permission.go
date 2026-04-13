package model

// Permission 权限模型，ACL 规则表
type Permission struct {
	Base
	RoleID   uint   `json:"role_id" gorm:"index;not null"`
	Role     Role   `json:"role" gorm:"foreignKey:RoleID"`
	Resource string `json:"resource" gorm:"size:64;not null"` // module, project, user, role, *
	Action   string `json:"action" gorm:"size:32;not null"`   // read, write, delete, *
	Effect   string `json:"effect" gorm:"size:16;not null;default:allow"` // allow, deny
}

// TableName 指定表名
func (Permission) TableName() string {
	return "permissions"
}

// PermissionEffect 权限效果常量
const (
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// PredefinedPermissions 预定义角色权限
var PredefinedPermissions = map[string][]Permission{
	RoleAdmin: {
		{Resource: "*", Action: "*", Effect: EffectAllow},
	},
	RoleMember: {
		{Resource: "module", Action: "read", Effect: EffectAllow},
		{Resource: "project", Action: "*", Effect: EffectAllow},
		{Resource: "user", Action: "read", Effect: EffectAllow},
		{Resource: "role", Action: "read", Effect: EffectAllow},
	},
	RoleGuest: {
		{Resource: "*", Action: "read", Effect: EffectAllow},
	},
}

// MatchPermission 检查权限是否匹配（支持 * 通配符）
func MatchPermission(ruleResource, ruleAction, reqResource, reqAction string) bool {
	return (ruleResource == "*" || ruleResource == reqResource) &&
		(ruleAction == "*" || ruleAction == reqAction)
}
