package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchPermission(t *testing.T) {
	tests := []struct {
		name       string
		ruleRes    string
		ruleAction string
		reqRes     string
		reqAction  string
		expected   bool
	}{
		{"wildcard both", "*", "*", "module", "read", true},
		{"wildcard resource", "*", "read", "module", "read", true},
		{"wildcard action", "module", "*", "module", "delete", true},
		{"exact match", "module", "read", "module", "read", true},
		{"resource mismatch", "module", "read", "project", "read", false},
		{"action mismatch", "module", "read", "module", "delete", false},
		{"both mismatch", "module", "read", "project", "delete", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchPermission(tt.ruleRes, tt.ruleAction, tt.reqRes, tt.reqAction)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUser_CheckPassword(t *testing.T) {
	u := &User{}
	u.SetPassword("mypassword")
	// Manually trigger hash since GORM hooks don't run in unit tests
	require.NoError(t, u.hashPasswordIfNeeded())

	assert.True(t, u.CheckPassword("mypassword"))
	assert.False(t, u.CheckPassword("wrongpassword"))
}

func TestUser_ToResponse(t *testing.T) {
	u := &User{
		Username: "testuser",
		Email:    "test@example.com",
		Nickname: "Test",
		RoleID:   2,
	}
	u.ID = 1

	resp := u.ToResponse()
	assert.Equal(t, uint(1), resp["id"])
	assert.Equal(t, "testuser", resp["username"])
	assert.Equal(t, "test@example.com", resp["email"])
	assert.Equal(t, uint(2), resp["role_id"])
	// No role loaded, should not have role key
	_, hasRole := resp["role"]
	assert.False(t, hasRole)
}

func TestUser_ToResponse_WithRole(t *testing.T) {
	role := Role{Name: "admin"}
	role.ID = 2

	u := &User{
		Username: "testuser",
		Email:    "test@example.com",
		RoleID:   2,
		Role:     role,
	}
	u.ID = 1

	resp := u.ToResponse()
	assert.Equal(t, uint(2), resp["role_id"])
	_, hasRole := resp["role"]
	assert.True(t, hasRole)
}
