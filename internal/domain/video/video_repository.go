package video

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// Constants
const FeatureID = 1

// LearningItem model
type LearningItem struct {
	ID        uuid.UUID       `json:"id"`
	FeatureID int             `json:"feature_id"`
	Content   string          `json:"content"`
	Language  string          `json:"language"`
	Level     *string         `json:"level"`
	Details   json.RawMessage `json:"details"`
	Metadata  json.RawMessage `json:"metadata"`
	Tags      json.RawMessage `json:"tags"`
	IsActive  bool            `json:"is_active"`
	CreatedAt *time.Time      `json:"created_at"`
	UpdatedAt *time.Time      `json:"updated_at"`
}

// VideoDetails is the structure of the details field in LearningItem model
type VideoDetails struct {
	Topic       string              `json:"topic"`
	Description string              `json:"description"`
	Language    string              `json:"language"`
	Level       string              `json:"level"`
	Transcript  string              `json:"transcript"`
	Tags        []string            `json:"tags"`
	Segments    []TranscriptSegment `json:"segments"`
	GistQuiz    []struct {
		ID      int    `json:"id"`
		Type    string `json:"type"`
		Options []struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			IsCorrect bool   `json:"is_correct"`
		} `json:"options"`
		Category     string `json:"category"`
		Question     string `json:"question"`
		CorrectOrder any    `json:"correct_order"`
	} `json:"gist_quiz"`
	RetellStory struct {
		KeyPoints     []string `json:"key_points"`
		RetellExample string   `json:"retell_example"`
	} `json:"retell_story"`
}

// VideoMetadata is the structure of the metadata field in LearningItem model
type VideoMetadata struct {
	UserID       string `json:"user_id"`
	BatchID      string `json:"batch_id"`
	VideoURL     string `json:"video_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	Status       string `json:"status"`
}

// VideoRepository interface
type VideoRepository interface {
	CreateVideo(ctx context.Context, item *LearningItem) *errors.AppError
	UpdateVideo(ctx context.Context, item *LearningItem) *errors.AppError
}

type videoRepository struct {
	db *client.PostgresClient
}

func NewVideoRepository(db *client.PostgresClient) VideoRepository {
	return &videoRepository{db: db}
}

func (r *videoRepository) CreateVideo(ctx context.Context, item *LearningItem) *errors.AppError {
	query := `
		INSERT INTO learning_items (
			id, feature_id, content, language, level, details, tags, metadata, is_active
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.ID,
		FeatureID,
		item.Content,
		item.Language,
		item.Level,
		item.Details,
		item.Tags,
		item.Metadata,
		item.IsActive,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return errors.InternalWrap("failed to create video content", err)
	}

	return nil
}

func (r *videoRepository) UpdateVideo(ctx context.Context, item *LearningItem) *errors.AppError {
	query := `
		UPDATE learning_items
		SET feature_id = $1, content = $2, language = $3, level = $4, tags = $5, details = $6, metadata = $7, is_active = $8
		WHERE id = $9
	`

	err := r.db.Pool.QueryRow(ctx, query,
		FeatureID,
		item.Content,
		item.Language,
		item.Level,
		item.Tags,
		item.Details,
		item.Metadata,
		item.IsActive,
		item.ID,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return errors.InternalWrap("failed to update video details", err)
	}

	return nil
}
