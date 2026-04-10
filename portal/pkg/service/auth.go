package service

import (
	"errors"

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
			return "", nil, errors.New("invalid username or password")
		}
		return "", nil, err
	}

	if !user.CheckPassword(password) {
		return "", nil, errors.New("invalid username or password")
	}

	// 加载角色
	if err := s.db.Model(&user).Association("Role").Find(&user.Role); err != nil {
		return "", nil, err
	}

	token, err := utils.GenerateToken(user.ID, user.Username, user.RoleID, user.Role.Name)
	if err != nil {
		return "", nil, err
	}

	return token, &user, nil
}

// Register 用户注册
func (s *AuthService) Register(username, email, password, nickname string) (*model.User, error) {
	// 检查用户名
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, errors.New("username already exists")
	}

	// 检查邮箱
	s.db.Model(&model.User{}).Where("email = ?", email).Count(&count)
	if count > 0 {
		return nil, errors.New("email already exists")
	}

	// 获取默认角色(guest)
	var role model.Role
	if err := s.db.Where("name = ?", model.RoleGuest).First(&role).Error; err != nil {
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
		return nil, err
	}

	user.Role = role
	return &user, nil
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return errors.New("user not found")
	}

	if !user.CheckPassword(oldPassword) {
		return errors.New("incorrect old password")
	}

	user.SetPassword(newPassword)
	return s.db.Model(&user).Update("password", user.Password).Error
}

// GetUserByID 根据ID获取用户
func (s *AuthService) GetUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.Preload("Role").First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
