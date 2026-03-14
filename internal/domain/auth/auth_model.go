package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// -------------------------------------------------------------------------
// Models
// -------------------------------------------------------------------------

// User model
type User struct {
	ID           uuid.UUID       `json:"id"`
	Email        string          `json:"email"`
	PasswordHash string          `json:"-"`
	DisplayName  string          `json:"display_name"`
	AvatarURL    *string         `json:"avatar_url,omitempty"`
	Bio          *string         `json:"bio,omitempty"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// -------------------------------------------------------------------------
// Interfaces
// -------------------------------------------------------------------------

// AuthService interface
type AuthService struct {
	userRepo UserRepository
	jwtRepo  JWTRepository
}

// UserRepository interface
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
}

// JWTRepository interface
type JWTRepository interface {
	GenerateToken(user *User) (string, error)
	ValidateToken(tokenString string) (*TokenClaims, error)
}

// -------------------------------------------------------------------------
// Inputs
// -------------------------------------------------------------------------

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type LoginInput struct {
	Email    string
	Password string
}

// -------------------------------------------------------------------------
// Responses
// -------------------------------------------------------------------------

// TokenClaims represents the structured claims inside the JWT.
type TokenClaims struct {
	UserID      string
	Email       string
	DisplayName string
	AvatarURL   string
}

// AuthResponse is returned on successful register/login.
type AuthResponse struct {
	User  *User  `json:"user"`
	Token string `json:"token"`
}
