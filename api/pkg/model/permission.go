package model

// Resource type constants
type ResourceType string

const (
	// Original resources
	ResourceModule  = "module"
	ResourceProject = "project"
	ResourceUser    = "user"
	ResourceRole    = "role"
	ResourceAll     = "*"

	// New K8s CRD resources
	ResourceCluster     = "cluster"     // Cluster resource - Admin only
	ResourceAddon       = "addon"       // Addon resource - Admin only
	ResourceApplication = "application" // Application resource - Admin/Member
	ResourceWorkspace   = "workspace"   // Workspace - Admin/Member
)

// Action type constants
const (
	ActionRead   = "read"
	ActionWrite  = "write"
	ActionDelete = "delete"
	ActionAll    = "*"
)

// Permission permission model, ACL rule table
type Permission struct {
	Base
	RoleID   uint   `json:"role_id" gorm:"index;not null"`
	Role     Role   `json:"role" gorm:"foreignKey:RoleID"`
	Resource string `json:"resource" gorm:"size:64;not null"` // module, project, user, role, cluster, addon, application, workspace, *
	Action   string `json:"action" gorm:"size:32;not null"`   // read, write, delete, *
	Effect   string `json:"effect" gorm:"size:16;not null;default:allow"` // allow, deny
}

// TableName specifies table name
func (Permission) TableName() string {
	return "permissions"
}

// PermissionEffect permission effect constants
const (
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// PredefinedPermissions predefined role permissions
// Admin: manage all resources
// Member: developer role, only manage applications, no system resource permissions
// Guest: visitor, read-only all resources
var PredefinedPermissions = map[string][]Permission{
	RoleAdmin: {
		{Resource: ResourceAll, Action: ActionAll, Effect: EffectAllow},
	},
	RoleMember: {
		// System resources - Member has no permissions (not configured means denied)
		// System resources like cluster, addon are not visible to Member

		// Application resources - Member full permissions
		{Resource: ResourceApplication, Action: ActionAll, Effect: EffectAllow},

		// Workspace - Member full permissions (for application grouping)
		{Resource: ResourceWorkspace, Action: ActionAll, Effect: EffectAllow},

		// Original permissions
		{Resource: ResourceModule, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceProject, Action: ActionAll, Effect: EffectAllow},
		{Resource: ResourceUser, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceRole, Action: ActionRead, Effect: EffectAllow},
	},
	RoleGuest: {
		// Guest read-only applications and workspaces
		{Resource: ResourceApplication, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceWorkspace, Action: ActionRead, Effect: EffectAllow},
		{Resource: ResourceAll, Action: ActionRead, Effect: EffectAllow},
	},
}

// MatchPermission checks if permission matches (supports * wildcard)
func MatchPermission(ruleResource, ruleAction, reqResource, reqAction string) bool {
	return (ruleResource == "*" || ruleResource == reqResource) &&
		(ruleAction == "*" || ruleAction == reqAction)
}
