package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// TranscriptSegment represents a single segment of a video transcript with timing.
type TranscriptSegment struct {
	Text     string  `json:"text"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// Video represents a video entity.
type Video struct {
	ID               uuid.UUID           `json:"id"`
	UserID           uuid.UUID           `json:"user_id"`
	VideoURL         string              `json:"video_url"`
	Status           string              `json:"status"`
	Transcript       []TranscriptSegment `json:"transcript"`
	RawResponse      json.RawMessage     `json:"raw_response,omitempty"`
	DetectedLanguage string              `json:"detected_language"`
	ProcessingStatus string              `json:"processing_status"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// VideoRepository defines the interface for video data access.
type VideoRepository interface {
	Create(ctx context.Context, video *Video) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status, videoURL string) error
	UpdateTranscript(ctx context.Context, id uuid.UUID, segments []TranscriptSegment, rawResponse json.RawMessage, detectedLanguage, processingStatus string) error
	GetByID(ctx context.Context, id uuid.UUID) (*Video, error)
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

// UpdateTranscript updates the transcript (JSONB), raw provider response, detected language, and processing status.
func (r *PostgresVideoRepository) UpdateTranscript(ctx context.Context, id uuid.UUID, segments []TranscriptSegment, rawResponse json.RawMessage, detectedLanguage, processingStatus string) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	// Marshal segments to JSON for JSONB column
	transcriptJSON, err := json.Marshal(segments)
	if err != nil {
		return fmt.Errorf("failed to marshal transcript segments: %w", err)
	}

	query := `UPDATE videos SET transcript = $1, raw_response = $2, detected_language = $3, processing_status = $4, updated_at = NOW() WHERE id = $5`
	result, err := r.db.Pool.Exec(ctx, query, transcriptJSON, rawResponse, detectedLanguage, processingStatus, id)
	if err != nil {
		return fmt.Errorf("failed to update video transcript: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("video not found: %s", id)
	}

	return nil
}

// GetByID retrieves a video by its ID.
func (r *PostgresVideoRepository) GetByID(ctx context.Context, id uuid.UUID) (*Video, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, user_id, video_url, status, transcript, raw_response, detected_language, processing_status, created_at, updated_at
		FROM videos
		WHERE id = $1
	`

	var video Video
	var transcriptJSON []byte
	var rawResponseJSON []byte

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&video.ID,
		&video.UserID,
		&video.VideoURL,
		&video.Status,
		&transcriptJSON,
		&rawResponseJSON,
		&video.DetectedLanguage,
		&video.ProcessingStatus,
		&video.CreatedAt,
		&video.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get video: %w", err)
	}

	// Unmarshal JSONB transcript (if present)
	if len(transcriptJSON) > 0 {
		if err := json.Unmarshal(transcriptJSON, &video.Transcript); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transcript: %w", err)
		}
	} else {
		video.Transcript = make([]TranscriptSegment, 0)
	}

	// Store raw response bytes directly
	if len(rawResponseJSON) > 0 {
		video.RawResponse = rawResponseJSON
	}

	return &video, nil
}
