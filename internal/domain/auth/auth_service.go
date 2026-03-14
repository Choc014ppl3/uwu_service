package auth

import (
	"context"

	errors "github.com/windfall/uwu_service/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

// NewAuthService creates a new AuthService.
func NewAuthService(userRepo UserRepository, jwtRepo JWTRepository) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		jwtRepo:  jwtRepo,
	}
}

// Register creates a new user account and returns a JWT token.
func (s *AuthService) Register(ctx context.Context, req RegisterInput) (*AuthResponse, error) {
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

	user := &User{
		Email:        req.Email,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, errors.InternalWrap("failed to create user", err)
	}

	// Generate JWT
	token, err := s.jwtRepo.GenerateToken(user)
	if err != nil {
		return nil, errors.InternalWrap("failed to generate token", err)
	}

	return &AuthResponse{User: user, Token: token}, nil
}

// Login authenticates a user and returns a JWT token.
func (s *AuthService) Login(ctx context.Context, req LoginInput) (*AuthResponse, error) {
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
	token, err := s.jwtRepo.GenerateToken(user)
	if err != nil {
		return nil, errors.InternalWrap("failed to generate token", err)
	}

	return &AuthResponse{User: user, Token: token}, nil
}
