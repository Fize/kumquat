package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/fize/go-ext/log"
	apperr "github.com/fize/kumquat/portal/pkg/errors"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/repository"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	repo   repository.UserRepository
	roleRepo repository.RoleRepository
	db     *gorm.DB // 保留用于复杂操作
}

// NewUserService 创建用户服务
func NewUserService(repo repository.UserRepository, roleRepo repository.RoleRepository, db *gorm.DB) *UserService {
	return &UserService{repo: repo, roleRepo: roleRepo, db: db}
}

// List 获取用户列表
func (s *UserService) List(ctx context.Context, page, size int) ([]model.User, int64, error) {
	users, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		log.ErrorContext(ctx, "list users failed", "err", err)
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return users, total, nil
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(ctx context.Context, id uint) (*model.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeUserNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return user, nil
}

// Create 创建用户
func (s *UserService) Create(ctx context.Context, username, email, password, nickname string, roleID uint, moduleID *uint) (*model.User, error) {
	exists, err := s.repo.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	if exists {
		log.WarnContext(ctx, "create user failed: username exists", "username", username)
		return nil, apperr.New(apperr.CodeUsernameExists, "")
	}

	exists, err = s.repo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	if exists {
		log.WarnContext(ctx, "create user failed: email exists", "email", email)
		return nil, apperr.New(apperr.CodeEmailExists, "")
	}

	_, err = s.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		log.WarnContext(ctx, "create user failed: role not found", "role_id", roleID)
		return nil, apperr.New(apperr.CodeRoleNotFound, "")
	}

	user := model.User{
		Username: username,
		Email:    email,
		Nickname: nickname,
		RoleID:   roleID,
		ModuleID: moduleID,
	}
	user.SetPassword(password)

	if err := s.repo.Create(ctx, &user); err != nil {
		log.ErrorContext(ctx, "create user failed: db error", "err", err, "username", username)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "user created", "user_id", user.ID, "username", username, "role_id", roleID)
	return s.repo.GetByID(ctx, user.ID)
}

// Update 更新用户
func (s *UserService) Update(ctx context.Context, id uint, nickname string, roleID uint, moduleID *uint) (*model.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "update user failed: not found", "user_id", id)
		return nil, apperr.New(apperr.CodeUserNotFound, "")
	}

	updates := map[string]interface{}{}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if roleID > 0 {
		_, err := s.roleRepo.GetByID(ctx, roleID)
		if err != nil {
			log.WarnContext(ctx, "update user failed: role not found", "role_id", roleID)
			return nil, apperr.New(apperr.CodeRoleNotFound, "")
		}
		updates["role_id"] = roleID
	}
	if moduleID != nil {
		updates["module_id"] = *moduleID
	}

	if err := s.repo.Update(ctx, user, updates); err != nil {
		log.ErrorContext(ctx, "update user failed: db error", "err", err, "user_id", id)
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "user updated", "user_id", id, "username", user.Username, "updates", fmt.Sprintf("%v", updates))
	return s.repo.GetByID(ctx, id)
}

// Delete 删除用户
func (s *UserService) Delete(ctx context.Context, id uint) error {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.WarnContext(ctx, "delete user failed: not found", "user_id", id)
		return apperr.New(apperr.CodeUserNotFound, "")
	}

	if user.RoleID == 1 {
		count, err := s.repo.CountByRoleID(ctx, 1)
		if err != nil {
			return apperr.WrapCode(apperr.CodeInternal, err)
		}
		if count <= 1 {
			log.WarnContext(ctx, "delete user failed: cannot delete last admin", "user_id", id)
			return apperr.New(apperr.CodeLastAdmin, "")
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		log.ErrorContext(ctx, "delete user failed: db error", "err", err, "user_id", id)
		return apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "user deleted", "user_id", id, "username", user.Username)
	return nil
}

// Search 搜索用户
func (s *UserService) Search(ctx context.Context, keyword string, page, size int) ([]model.User, int64, error) {
	users, total, err := s.repo.Search(ctx, keyword, page, size)
	if err != nil {
		log.ErrorContext(ctx, "search users failed", "err", err, "keyword", keyword)
		return nil, 0, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return users, total, nil
}
