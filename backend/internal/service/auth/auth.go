package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"camguid/internal/models"
	"camguid/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user account is disabled")
	ErrUserExists         = errors.New("username already exists")
)

type Service struct {
	userRepo  *repository.UserRepository
	jwtSecret []byte
}

func NewService(userRepo *repository.UserRepository, jwtSecret string) *Service {
	return &Service{
		userRepo:  userRepo,
		jwtSecret: []byte(jwtSecret),
	}
}

// Login validates credentials and returns JWT tokens.
func (s *Service) Login(ctx context.Context, req models.LoginRequest) (*models.TokenResponse, *models.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, nil, fmt.Errorf("database error: %w", err)
	}
	if user == nil {
		return nil, nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, nil, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	tokens, err := s.generateTokens(user)
	if err != nil {
		return nil, nil, err
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)

	return tokens, user, nil
}

// Register creates a new user. Admin-only operation.
func (s *Service) Register(ctx context.Context, req models.RegisterRequest) (*models.User, error) {
	existing, _ := s.userRepo.GetByUsername(ctx, req.Username)
	if existing != nil {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &models.User{
		ID:           uuid.New(),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         req.Role,
		IsActive:     true,
	}
	if req.Email != "" {
		user.Email = &req.Email
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// ChangePassword updates the user's password after verifying the old one.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, req models.ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return errors.New("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.userRepo.UpdatePassword(ctx, userID, string(hash))
}

// RefreshToken validates a refresh token and issues new tokens.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*models.TokenResponse, error) {
	claims, err := s.validateToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		return nil, errors.New("invalid token type")
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, errors.New("invalid user ID in token")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return nil, errors.New("user not found or disabled")
	}

	return s.generateTokens(user)
}

// EnsureDefaultAdmin creates a default admin if no admins exist.
func (s *Service) EnsureDefaultAdmin(ctx context.Context) error {
	count, err := s.userRepo.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := "admin"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 12)

	admin := &models.User{
		ID:           uuid.New(),
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         "admin",
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, admin); err != nil {
		return err
	}

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║   DEFAULT ADMIN ACCOUNT CREATED          ║")
	fmt.Printf("║   Username: admin                        ║\n")
	fmt.Printf("║   Password: %-28s ║\n", password)
	fmt.Println("║   ⚠ Change this password after login!    ║")
	fmt.Println("╚══════════════════════════════════════════╝")

	return nil
}

func (s *Service) generateTokens(user *models.User) (*models.TokenResponse, error) {
	// Access token (15 min)
	accessClaims := jwt.MapClaims{
		"sub":  user.ID.String(),
		"role": user.Role,
		"type": "access",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := accessToken.SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	// Refresh token (7 days)
	refreshClaims := jwt.MapClaims{
		"sub":  user.ID.String(),
		"type": "refresh",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	return &models.TokenResponse{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
		ExpiresIn:    900, // 15 minutes in seconds
	}, nil
}

func (s *Service) validateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
