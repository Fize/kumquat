package model

import (
	"encoding/json"
)

// JSONConfig JSON config type
type JSONConfig map[string]interface{}

// Value implements driver.Valuer
func (j JSONConfig) Value() (interface{}, error) {
	if j == nil {
		return nil, nil
	}
	data, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// Scan implements sql.Scanner
func (j *JSONConfig) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, j)
}

// Project project model
type Project struct {
	Base
	Name     string     `json:"name" gorm:"not null;size:128"`
	ModuleID uint       `json:"module_id" gorm:"not null;index"`
	Module   Module     `json:"module" gorm:"foreignKey:ModuleID"`
	Config   JSONConfig `json:"config,omitempty" gorm:"type:text"`
}

// TableName specifies table name
func (Project) TableName() string {
	return "projects"
}

// ToResponse converts to response structure
func (p *Project) ToResponse() map[string]interface{} {
	resp := map[string]interface{}{
		"id":         p.ID,
		"name":       p.Name,
		"module_id":  p.ModuleID,
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	}
	if p.Module.ID > 0 {
		resp["module"] = p.Module.ToResponse()
	}
	if p.Config != nil {
		resp["config"] = p.Config
	}
	return resp
}
