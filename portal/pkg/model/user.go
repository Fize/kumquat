package model

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User user model
type User struct {
	Base
	Username string  `json:"username" gorm:"uniqueIndex;not null;size:64"`
	Email    string  `json:"email" gorm:"uniqueIndex;not null;size:128"`
	Password string  `json:"-" gorm:"not null;size:256"`
	Nickname string  `json:"nickname" gorm:"size:64"`
	RoleID   uint    `json:"role_id" gorm:"not null;default:3"`
	Role     Role    `json:"role" gorm:"foreignKey:RoleID"`
	ModuleID *uint   `json:"module_id,omitempty" gorm:"index"`
	Module   *Module `json:"module,omitempty" gorm:"foreignKey:ModuleID"`
}

// TableName specifies table name
func (User) TableName() string {
	return "users"
}

// BeforeCreate automatically encrypts password before creation
func (u *User) BeforeCreate(tx *gorm.DB) error {
	return u.hashPasswordIfNeeded()
}

// BeforeUpdate automatically encrypts password before update (if password is modified)
func (u *User) BeforeUpdate(tx *gorm.DB) error {
	return u.hashPasswordIfNeeded()
}

// hashPasswordIfNeeded encrypts password if it's not empty and not in bcrypt hash format
func (u *User) hashPasswordIfNeeded() error {
	if u.Password != "" && !strings.HasPrefix(u.Password, "$2a$") {
		hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hashed)
	}
	return nil
}

// CheckPassword verifies password
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
}

// SetPassword sets plaintext password (will be encrypted in BeforeCreate)
func (u *User) SetPassword(password string) {
	u.Password = password
}

// ToResponse converts to response structure (without sensitive information)
func (u *User) ToResponse() map[string]interface{} {
	resp := map[string]interface{}{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"nickname":   u.Nickname,
		"role_id":    u.RoleID,
		"created_at": u.CreatedAt,
		"updated_at": u.UpdatedAt,
	}
	if u.Role.ID > 0 {
		resp["role"] = u.Role.ToResponse()
	}
	if u.Module != nil {
		resp["module_id"] = u.ModuleID
		resp["module"] = u.Module.ToResponse()
	}
	return resp
}
