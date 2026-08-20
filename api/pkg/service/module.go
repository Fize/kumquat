package service

import (
	"context"
	"errors"
	"sort"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/api/pkg/errors"
	"github.com/fize/kumquat/api/pkg/model"
	"github.com/fize/kumquat/api/pkg/repository"
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

// ListScoped returns the caller's module as the root and its complete subtree.
func (s *ModuleService) ListScoped(ctx context.Context, rootPublicID string) ([]model.Module, error) {
	root, err := s.GetByPublicID(ctx, rootPublicID)
	if err != nil {
		return nil, err
	}
	ids, err := descendantModuleIDs(s.db.WithContext(ctx), root.ID)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	var modules []model.Module
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&modules).Error; err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	for i := range modules {
		if modules[i].ID == root.ID {
			modules[i].ParentID = nil
			modules[i].Parent = nil
		}
	}
	return s.buildTree(modules), nil
}

func (s *ModuleService) CanAccess(ctx context.Context, rootPublicID, targetPublicID string) (bool, error) {
	root, err := s.GetByPublicID(ctx, rootPublicID)
	if err != nil {
		return false, err
	}
	target, err := s.GetByPublicID(ctx, targetPublicID)
	if err != nil {
		return false, err
	}
	ids, err := descendantModuleIDs(s.db.WithContext(ctx), root.ID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == target.ID {
			return true, nil
		}
	}
	return false, nil
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
			parent.Children[i].Parent = &model.Module{PublicID: parent.PublicID}
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

func (s *ModuleService) GetChildren(ctx context.Context, id uint) ([]model.Module, error) {
	if _, err := s.GetByID(ctx, id); err != nil {
		return nil, err
	}
	children, err := s.repo.GetChildren(ctx, id)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return children, nil
}

func (s *ModuleService) GetByPublicID(ctx context.Context, id string) (*model.Module, error) {
	var m model.Module
	if err := s.db.WithContext(ctx).Preload("Parent").Where("public_id = ?", id).First(&m).Error; err != nil {
		return nil, apperr.New(apperr.CodeModuleNotFound, "")
	}
	return &m, nil
}
func (s *ModuleService) GetChildrenByPublicID(ctx context.Context, id string) ([]model.Module, error) {
	m, err := s.GetByPublicID(ctx, id)
	if err != nil {
		return nil, err
	}
	children, err := s.GetChildren(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	for i := range children {
		children[i].Parent = &model.Module{PublicID: m.PublicID}
	}
	return children, nil
}
func (s *ModuleService) CreateWithParentPublicID(ctx context.Context, name string, parentID *string, sort int) (*model.Module, error) {
	var internal *uint
	if parentID != nil {
		p, err := s.GetByPublicID(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		internal = &p.ID
	}
	created, err := s.Create(ctx, name, internal, sort)
	if err != nil {
		return nil, err
	}
	return s.GetByPublicID(ctx, created.PublicID)
}
func (s *ModuleService) UpdateByPublicID(ctx context.Context, id, name string, sort int) (*model.Module, error) {
	m, err := s.GetByPublicID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err = s.Update(ctx, m.ID, name, sort); err != nil {
		return nil, err
	}
	return s.GetByPublicID(ctx, id)
}
func (s *ModuleService) DeleteByPublicID(ctx context.Context, id string) error {
	m, err := s.GetByPublicID(ctx, id)
	if err != nil {
		return err
	}
	return s.Delete(ctx, m.ID)
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
	renamed := name != "" && name != module.Name

	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	updates["sort"] = sort

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Module{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if !renamed {
			return nil
		}
		newPath := ""
		if module.ParentID == nil {
			newPath = "/" + name
		} else {
			var parent model.Module
			if err := tx.First(&parent, *module.ParentID).Error; err != nil {
				return err
			}
			newPath = parent.Path + "/" + name
		}
		if err := tx.Model(&model.Module{}).Where("id = ?", id).Update("path", newPath).Error; err != nil {
			return err
		}
		return rewriteChildPaths(tx, id, newPath)
	}); err != nil {
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
	ids, err := descendantModuleIDs(s.db.WithContext(ctx), id)
	if err != nil {
		return apperr.WrapCode(apperr.CodeInternal, err)
	}
	var references int64
	if err := s.db.WithContext(ctx).Model(&model.Project{}).Where("module_id IN ?", ids).Count(&references).Error; err != nil {
		return apperr.WrapCode(apperr.CodeInternal, err)
	}
	var userReferences int64
	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("module_id IN ?", ids).Count(&userReferences).Error; err != nil {
		return apperr.WrapCode(apperr.CodeInternal, err)
	}
	if references+userReferences > 0 {
		return apperr.New(apperr.CodeConflict, "module or descendants are still referenced")
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

func rewriteChildPaths(tx *gorm.DB, parentID uint, parentPath string) error {
	var children []model.Module
	if err := tx.Where("parent_id = ?", parentID).Find(&children).Error; err != nil {
		return err
	}
	for i := range children {
		path := parentPath + "/" + children[i].Name
		if err := tx.Model(&model.Module{}).Where("id = ?", children[i].ID).Update("path", path).Error; err != nil {
			return err
		}
		if err := rewriteChildPaths(tx, children[i].ID, path); err != nil {
			return err
		}
	}
	return nil
}

func descendantModuleIDs(db *gorm.DB, root uint) ([]uint, error) {
	ids := []uint{root}
	frontier := []uint{root}
	for len(frontier) > 0 {
		var children []model.Module
		if err := db.Select("id").Where("parent_id IN ?", frontier).Find(&children).Error; err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, child := range children {
			ids = append(ids, child.ID)
			frontier = append(frontier, child.ID)
		}
	}
	return ids, nil
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
