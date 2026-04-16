package service

import (
	"context"
	"errors"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
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
	project, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "update project failed: not found", "project_id", id)
		return nil, apperr.New(apperr.CodeProjectNotFound, "")
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
