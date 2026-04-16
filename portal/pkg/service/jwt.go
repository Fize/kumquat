package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT claims
type Claims struct {
	UserID   uint   `json:"userId"`
	Username string `json:"username"`
	RoleID   uint   `json:"roleId"`
	RoleName string `json:"roleName"`
	jwt.RegisteredClaims
}

// JWTService JWT service
type JWTService struct {
	secret              []byte
	expireDuration      time.Duration
	resetExpireDuration time.Duration
}

// NewJWTService creates JWT service
func NewJWTService(secret string, expire, resetExpire time.Duration) *JWTService {
	return &JWTService{
		secret:              []byte(secret),
		expireDuration:      expire,
		resetExpireDuration: resetExpire,
	}
}

// GenerateToken generates JWT token
func (s *JWTService) GenerateToken(userID uint, username string, roleID uint, roleName string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RoleID:   roleID,
		RoleName: roleName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expireDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "kumquat-portal",
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ParseToken parses JWT token
func (s *JWTService) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// GetExpireDuration gets expiration duration
func (s *JWTService) GetExpireDuration() time.Duration {
	return s.expireDuration
}

// GetResetExpireDuration gets password reset expiration duration
func (s *JWTService) GetResetExpireDuration() time.Duration {
	return s.resetExpireDuration
}
