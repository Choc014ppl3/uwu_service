package auth

import (
	"context"

	"github.com/windfall/uwu_service/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

// AuthService
type AuthService struct {
	authRepo AuthRepository
}

// AuthResponse is returned on successful register/login.
type AuthResponse struct {
	User  *User  `json:"user"`
	Token string `json:"token"`
}

// NewAuthService creates a new AuthService.
func NewAuthService(authRepo AuthRepository) *AuthService {
	return &AuthService{
		authRepo: authRepo,
	}
}

// Register creates a new user account and returns a JWT token.
func (s *AuthService) Register(ctx context.Context, req RegisterInput) (*AuthResponse, *errors.AppError) {
	// Check if user already exists
	existing, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.Validation("email already registered")
	}

	// Hash password
	hashed, bcryptErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if bcryptErr != nil {
		return nil, errors.UnauthorizedWrap("failed to hash password", bcryptErr)
	}

	user := &User{
		Email:        req.Email,
		PasswordHash: string(hashed),
		DisplayName:  req.DisplayName,
		AvatarURL:    &req.AvatarURL,
	}

	if err := s.authRepo.RegisterUser(ctx, user); err != nil {
		return nil, err
	}

	// Generate JWT
	token, err := s.authRepo.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: user, Token: token}, nil
}

// Login authenticates a user and returns a JWT token.
func (s *AuthService) Login(ctx context.Context, req LoginInput) (*AuthResponse, *errors.AppError) {
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.Unauthorized("invalid email or password")
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.Unauthorized("invalid email or password")
	}

	// Generate JWT
	token, err := s.authRepo.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: user, Token: token}, nil
}
