package service

import (
	"context"
	"errors"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/apiserver/pkg/errors"
	"github.com/fize/kumquat/apiserver/pkg/model"
	"github.com/fize/kumquat/apiserver/pkg/repository"
	"gorm.io/gorm"
)

// ProjectService project service
type ProjectService struct {
	repo repository.ProjectRepository
	db   *gorm.DB
}

// NewProjectService creates project service
func NewProjectService(repo repository.ProjectRepository, db *gorm.DB) *ProjectService {
	return &ProjectService{repo: repo, db: db}
}

// List gets project list
func (s *ProjectService) List(ctx context.Context, page, size int) ([]model.Project, int64, error) {
	projects, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		log.ErrorContext(ctx, "list projects failed", "err", err)
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return projects, total, nil
}

func (s *ProjectService) ListScoped(ctx context.Context, modulePublicID string, page, size int) ([]model.Project, int64, error) {
	ids, err := s.scopedModuleIDs(ctx, modulePublicID)
	if err != nil {
		return nil, 0, err
	}
	query := s.db.WithContext(ctx).Model(&model.Project{}).Where("module_id IN ?", ids)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	var projects []model.Project
	if err := query.Preload("Module").Order("created_at desc").Offset((page - 1) * size).Limit(size).Find(&projects).Error; err != nil {
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return projects, total, nil
}

func (s *ProjectService) CanAccess(ctx context.Context, modulePublicID, projectPublicID string) (bool, error) {
	ids, err := s.scopedModuleIDs(ctx, modulePublicID)
	if err != nil {
		return false, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Project{}).Where("public_id = ? AND module_id IN ?", projectPublicID, ids).Count(&count).Error; err != nil {
		return false, err
	}
	return count == 1, nil
}

func (s *ProjectService) scopedModuleIDs(ctx context.Context, publicID string) ([]uint, error) {
	var root model.Module
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&root).Error; err != nil {
		return nil, apperr.New(apperr.CodeModuleNotFound, "")
	}
	ids, err := descendantModuleIDs(s.db.WithContext(ctx), root.ID)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return ids, nil
}

// GetByID gets project by ID
func (s *ProjectService) GetByID(ctx context.Context, id uint) (*model.Project, error) {
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeProjectNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return project, nil
}

func (s *ProjectService) GetByPublicID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	if err := s.db.WithContext(ctx).Preload("Module").Where("public_id = ?", id).First(&p).Error; err != nil {
		return nil, apperr.New(apperr.CodeProjectNotFound, "")
	}
	return &p, nil
}
func (s *ProjectService) CreateWithModulePublicID(ctx context.Context, name, moduleID string, config model.JSONConfig) (*model.Project, error) {
	var m model.Module
	if err := s.db.WithContext(ctx).Where("public_id = ?", moduleID).First(&m).Error; err != nil {
		return nil, apperr.New(apperr.CodeModuleNotFound, "")
	}
	return s.Create(ctx, name, m.ID, config)
}
func (s *ProjectService) UpdateByPublicID(ctx context.Context, id, name, moduleID string, config model.JSONConfig) (*model.Project, error) {
	p, err := s.GetByPublicID(ctx, id)
	if err != nil {
		return nil, err
	}
	if moduleID != "" && moduleID != p.Module.PublicID {
		return nil, apperr.New(apperr.CodeConflict, "project module association is immutable")
	}
	return s.Update(ctx, p.ID, name, config)
}
func (s *ProjectService) DeleteByPublicID(ctx context.Context, id string) error {
	p, err := s.GetByPublicID(ctx, id)
	if err != nil {
		return err
	}
	return s.Delete(ctx, p.ID)
}
func (s *ProjectService) ListByModulePublicID(ctx context.Context, moduleID string, page, size int) ([]model.Project, int64, error) {
	var module model.Module
	if err := s.db.WithContext(ctx).Where("public_id = ?", moduleID).First(&module).Error; err != nil {
		return nil, 0, apperr.New(apperr.CodeModuleNotFound, "")
	}
	return s.ListByModule(ctx, module.ID, page, size)
}

// Create creates a project
func (s *ProjectService) Create(ctx context.Context, name string, moduleID uint, config model.JSONConfig) (*model.Project, error) {
	exists, err := s.repo.ExistsModule(ctx, moduleID)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	if !exists {
		log.WarnContext(ctx, "create project failed: module not found", "module_id", moduleID)
		return nil, apperr.New(apperr.CodeModuleNotFound, "")
	}

	project := model.Project{
		Name:     name,
		ModuleID: moduleID,
		Config:   config,
	}

	if err := s.repo.Create(ctx, &project); err != nil {
		log.ErrorContext(ctx, "create project failed: db error", "err", err, "name", name)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "project created", "project_id", project.ID, "name", name, "module_id", moduleID)
	return s.repo.GetByID(ctx, project.ID)
}

// Update updates a project
func (s *ProjectService) Update(ctx context.Context, id uint, name string, config model.JSONConfig) (*model.Project, error) {
	return s.UpdateWithModule(ctx, id, name, 0, config)
}

func (s *ProjectService) UpdateWithModule(ctx context.Context, id uint, name string, moduleID uint, config model.JSONConfig) (*model.Project, error) {
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "update project failed: not found", "project_id", id)
		return nil, apperr.New(apperr.CodeProjectNotFound, "")
	}
	if moduleID != 0 && moduleID != project.ModuleID {
		return nil, apperr.New(apperr.CodeConflict, "project module association is immutable")
	}

	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	if config != nil {
		updates["config"] = config
	}

	if err := s.repo.Update(ctx, project, updates); err != nil {
		log.ErrorContext(ctx, "update project failed: db error", "err", err, "project_id", id)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "project updated", "project_id", id)
	return s.repo.GetByID(ctx, id)
}

// Delete deletes a project
func (s *ProjectService) Delete(ctx context.Context, id uint) error {
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "delete project failed: not found", "project_id", id)
		return apperr.New(apperr.CodeProjectNotFound, "")
	}
	var workspaces int64
	if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("kind = ? AND project_id = ? AND archived_at IS NULL", model.ResourceWorkspace, id).Count(&workspaces).Error; err != nil {
		return apperr.WrapCode(apperr.CodeInternal, err)
	}
	if workspaces > 0 {
		return apperr.New(apperr.CodeConflict, "project is still referenced by workspaces")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		log.ErrorContext(ctx, "delete project failed: db error", "err", err, "project_id", id)
		return apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "project deleted", "project_id", id, "name", project.Name)
	return nil
}

// ListByModule gets projects by module
func (s *ProjectService) ListByModule(ctx context.Context, moduleID uint, page, size int) ([]model.Project, int64, error) {
	projects, total, err := s.repo.ListByModuleID(ctx, moduleID, page, size)
	if err != nil {
		log.ErrorContext(ctx, "list projects by module failed", "err", err, "module_id", moduleID)
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return projects, total, nil
}
