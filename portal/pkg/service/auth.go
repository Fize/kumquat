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

// AuthService authentication service
type AuthService struct {
	repo       repository.UserRepository
	roleRepo   repository.RoleRepository
	jwtService *JWTService
	db         *gorm.DB
}

// NewAuthService creates authentication service
func NewAuthService(repo repository.UserRepository, roleRepo repository.RoleRepository, jwtService *JWTService, db *gorm.DB) *AuthService {
	return &AuthService{repo: repo, roleRepo: roleRepo, jwtService: jwtService, db: db}
}

// Login user login
func (s *AuthService) Login(ctx context.Context, username, password string) (string, *model.User, error) {
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.WarnContext(ctx, "login failed: user not found", "username", username)
			return "", nil, apperr.New(apperr.CodeInvalidPassword, "invalid username or password")
		}
		log.ErrorContext(ctx, "login failed: db error", "err", err, "username", username)
		return "", nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	if !user.CheckPassword(password) {
		log.WarnContext(ctx, "login failed: incorrect password", "username", username)
		return "", nil, apperr.New(apperr.CodeInvalidPassword, "invalid username or password")
	}

	// Load role
	if err := s.db.Model(user).Association("Role").Find(&user.Role); err != nil {
		log.ErrorContext(ctx, "login failed: load role error", "err", err, "user_id", user.ID)
		return "", nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	token, err := s.jwtService.GenerateToken(user.ID, user.Username, user.RoleID, user.Role.Name)
	if err != nil {
		log.ErrorContext(ctx, "login failed: generate token error", "err", err, "user_id", user.ID)
		return "", nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "user login", "user_id", user.ID, "username", user.Username, "role", user.Role.Name)
	return token, user, nil
}

// Register user registration (using transaction)
func (s *AuthService) Register(ctx context.Context, username, email, password, nickname string) (*model.User, error) {
	// Pre-check
	exists, err := s.repo.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	if exists {
		log.WarnContext(ctx, "register failed: username exists", "username", username)
		return nil, apperr.New(apperr.CodeUsernameExists, "")
	}

	exists, err = s.repo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	if exists {
		log.WarnContext(ctx, "register failed: email exists", "email", email)
		return nil, apperr.New(apperr.CodeEmailExists, "")
	}

	var user model.User
	var role model.Role

	// Create user in transaction
	err = repository.WithTransaction(s.db, ctx, func(tx *gorm.DB) error {
		// Query default role
		if err := tx.Where("name = ?", model.RoleGuest).First(&role).Error; err != nil {
			log.ErrorContext(ctx, "register failed: default role not found", "err", err)
			return err
		}

		user = model.User{
			Username: username,
			Email:    email,
			Nickname: nickname,
			RoleID:   role.ID,
		}
		user.SetPassword(password)

		if err := tx.Create(&user).Error; err != nil {
			log.ErrorContext(ctx, "register failed: create user error", "err", err, "username", username)
			return err
		}

		return nil
	})

	if err != nil {
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}

	user.Role = role
	log.InfoContext(ctx, "user registered", "user_id", user.ID, "username", username, "email", email)
	return &user, nil
}

// ChangePassword changes password
func (s *AuthService) ChangePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		log.WarnContext(ctx, "change password failed: user not found", "user_id", userID)
		return apperr.New(apperr.CodeUserNotFound, "")
	}

	if !user.CheckPassword(oldPassword) {
		log.WarnContext(ctx, "change password failed: incorrect old password", "user_id", userID)
		return apperr.New(apperr.CodeInvalidPassword, "incorrect old password")
	}

	user.Password = newPassword
	if err := s.db.Save(user).Error; err != nil {
		log.ErrorContext(ctx, "change password failed: db error", "err", err, "user_id", userID)
		return apperr.WrapCode(apperr.CodeInternal, err)
	}

	log.InfoContext(ctx, "password changed", "user_id", userID, "username", user.Username)
	return nil
}

// GetUserByID gets user by ID
func (s *AuthService) GetUserByID(ctx context.Context, userID uint) (*model.User, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeUserNotFound, "")
		}
		return nil, apperr.WrapCode(apperr.CodeInternal, err)
	}
	return user, nil
}
