package service

import (
	"errors"

	"github.com/fize/go-ext/log"
	"github.com/fize/kumquat/portal/pkg/model"
	"github.com/fize/kumquat/portal/pkg/utils"
	"gorm.io/gorm"
)

// AuthService 认证服务
type AuthService struct {
	db *gorm.DB
}

// NewAuthService 创建认证服务
func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{db: db}
}

// Login 用户登录
func (s *AuthService) Login(username, password string) (string, *model.User, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn("login failed: user not found", "username", username)
			return "", nil, errors.New("invalid username or password")
		}
		log.Error("login failed: db error", "err", err, "username", username)
		return "", nil, err
	}

	if !user.CheckPassword(password) {
		log.Warn("login failed: incorrect password", "username", username)
		return "", nil, errors.New("invalid username or password")
	}

	if err := s.db.Model(&user).Association("Role").Find(&user.Role); err != nil {
		log.Error("login failed: load role error", "err", err, "user_id", user.ID)
		return "", nil, err
	}

	token, err := utils.GenerateToken(user.ID, user.Username, user.RoleID, user.Role.Name)
	if err != nil {
		log.Error("login failed: generate token error", "err", err, "user_id", user.ID)
		return "", nil, err
	}

	log.Info("user login", "user_id", user.ID, "username", user.Username, "role", user.Role.Name)
	return token, &user, nil
}

// Register 用户注册
func (s *AuthService) Register(username, email, password, nickname string) (*model.User, error) {
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		log.Warn("register failed: username exists", "username", username)
		return nil, errors.New("username already exists")
	}

	s.db.Model(&model.User{}).Where("email = ?", email).Count(&count)
	if count > 0 {
		log.Warn("register failed: email exists", "email", email)
		return nil, errors.New("email already exists")
	}

	var role model.Role
	if err := s.db.Where("name = ?", model.RoleGuest).First(&role).Error; err != nil {
		log.Error("register failed: default role not found", "err", err)
		return nil, errors.New("default role not found")
	}

	user := model.User{
		Username: username,
		Email:    email,
		Nickname: nickname,
		RoleID:   role.ID,
	}
	user.SetPassword(password)

	if err := s.db.Create(&user).Error; err != nil {
		log.Error("register failed: create user error", "err", err, "username", username)
		return nil, err
	}

	user.Role = role
	log.Info("user registered", "user_id", user.ID, "username", username, "email", email)
	return &user, nil
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		log.Warn("change password failed: user not found", "user_id", userID)
		return errors.New("user not found")
	}

	if !user.CheckPassword(oldPassword) {
		log.Warn("change password failed: incorrect old password", "user_id", userID)
		return errors.New("incorrect old password")
	}

	user.SetPassword(newPassword)
	if err := s.db.Model(&user).Update("password", user.Password).Error; err != nil {
		log.Error("change password failed: db error", "err", err, "user_id", userID)
		return err
	}

	log.Info("password changed", "user_id", userID, "username", user.Username)
	return nil
}

// GetUserByID 根据ID获取用户
func (s *AuthService) GetUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.Preload("Role").First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
