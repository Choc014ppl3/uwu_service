package profile

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// Profile is the public profile model returned by the profile domain.
type Profile struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	Bio         *string   `json:"bio,omitempty"`
	Settings    []byte    `json:"settings,omitempty"`
}

// ProfileRepository loads profile data from storage.
type ProfileRepository interface {
	GetProfile(ctx context.Context, userID string) (*Profile, *errors.AppError)
}

type profileRepository struct {
	db *client.PostgresClient
}

// NewProfileRepository creates a new profile repository.
func NewProfileRepository(db *client.PostgresClient) ProfileRepository {
	return &profileRepository{db: db}
}

func (r *profileRepository) GetProfile(ctx context.Context, userID string) (*Profile, *errors.AppError) {
	query := `
		SELECT id, email, display_name, avatar_url, bio, settings
		FROM users
		WHERE id = $1
	`

	var profile Profile
	err := r.db.Pool.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.Email,
		&profile.DisplayName,
		&profile.AvatarURL,
		&profile.Bio,
		&profile.Settings,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("profile not found")
		}
		return nil, errors.InternalWrap("failed to get profile", err)
	}

	return &profile, nil
}
