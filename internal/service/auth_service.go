package service

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// AuthService handles authentication logic.
type AuthService struct {
	userRepo  repository.UserRepository
	jwtSecret []byte
}

// NewAuthService creates a new AuthService.
func NewAuthService(userRepo repository.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		jwtSecret: []byte(jwtSecret),
	}
}

// RegisterReq represents a registration request.
type RegisterReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Bio         string `json:"bio,omitempty"`
}

// TokenClaims represents the structured claims inside the JWT.
type TokenClaims struct {
	UserID      string
	Email       string
	DisplayName string
	AvatarURL   string
}

// LoginReq represents a login request.
type LoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse is returned on successful register/login.
type AuthResponse struct {
	User  *repository.User `json:"user"`
	Token string           `json:"token"`
}

// Register creates a new user account and returns a JWT token.
func (s *AuthService) Register(ctx context.Context, req RegisterReq) (*AuthResponse, error) {
	// Check if user already exists
	existing, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, errors.InternalWrap("failed to check existing user", err)
	}
	if existing != nil {
		return nil, errors.New(errors.ErrConflict, "email already registered")
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.InternalWrap("failed to hash password", err)
	}

	var avatarURLPtr *string
	if req.AvatarURL != "" {
		avatarURLPtr = &req.AvatarURL
	}
	var bioPtr *string
	if req.Bio != "" {
		bioPtr = &req.Bio
	}

	user := &repository.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
		AvatarURL:    avatarURLPtr,
		Bio:          bioPtr,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, errors.InternalWrap("failed to create user", err)
	}

	// Generate JWT
	token, err := s.generateToken(user)
	if err != nil {
		return nil, errors.InternalWrap("failed to generate token", err)
	}

	return &AuthResponse{User: user, Token: token}, nil
}

// Login authenticates a user and returns a JWT token.
func (s *AuthService) Login(ctx context.Context, req LoginReq) (*AuthResponse, error) {
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, errors.InternalWrap("failed to find user", err)
	}
	if user == nil {
		return nil, errors.Unauthorized("invalid email or password")
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.Unauthorized("invalid email or password")
	}

	// Generate JWT
	token, err := s.generateToken(user)
	if err != nil {
		return nil, errors.InternalWrap("failed to generate token", err)
	}

	return &AuthResponse{User: user, Token: token}, nil
}

// ValidateToken parses and validates a JWT token string, returning the structured claims.
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
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
		return nil, fmt.Errorf("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid subject claim")
	}

	email, _ := claims["email"].(string)
	displayName, _ := claims["display_name"].(string)
	avatarURL, _ := claims["avatar_url"].(string)

	return &TokenClaims{
		UserID:      userID,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
	}, nil
}

func (s *AuthService) generateToken(user *repository.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":          user.ID.String(),
		"email":        user.Email,
		"display_name": user.DisplayName,
		"iat":          time.Now().Unix(),
		"exp":          time.Now().Add(72 * time.Hour).Unix(),
	}
	if user.AvatarURL != nil {
		claims["avatar_url"] = *user.AvatarURL
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}
