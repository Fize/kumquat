package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MaxModuleLevel maximum module level
const MaxModuleLevel = 5

// Module module model (tree structure)
type Module struct {
	Base
	Name     string   `json:"name" gorm:"not null;size:64"`
	ParentID *uint    `json:"parent_id,omitempty" gorm:"index"`
	Parent   *Module  `json:"parent,omitempty" gorm:"foreignKey:ParentID"`
	Level    int      `json:"level" gorm:"not null;default:1"`
	Sort     int      `json:"sort" gorm:"default:0"`
	Path     string   `json:"path" gorm:"index;size:256"`
	Children []Module `json:"children,omitempty" gorm:"-"`
}

// TableName specifies table name
func (Module) TableName() string {
	return "modules"
}

// BeforeCreate validates and calculates before creation
func (m *Module) BeforeCreate(tx *gorm.DB) error {
	if m.ParentID != nil {
		var parent Module
		if err := tx.First(&parent, *m.ParentID).Error; err != nil {
			return fmt.Errorf("parent module not found: %w", err)
		}
		m.Level = parent.Level + 1
		if m.Level > MaxModuleLevel {
			return fmt.Errorf("module level cannot exceed %d", MaxModuleLevel)
		}
		m.Path = parent.Path + "/" + m.Name
	} else {
		m.Level = 1
		m.Path = "/" + m.Name
	}

	return nil
}

// GetPathSegments gets path segments
func (m *Module) GetPathSegments() []string {
	if m.Path == "" {
		return []string{}
	}
	segments := strings.Split(m.Path, "/")
	var result []string
	for _, s := range segments {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// ToResponse converts to response structure
func (m *Module) ToResponse() map[string]interface{} {
	resp := map[string]interface{}{
		"id":         m.ID,
		"name":       m.Name,
		"parent_id":  m.ParentID,
		"level":      m.Level,
		"sort":       m.Sort,
		"path":       m.Path,
		"created_at": m.CreatedAt,
		"updated_at": m.UpdatedAt,
	}

	if len(m.Children) > 0 {
		children := make([]map[string]interface{}, len(m.Children))
		for i, child := range m.Children {
			children[i] = child.ToResponse()
		}
		resp["children"] = children
	}

	return resp
}
