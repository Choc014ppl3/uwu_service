package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

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

// AuthRepository interface
type AuthRepository interface {
	RegisterUser(ctx context.Context, user *User) *errors.AppError
	VerifyEmail(ctx context.Context, email string) (*User, *errors.AppError)
	GenerateToken(user *User) (string, *errors.AppError)
	ValidateToken(tokenString string) (*TokenClaims, *errors.AppError)
}

// TokenClaims represents the structured claims inside the JWT
type TokenClaims struct {
	UserID      string
	Email       string
	DisplayName string
	AvatarURL   string
}

// AuthRepository struct
type authRepository struct {
	db     *client.PostgresClient
	secret []byte
}

// NewAuthRepository constructor
func NewAuthRepository(db *client.PostgresClient, secret []byte) *authRepository {
	return &authRepository{db: db, secret: secret}
}

// RegisterUser inserts a new user into the database.
func (r *authRepository) RegisterUser(ctx context.Context, user *User) *errors.AppError {
	if r.db == nil || r.db.Pool == nil {
		return errors.Internal("database not configured")
	}

	query := `
        INSERT INTO users (email, password_hash, display_name, avatar_url, bio, settings)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, created_at, updated_at
    `

	settingsDB := user.Settings
	if len(settingsDB) == 0 {
		settingsDB = []byte("{}")
	}

	err := r.db.Pool.QueryRow(ctx, query,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.AvatarURL,
		user.Bio,
		settingsDB,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return errors.InternalWrap("failed to create user", err)
	}

	return nil
}

// VerifyUser retrieves a user by email address.
func (r *authRepository) VerifyEmail(ctx context.Context, email string) (*User, *errors.AppError) {
	if r.db == nil || r.db.Pool == nil {
		return nil, errors.Internal("database not configured")
	}

	query := `
        SELECT id, email, password_hash, display_name, avatar_url, bio, settings, created_at, updated_at
        FROM users
        WHERE email = $1
    `

	var user User
	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Bio,
		&user.Settings,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, errors.InternalWrap("failed to get user by email", err)
	}

	return &user, nil
}

// ValidateToken parses and validates a JWT token string, returning the structured claims.
func (s *authRepository) ValidateToken(tokenString string) (*TokenClaims, *errors.AppError) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, errors.InternalWrap("failed to parse token", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.Internal("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, errors.Internal("invalid subject claim")
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

func (s *authRepository) GenerateToken(user *User) (string, *errors.AppError) {
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
	jwtString, err := token.SignedString(s.secret)
	if err != nil {
		return "", errors.InternalWrap("failed to generate token", err)
	}
	return jwtString, nil
}
