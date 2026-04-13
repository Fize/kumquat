package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/model"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	db *gorm.DB
}

// NewUserService 创建用户服务
func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

// List 获取用户列表
func (s *UserService) List(ctx context.Context, page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	if err := s.db.Model(&model.User{}).Count(&total).Error; err != nil {
		log.ErrorContext(ctx, "list users failed: count error", "err", err)
		return nil, 0, err
	}

	if err := s.db.Preload("Role").Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Find(&users).Error; err != nil {
		log.ErrorContext(ctx, "list users failed: query error", "err", err)
		return nil, 0, err
	}

	return users, total, nil
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	if err := s.db.Preload("Role").Preload("Module").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Create 创建用户
func (s *UserService) Create(ctx context.Context, username, email, password, nickname string, roleID uint, moduleID *uint) (*model.User, error) {
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		log.WarnContext(ctx, "create user failed: username exists", "username", username)
		return nil, errors.New("username already exists")
	}

	s.db.Model(&model.User{}).Where("email = ?", email).Count(&count)
	if count > 0 {
		log.WarnContext(ctx, "create user failed: email exists", "email", email)
		return nil, errors.New("email already exists")
	}

	var role model.Role
	if err := s.db.First(&role, roleID).Error; err != nil {
		log.WarnContext(ctx, "create user failed: role not found", "role_id", roleID)
		return nil, errors.New("role not found")
	}

	if moduleID != nil {
		var module model.Module
		if err := s.db.First(&module, *moduleID).Error; err != nil {
			log.WarnContext(ctx, "create user failed: module not found", "module_id", *moduleID)
			return nil, errors.New("module not found")
		}
	}

	user := model.User{
		Username: username,
		Email:    email,
		Nickname: nickname,
		RoleID:   roleID,
		ModuleID: moduleID,
	}
	user.SetPassword(password)

	if err := s.db.Create(&user).Error; err != nil {
		log.ErrorContext(ctx, "create user failed: db error", "err", err, "username", username)
		return nil, err
	}

	user.Role = role
	log.InfoContext(ctx, "user created", "user_id", user.ID, "username", username, "role_id", roleID)
	return &user, nil
}

// Update 更新用户
func (s *UserService) Update(ctx context.Context, id uint, nickname string, roleID uint, moduleID *uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		log.WarnContext(ctx, "update user failed: not found", "user_id", id)
		return nil, errors.New("user not found")
	}

	updates := map[string]interface{}{}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if roleID > 0 {
		var role model.Role
		if err := s.db.First(&role, roleID).Error; err != nil {
			log.WarnContext(ctx, "update user failed: role not found", "role_id", roleID)
			return nil, errors.New("role not found")
		}
		updates["role_id"] = roleID
	}
	if moduleID != nil {
		var module model.Module
		if err := s.db.First(&module, *moduleID).Error; err != nil {
			log.WarnContext(ctx, "update user failed: module not found", "module_id", *moduleID)
			return nil, errors.New("module not found")
		}
		updates["module_id"] = *moduleID
	}

	if err := s.db.Model(&user).Updates(updates).Error; err != nil {
		log.ErrorContext(ctx, "update user failed: db error", "err", err, "user_id", id)
		return nil, err
	}

	log.InfoContext(ctx, "user updated", "user_id", id, "username", user.Username, "updates", fmt.Sprintf("%v", updates))
	return s.GetByID(ctx, id)
}

// Delete 删除用户
func (s *UserService) Delete(ctx context.Context, id uint) error {
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		log.WarnContext(ctx, "delete user failed: not found", "user_id", id)
		return errors.New("user not found")
	}

	if user.RoleID == 1 {
		var count int64
		s.db.Model(&model.User{}).Where("role_id = ?", 1).Count(&count)
		if count <= 1 {
			log.WarnContext(ctx, "delete user failed: cannot delete last admin", "user_id", id)
			return errors.New("cannot delete the last admin")
		}
	}

	if err := s.db.Delete(&user).Error; err != nil {
		log.ErrorContext(ctx, "delete user failed: db error", "err", err, "user_id", id)
		return err
	}

	log.InfoContext(ctx, "user deleted", "user_id", id, "username", user.Username)
	return nil
}

// Search 搜索用户
func (s *UserService) Search(ctx context.Context, keyword string, page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := s.db.Model(&model.User{}).
		Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?",
			fmt.Sprintf("%%%s%%", keyword),
			fmt.Sprintf("%%%s%%", keyword),
			fmt.Sprintf("%%%s%%", keyword))

	if err := query.Count(&total).Error; err != nil {
		log.ErrorContext(ctx, "search users failed: count error", "err", err, "keyword", keyword)
		return nil, 0, err
	}

	if err := query.Preload("Role").Preload("Module").
		Offset((page - 1) * size).Limit(size).
		Find(&users).Error; err != nil {
		log.ErrorContext(ctx, "search users failed: query error", "err", err, "keyword", keyword)
		return nil, 0, err
	}

	return users, total, nil
}
