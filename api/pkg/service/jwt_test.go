package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTService_GenerateAndParseToken(t *testing.T) {
	service := NewJWTService("test-secret", time.Hour, 10*time.Minute)

	token, err := service.GenerateToken(1, "testuser", 2, "admin")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := service.ParseToken(token)
	require.NoError(t, err)
	assert.Equal(t, uint(1), claims.UserID)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, uint(2), claims.RoleID)
	assert.Equal(t, "admin", claims.RoleName)
}

func TestJWTService_ParseToken_InvalidToken(t *testing.T) {
	service := NewJWTService("test-secret", time.Hour, 10*time.Minute)

	_, err := service.ParseToken("invalid-token")
	assert.Error(t, err)
}

func TestJWTService_ParseToken_WrongSecret(t *testing.T) {
	service1 := NewJWTService("secret1", time.Hour, 10*time.Minute)
	service2 := NewJWTService("secret2", time.Hour, 10*time.Minute)

	token, err := service1.GenerateToken(1, "testuser", 2, "admin")
	require.NoError(t, err)

	_, err = service2.ParseToken(token)
	assert.Error(t, err)
}

func TestJWTService_GetExpireDuration(t *testing.T) {
	service := NewJWTService("test-secret", 2*time.Hour, 30*time.Minute)

	assert.Equal(t, 2*time.Hour, service.GetExpireDuration())
	assert.Equal(t, 30*time.Minute, service.GetResetExpireDuration())
}
