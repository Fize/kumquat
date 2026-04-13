package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MaxModuleLevel 最大模块层级
const MaxModuleLevel = 5

// Module 模块模型（树形结构）
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

// TableName 指定表名
func (Module) TableName() string {
	return "modules"
}

// BeforeCreate 创建前校验和计算
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

// GetPathSegments 获取路径段
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

// ToResponse 转换为响应结构
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
