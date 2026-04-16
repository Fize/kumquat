package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchPermission_WildcardResource(t *testing.T) {
	// 通配符 * 匹配任意资源
	assert.True(t, MatchPermission("*", "read", "cluster", "read"))
	assert.True(t, MatchPermission("*", "read", "application", "read"))
	assert.True(t, MatchPermission("*", "read", "anyresource", "read"))
}

func TestMatchPermission_WildcardAction(t *testing.T) {
	// 通配符 * 匹配任意操作
	assert.True(t, MatchPermission("cluster", "*", "cluster", "read"))
	assert.True(t, MatchPermission("cluster", "*", "cluster", "write"))
	assert.True(t, MatchPermission("cluster", "*", "cluster", "delete"))
}

func TestMatchPermission_FullWildcard(t *testing.T) {
	// 完全通配符
	assert.True(t, MatchPermission("*", "*", "anyresource", "anyaction"))
}

func TestMatchPermission_ExactMatch(t *testing.T) {
	// 精确匹配
	assert.True(t, MatchPermission("cluster", "read", "cluster", "read"))
	assert.True(t, MatchPermission("application", "write", "application", "write"))
}

func TestMatchPermission_ResourceMismatch(t *testing.T) {
	// 资源不匹配
	assert.False(t, MatchPermission("cluster", "read", "addon", "read"))
	assert.False(t, MatchPermission("application", "write", "workspace", "write"))
}

func TestMatchPermission_ActionMismatch(t *testing.T) {
	// 操作不匹配
	assert.False(t, MatchPermission("cluster", "read", "cluster", "write"))
	assert.False(t, MatchPermission("cluster", "read", "cluster", "delete"))
}

func TestMatchPermission_BothMismatch(t *testing.T) {
	// 资源和操作都不匹配
	assert.False(t, MatchPermission("cluster", "read", "addon", "write"))
}

func TestMatchPermission_EmptyStrings(t *testing.T) {
	// 空字符串精确匹配空字符串（因为 "" == "" 为 true）
	assert.True(t, MatchPermission("", "", "", ""))
	// 非空字符串与空字符串 - 资源匹配但操作不匹配
	assert.False(t, MatchPermission("cluster", "", "cluster", "write"))
	// 资源和操作都精确匹配
	assert.True(t, MatchPermission("", "read", "", "read"))
}

// ===== PredefinedPermissions 角色权限验证 =====

func TestPredefinedPermissions_Admin(t *testing.T) {
	perms := PredefinedPermissions[RoleAdmin]
	assert.Len(t, perms, 1)

	perm := perms[0]
	assert.Equal(t, ResourceAll, perm.Resource)
	assert.Equal(t, ActionAll, perm.Action)
	assert.Equal(t, EffectAllow, perm.Effect)

	// Admin 可以操作所有资源
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "cluster", "read"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "addon", "write"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "application", "delete"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "workspace", "read"))
}

func TestPredefinedPermissions_Member(t *testing.T) {
	perms := PredefinedPermissions[RoleMember]

	// Member 权限列表
	permMap := make(map[string]Permission)
	for _, p := range perms {
		permMap[p.Resource] = p
	}

	// 应用资源 - 完全权限
	appPerm, ok := permMap[ResourceApplication]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, appPerm.Action)
	assert.Equal(t, EffectAllow, appPerm.Effect)

	// 工作空间 - 完全权限
	wsPerm, ok := permMap[ResourceWorkspace]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, wsPerm.Action)
	assert.Equal(t, EffectAllow, wsPerm.Effect)

	// 模块 - 只读
	modPerm, ok := permMap[ResourceModule]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, modPerm.Action)
	assert.Equal(t, EffectAllow, modPerm.Effect)

	// 项目 - 完全权限
	projPerm, ok := permMap[ResourceProject]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, projPerm.Action)

	// 用户/角色 - 只读
	_, ok = permMap[ResourceUser]
	assert.True(t, ok)
	_, ok = permMap[ResourceRole]
	assert.True(t, ok)

	// cluster/addon - 无配置（隐式拒绝）
	_, hasCluster := permMap[ResourceCluster]
	_, hasAddon := permMap[ResourceAddon]
	assert.False(t, hasCluster)
	assert.False(t, hasAddon)
}

func TestPredefinedPermissions_Guest(t *testing.T) {
	perms := PredefinedPermissions[RoleGuest]

	permMap := make(map[string]Permission)
	for _, p := range perms {
		permMap[p.Resource] = p
	}

	// 应用 - 只读
	appPerm, ok := permMap[ResourceApplication]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, appPerm.Action)
	assert.Equal(t, EffectAllow, appPerm.Effect)

	// 工作空间 - 只读
	wsPerm, ok := permMap[ResourceWorkspace]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, wsPerm.Action)

	// 全局只读
	globalPerm, ok := permMap[ResourceAll]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, globalPerm.Action)
	assert.Equal(t, EffectAllow, globalPerm.Effect)

	// Guest 不能写任何资源
	for _, p := range perms {
		assert.NotEqual(t, ActionWrite, p.Action, "Guest should not have write permission")
		assert.NotEqual(t, ActionDelete, p.Action, "Guest should not have delete permission")
	}
}

func TestPermissionCheck_ImplicitDeny(t *testing.T) {
	// 当权限未配置时，应视为隐式拒绝
	// Member 没有 cluster 权限配置
	memberPerms := PredefinedPermissions[RoleMember]
	for _, p := range memberPerms {
		if p.Resource == ResourceCluster || p.Resource == ResourceAddon {
			t.Errorf("Member should not have %s permission", p.Resource)
		}
	}
}