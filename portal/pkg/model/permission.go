package model

// 资源类型常量
type ResourceType string

const (
	// 原有资源
	ResourceModule  = "module"
	ResourceProject = "project"
	ResourceUser    = "user"
	ResourceRole    = "role"
	ResourceAll     = "*"

	// 新增 K8s CRD 资源
	ResourceCluster     = "cluster"     // 集群资源 - 仅 Admin
	ResourceAddon       = "addon"       // 插件资源 - 仅 Admin
	ResourceApplication = "application" // 应用资源 - Admin/Member
	ResourceWorkspace   = "workspace"   // 工作空间 - Admin/Member
)

// 操作类型常量
const (
	ActionRead   = "read"
	ActionWrite  = "write"
	ActionDelete = "delete"
	ActionAll    = "*"
)

// Permission 权限模型，ACL 规则表
type Permission struct {
	Base
	RoleID   uint   `json:"role_id" gorm:"index;not null"`
	Role     Role   `json:"role" gorm:"foreignKey:RoleID"`
	Resource string `json:"resource" gorm:"size:64;not null"` // module, project, user, role, cluster, addon, application, workspace, *
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
// Admin: 管理所有资源
// Member: 开发者角色，只管理应用，无系统资源权限
// Guest: 访客，只读所有资源
var PredefinedPermissions = map[string][]Permission{
	RoleAdmin: {
		{Resource: ResourceAll, Action: ActionAll, Effect: EffectAllow},
	},
	RoleMember: {
		// 系统资源 - Member 无权限（不配置即拒绝）
		// cluster, addon 等系统资源 Member 不可见

		// 应用资源 - Member 完全权限
		{Resource: ResourceApplication, Action: ActionAll, Effect: EffectAllow},

		// 工作空间 - Member 完全权限（用于应用分组）
		{Resource: ResourceWorkspace, Action: ActionAll, Effect: EffectAllow},

		// 原有权限
		{Resource: ResourceModule, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceProject, Action: ActionAll, Effect: EffectAllow},
		{Resource: ResourceUser, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceRole, Action: ActionRead, Effect: EffectAllow},
	},
	RoleGuest: {
		// Guest 只读应用和工作空间
		{Resource: ResourceApplication, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceWorkspace, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceAll, Action: ActionRead, Effect: EffectAllow},
	},
}

// MatchPermission 检查权限是否匹配（支持 * 通配符）
func MatchPermission(ruleResource, ruleAction, reqResource, reqAction string) bool {
	return (ruleResource == "*" || ruleResource == reqResource) &&
		(ruleAction == "*" || ruleAction == reqAction)
}
