package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/windfall/uwu_service/internal/client"
)

// User represents a user entity.
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

// UserRepository defines the interface for user data access.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
}

// PostgresUserRepository implements UserRepository with PostgreSQL.
type PostgresUserRepository struct {
	db *client.PostgresClient
}

// NewPostgresUserRepository creates a new PostgresUserRepository.
func NewPostgresUserRepository(db *client.PostgresClient) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// Create inserts a new user into the database.
func (r *PostgresUserRepository) Create(ctx context.Context, user *User) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
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
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByEmail retrieves a user by email address.
func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
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
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}
