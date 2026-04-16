package service

import (
	"context"
	"errors"
	"sort"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
	"gorm.io/gorm"
)

// ModuleService module service

type ModuleService struct {
	repo repository.ModuleRepository
	db   *gorm.DB // reserved for recursive deletion transaction
}

// NewModuleService creates a module service
func NewModuleService(repo repository.ModuleRepository, db *gorm.DB) *ModuleService {
	return &ModuleService{repo: repo, db: db}
}

// List gets module list (tree structure)
func (s *ModuleService) List(ctx context.Context) ([]model.Module, error) {
	modules, err := s.repo.List(ctx)
	if err != nil {
		log.ErrorContext(ctx, "list modules failed", "err", err)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return s.buildTree(modules), nil
}

// buildTree builds tree structure
func (s *ModuleService) buildTree(modules []model.Module) []model.Module {
	moduleMap := make(map[uint][]model.Module)
	var roots []model.Module

	for _, m := range modules {
		if m.ParentID == nil {
			roots = append(roots, m)
		} else {
			moduleMap[*m.ParentID] = append(moduleMap[*m.ParentID], m)
		}
	}

	var buildChildren func(parent *model.Module)
	buildChildren = func(parent *model.Module) {
		children := moduleMap[parent.ID]
		sort.Slice(children, func(i, j int) bool { return children[i].Sort < children[j].Sort })
		parent.Children = children
		for i := range parent.Children {
			buildChildren(&parent.Children[i])
		}
	}

	sort.Slice(roots, func(i, j int) bool { return roots[i].Sort < roots[j].Sort })
	for i := range roots {
		buildChildren(&roots[i])
	}

	return roots
}

// GetByID gets module by ID
func (s *ModuleService) GetByID(ctx context.Context, id uint) (*model.Module, error) {
	module, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeModuleNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return module, nil
}

// Create creates a module
func (s *ModuleService) Create(ctx context.Context, name string, parentID *uint, sort int) (*model.Module, error) {
	if parentID != nil {
		parent, err := s.repo.GetByID(ctx, *parentID)
		if err != nil {
			log.WarnContext(ctx, "create module failed: parent not found", "parent_id", *parentID)
			return nil, apperr.New(apperr.CodeModuleNotFound, "parent module not found")
		}
		if parent.Level >= model.MaxModuleLevel {
			log.WarnContext(ctx, "create module failed: parent at max level", "parent_id", *parentID, "level", parent.Level)
			return nil, apperr.New(apperr.CodeBadRequest, "parent module already at max level")
		}
	}

	module := model.Module{
		Name:     name,
		ParentID: parentID,
		Sort:     sort,
	}

	if err := s.repo.Create(ctx, &module); err != nil {
		log.ErrorContext(ctx, "create module failed: db error", "err", err, "name", name)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "module created", "module_id", module.ID, "name", name, "parent_id", parentID)
	return s.repo.GetByID(ctx, module.ID)
}

// Update updates a module
func (s *ModuleService) Update(ctx context.Context, id uint, name string, sort int) (*model.Module, error) {
	module, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "update module failed: not found", "module_id", id)
		return nil, apperr.New(apperr.CodeModuleNotFound, "")
	}

	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	updates["sort"] = sort

	if err := s.repo.Update(ctx, module, updates); err != nil {
		log.ErrorContext(ctx, "update module failed: db error", "err", err, "module_id", id)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "module updated", "module_id", id, "name", name)
	return s.repo.GetByID(ctx, id)
}

// Delete deletes a module (recursively deletes child modules, using transaction)
func (s *ModuleService) Delete(ctx context.Context, id uint) error {
	// First check if module exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "delete module failed: not found", "module_id", id)
		return apperr.New(apperr.CodeModuleNotFound, "")
	}

	// Recursively delete in transaction
	return repository.WithTransaction(s.db, ctx, func(tx *gorm.DB) error {
		if err := s.deleteChildrenTx(tx, id); err != nil {
			log.ErrorContext(ctx, "delete module failed: delete children error", "err", err, "module_id", id)
			return err
		}
		if err := tx.Delete(&model.Module{}, id).Error; err != nil {
			return err
		}
		log.InfoContext(ctx, "module deleted", "module_id", id)
		return nil
	})
}

// deleteChildrenTx recursively deletes child modules in transaction
func (s *ModuleService) deleteChildrenTx(tx *gorm.DB, parentID uint) error {
	var children []model.Module
	if err := tx.Where("parent_id = ?", parentID).Find(&children).Error; err != nil {
		return err
	}

	for _, child := range children {
		if err := s.deleteChildrenTx(tx, child.ID); err != nil {
			return err
		}
		if err := tx.Delete(&child).Error; err != nil {
			return err
		}
	}
	return nil
}
