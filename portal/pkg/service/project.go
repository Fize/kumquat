package service

import (
	"errors"

	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// ProjectService 项目服务
type ProjectService struct {
	db *gorm.DB
}

// NewProjectService 创建项目服务
func NewProjectService(db *gorm.DB) *ProjectService {
	return &ProjectService{db: db}
}

// List 获取项目列表
func (s *ProjectService) List(page, size int) ([]model.Project, int64, error) {
	var projects []model.Project
	var total int64

	if err := s.db.Model(&model.Project{}).Count(&total).Error; err != nil {
		log.Error("list projects failed: count error", "err", err)
		return nil, 0, err
	}

	if err := s.db.Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Order("created_at desc").
		Find(&projects).Error; err != nil {
		log.Error("list projects failed: query error", "err", err)
		return nil, 0, err
	}

	return projects, total, nil
}

// GetByID 根据ID获取项目
func (s *ProjectService) GetByID(id uint) (*model.Project, error) {
	var project model.Project
	if err := s.db.Preload("Module").First(&project, id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

// Create 创建项目
func (s *ProjectService) Create(name string, moduleID uint, config model.JSONConfig) (*model.Project, error) {
	var module model.Module
	if err := s.db.First(&module, moduleID).Error; err != nil {
		log.Warn("create project failed: module not found", "module_id", moduleID)
		return nil, errors.New("module not found")
	}

	project := model.Project{
		Name:     name,
		ModuleID: moduleID,
		Config:   config,
	}

	if err := s.db.Create(&project).Error; err != nil {
		log.Error("create project failed: db error", "err", err, "name", name)
		return nil, err
	}

	project.Module = module
	log.Info("project created", "project_id", project.ID, "name", name, "module_id", moduleID)
	return &project, nil
}

// Update 更新项目
func (s *ProjectService) Update(id uint, name string, config model.JSONConfig) (*model.Project, error) {
	project, err := s.GetByID(id)
	if err != nil {
		log.Warn("update project failed: not found", "project_id", id)
		return nil, errors.New("project not found")
	}

	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if config != nil {
		updates["config"] = config
	}

	if err := s.db.Model(project).Updates(updates).Error; err != nil {
		log.Error("update project failed: db error", "err", err, "project_id", id)
		return nil, err
	}

	log.Info("project updated", "project_id", id)
	return s.GetByID(id)
}

// Delete 删除项目
func (s *ProjectService) Delete(id uint) error {
	project, err := s.GetByID(id)
	if err != nil {
		log.Warn("delete project failed: not found", "project_id", id)
		return errors.New("project not found")
	}

	if err := s.db.Delete(project).Error; err != nil {
		log.Error("delete project failed: db error", "err", err, "project_id", id)
		return err
	}

	log.Info("project deleted", "project_id", id, "name", project.Name)
	return nil
}

// ListByModule 根据模块获取项目
func (s *ProjectService) ListByModule(moduleID uint, page, size int) ([]model.Project, int64, error) {
	var projects []model.Project
	var total int64

	query := s.db.Model(&model.Project{}).Where("module_id = ?", moduleID)
	if err := query.Count(&total).Error; err != nil {
		log.Error("list projects by module failed: count error", "err", err, "module_id", moduleID)
		return nil, 0, err
	}

	if err := query.Preload("Module").
		Offset((page-1)*size).Limit(size).
		Find(&projects).Error; err != nil {
		log.Error("list projects by module failed: query error", "err", err, "module_id", moduleID)
		return nil, 0, err
	}

	return projects, total, nil
}
