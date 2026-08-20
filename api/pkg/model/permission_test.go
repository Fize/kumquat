package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchPermission_WildcardResource(t *testing.T) {
	// Wildcard * matches any resource
	assert.True(t, MatchPermission("*", "read", "cluster", "read"))
	assert.True(t, MatchPermission("*", "read", "application", "read"))
	assert.True(t, MatchPermission("*", "read", "anyresource", "read"))
}

func TestMatchPermission_WildcardAction(t *testing.T) {
	// Wildcard * matches any action
	assert.True(t, MatchPermission("cluster", "*", "cluster", "read"))
	assert.True(t, MatchPermission("cluster", "*", "cluster", "write"))
	assert.True(t, MatchPermission("cluster", "*", "cluster", "delete"))
}

func TestMatchPermission_FullWildcard(t *testing.T) {
	// Full wildcard
	assert.True(t, MatchPermission("*", "*", "anyresource", "anyaction"))
}

func TestMatchPermission_ExactMatch(t *testing.T) {
	// Exact match
	assert.True(t, MatchPermission("cluster", "read", "cluster", "read"))
	assert.True(t, MatchPermission("application", "write", "application", "write"))
}

func TestMatchPermission_ResourceMismatch(t *testing.T) {
	// Resource mismatch
	assert.False(t, MatchPermission("cluster", "read", "addon", "read"))
	assert.False(t, MatchPermission("application", "write", "workspace", "write"))
}

func TestMatchPermission_ActionMismatch(t *testing.T) {
	// Action mismatch
	assert.False(t, MatchPermission("cluster", "read", "cluster", "write"))
	assert.False(t, MatchPermission("cluster", "read", "cluster", "delete"))
}

func TestMatchPermission_BothMismatch(t *testing.T) {
	// Both resource and action mismatch
	assert.False(t, MatchPermission("cluster", "read", "addon", "write"))
}

func TestMatchPermission_EmptyStrings(t *testing.T) {
	// Empty strings match exactly (because "" == "" is true)
	assert.True(t, MatchPermission("", "", "", ""))
	// Non-empty string with empty string - resource matches but action doesn't
	assert.False(t, MatchPermission("cluster", "", "cluster", "write"))
	// Both resource and action match exactly
	assert.True(t, MatchPermission("", "read", "", "read"))
}

// ===== PredefinedPermissions role permission verification =====

func TestPredefinedPermissions_Admin(t *testing.T) {
	perms := PredefinedPermissions[RoleAdmin]
	assert.Len(t, perms, 1)

	perm := perms[0]
	assert.Equal(t, ResourceAll, perm.Resource)
	assert.Equal(t, ActionAll, perm.Action)
	assert.Equal(t, EffectAllow, perm.Effect)

	// Admin can operate on all resources
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "cluster", "read"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "addon", "write"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "application", "delete"))
	assert.True(t, MatchPermission(perm.Resource, perm.Action, "workspace", "read"))
}

func TestPredefinedPermissions_Member(t *testing.T) {
	perms := PredefinedPermissions[RoleMember]

	// Member permission list
	permMap := make(map[string]Permission)
	for _, p := range perms {
		permMap[p.Resource] = p
	}

	// Application resource - full permission
	appPerm, ok := permMap[ResourceApplication]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, appPerm.Action)
	assert.Equal(t, EffectAllow, appPerm.Effect)

	// Workspace - full permission
	wsPerm, ok := permMap[ResourceWorkspace]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, wsPerm.Action)
	assert.Equal(t, EffectAllow, wsPerm.Effect)

	// Module - read-only
	modPerm, ok := permMap[ResourceModule]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, modPerm.Action)
	assert.Equal(t, EffectAllow, modPerm.Effect)

	// Project - full permission
	projPerm, ok := permMap[ResourceProject]
	assert.True(t, ok)
	assert.Equal(t, ActionAll, projPerm.Action)

	// User/Role - read-only
	_, ok = permMap[ResourceUser]
	assert.True(t, ok)
	_, ok = permMap[ResourceRole]
	assert.True(t, ok)

	// cluster/addon - no configuration (implicit deny)
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

	// Application - read-only
	appPerm, ok := permMap[ResourceApplication]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, appPerm.Action)
	assert.Equal(t, EffectAllow, appPerm.Effect)

	// Workspace - read-only
	wsPerm, ok := permMap[ResourceWorkspace]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, wsPerm.Action)

	// Global read-only
	globalPerm, ok := permMap[ResourceAll]
	assert.True(t, ok)
	assert.Equal(t, ActionRead, globalPerm.Action)
	assert.Equal(t, EffectAllow, globalPerm.Effect)

	// Guest cannot write any resources
	for _, p := range perms {
		assert.NotEqual(t, ActionWrite, p.Action, "Guest should not have write permission")
		assert.NotEqual(t, ActionDelete, p.Action, "Guest should not have delete permission")
	}
}

func TestPermissionCheck_ImplicitDeny(t *testing.T) {
	// When permission is not configured, it should be considered implicit deny
	// Member does not have cluster permission configuration
	memberPerms := PredefinedPermissions[RoleMember]
	for _, p := range memberPerms {
		if p.Resource == ResourceCluster || p.Resource == ResourceAddon {
			t.Errorf("Member should not have %s permission", p.Resource)
		}
	}
}
