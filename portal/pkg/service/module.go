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

// ModuleService 模块服务
type ModuleService struct {
	repo repository.ModuleRepository
	db   *gorm.DB // 保留用于递归删除
}

// NewModuleService 创建模块服务
func NewModuleService(repo repository.ModuleRepository, db *gorm.DB) *ModuleService {
	return &ModuleService{repo: repo, db: db}
}

// List 获取模块列表（树形）
func (s *ModuleService) List(ctx context.Context) ([]model.Module, error) {
	modules, err := s.repo.List(ctx)
	if err != nil {
		log.ErrorContext(ctx, "list modules failed", "err", err)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return s.buildTree(modules), nil
}

// buildTree 构建树形结构
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

// GetByID 根据ID获取模块
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

// Create 创建模块
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

// Update 更新模块
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

// Delete 删除模块（递归删除子模块）
func (s *ModuleService) Delete(ctx context.Context, id uint) error {
	module, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "delete module failed: not found", "module_id", id)
		return apperr.New(apperr.CodeModuleNotFound, "")
	}

	if err := s.deleteChildren(ctx, id); err != nil {
		log.ErrorContext(ctx, "delete module failed: delete children error", "err", err, "module_id", id)
		return apperr.WrapCode(apperr.CodeInternal, err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		log.ErrorContext(ctx, "delete module failed: db error", "err", err, "module_id", id)
		return apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "module deleted", "module_id", id, "name", module.Name)
	return nil
}

func (s *ModuleService) deleteChildren(ctx context.Context, parentID uint) error {
	children, err := s.repo.GetChildren(ctx, parentID)
	if err != nil {
		return err
	}

	for _, child := range children {
		if err := s.deleteChildren(ctx, child.ID); err != nil {
			return err
		}
		s.db.Delete(&child)
	}
	return nil
}
