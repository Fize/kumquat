package model

// Predefined roles
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleGuest  = "guest"
)

// Role 角色模型
type Role struct {
	Base
	Name string `json:"name" gorm:"uniqueIndex;not null;size:32"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "roles"
}

// ToResponse 转换为响应结构
func (r *Role) ToResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":         r.ID,
		"name":       r.Name,
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
}
