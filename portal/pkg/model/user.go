package model

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 用户模型
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

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// BeforeCreate 创建前自动加密密码
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hashed)
	}
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
}

// SetPassword 设置明文密码（将在 BeforeCreate 中加密）
func (u *User) SetPassword(password string) {
	u.Password = password
}

// ToResponse 转换为响应结构（不含敏感信息）
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
