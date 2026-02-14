package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// Video represents a video entity.
type Video struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	VideoURL  string    `json:"video_url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// VideoRepository defines the interface for video data access.
type VideoRepository interface {
	Create(ctx context.Context, video *Video) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status, videoURL string) error
}

// PostgresVideoRepository implements VideoRepository with PostgreSQL.
type PostgresVideoRepository struct {
	db *client.PostgresClient
}

// NewPostgresVideoRepository creates a new PostgresVideoRepository.
func NewPostgresVideoRepository(db *client.PostgresClient) *PostgresVideoRepository {
	return &PostgresVideoRepository{db: db}
}

// Create inserts a new video record.
func (r *PostgresVideoRepository) Create(ctx context.Context, video *Video) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO videos (user_id, video_url, status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		video.UserID,
		video.VideoURL,
		video.Status,
	).Scan(&video.ID, &video.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create video: %w", err)
	}

	return nil
}

// UpdateStatus updates the video status and URL after processing.
func (r *PostgresVideoRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status, videoURL string) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `UPDATE videos SET status = $1, video_url = $2 WHERE id = $3`
	_, err := r.db.Pool.Exec(ctx, query, status, videoURL, id)
	if err != nil {
		return fmt.Errorf("failed to update video status: %w", err)
	}

	return nil
}
