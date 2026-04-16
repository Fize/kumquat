package model

// Predefined roles
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleGuest  = "guest"
)

// Role role model
type Role struct {
	Base
	Name string `json:"name" gorm:"uniqueIndex;not null;size:32"`
}

// TableName specifies table name
func (Role) TableName() string {
	return "roles"
}

// ToResponse converts to response structure
func (r *Role) ToResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":         r.ID,
		"name":       r.Name,
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
}
