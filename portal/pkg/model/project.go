package model

import (
	"encoding/json"
)

// JSONConfig JSON 配置类型
type JSONConfig map[string]interface{}

// Value 实现 driver.Valuer
func (j JSONConfig) Value() (interface{}, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner
func (j *JSONConfig) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// Project 项目模型
type Project struct {
	Base
	Name     string     `json:"name" gorm:"not null;size:128"`
	ModuleID uint       `json:"module_id" gorm:"not null;index"`
	Module   Module     `json:"module" gorm:"foreignKey:ModuleID"`
	Config   JSONConfig `json:"config,omitempty" gorm:"type:text"`
}

// TableName 指定表名
func (Project) TableName() string {
	return "projects"
}

// ToResponse 转换为响应结构
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
