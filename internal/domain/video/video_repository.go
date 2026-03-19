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

// User Action model
type UserAction struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	ActionType string          `json:"action_type"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	DeletedAt  *time.Time      `json:"deleted_at"`
}

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
	CreatedBy string          `json:"created_by"`
	CreatedAt *time.Time      `json:"created_at"`
	UpdatedAt *time.Time      `json:"updated_at"`
	// Learning Item Actions
	Actions []UserAction `json:"actions"`
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
	UserID       string       `json:"user_id"`
	VideoURL     string       `json:"video_url"`
	ThumbnailURL string       `json:"thumbnail_url"`
	Status       string       `json:"status"`
	Batch        *BatchResult `json:"batch"`
}

// VideoRepository interface
type VideoRepository interface {
	GetVideo(ctx context.Context, videoID string) (*LearningItem, *errors.AppError)
	ListVideos(ctx context.Context, limit, offset int) ([]*LearningItem, int, *errors.AppError)
	CreateVideo(ctx context.Context, item *LearningItem) *errors.AppError
	UpdateVideo(ctx context.Context, item *LearningItem) *errors.AppError
	ToggleSaved(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError)
	StartQuiz(ctx context.Context, videoID, userID string) (string, *errors.AppError)
	ToggleTranscript(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError)
}

type videoRepository struct {
	db *client.PostgresClient
}

func NewVideoRepository(db *client.PostgresClient) VideoRepository {
	return &videoRepository{db: db}
}

func (r *videoRepository) GetVideo(ctx context.Context, videoID string) (*LearningItem, *errors.AppError) {
	query := `
		SELECT * FROM learning_items WHERE id = $1
	`

	var item LearningItem
	err := r.db.Pool.QueryRow(ctx, query, videoID).Scan(&item)
	if err != nil {
		return nil, errors.InternalWrap("failed to get video content", err)
	}

	return &item, nil
}

func (r *videoRepository) ListVideos(ctx context.Context, limit, offset int) ([]*LearningItem, int, *errors.AppError) {
	// 1. Get total count (เหมือนเดิม)
	countQuery := `SELECT COUNT(*) FROM learning_items WHERE feature_id = $1`
	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, FeatureID).Scan(&total)
	if err != nil {
		return nil, 0, errors.InternalWrap("failed to count video contents", err)
	}

	// 2. Get paginated results with LEFT JOIN & jsonb_agg
	query := `
		SELECT 
			l.id, l.feature_id, l.content, l.language, l.level, 
			l.details, l.metadata, l.tags, l.is_active, l.created_by, 
			l.created_at, l.updated_at,
			COALESCE(
				jsonb_agg(to_jsonb(ua)) FILTER (WHERE ua.id IS NOT NULL), 
				'[]'::jsonb
			) as actions
		FROM learning_items l
		LEFT JOIN user_actions ua 
			ON l.id = ua.learning_id 
			AND ua.action_type IN ('quiz_saved', 'quiz_transcript', 'submit_quiz')
			AND ua.deleted_at IS NULL
		WHERE l.feature_id = $1
		GROUP BY l.id
		ORDER BY l.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, FeatureID, limit, offset)
	if err != nil {
		return nil, 0, errors.InternalWrap("failed to list video contents", err)
	}
	defer rows.Close()

	var videos []*LearningItem
	for rows.Next() {
		var video LearningItem
		var actionsJSON []byte // ตัวแปรสำหรับรับก้อน JSON จาก DB

		err := rows.Scan(
			&video.ID,
			&video.FeatureID,
			&video.Content,
			&video.Language,
			&video.Level,
			&video.Details,
			&video.Metadata,
			&video.Tags,
			&video.IsActive,
			&video.CreatedBy,
			&video.CreatedAt,
			&video.UpdatedAt,
			&actionsJSON, // รับค่า jsonb เข้ามาเป็น bytes
		)
		if err != nil {
			return nil, 0, errors.InternalWrap("failed to scan video content", err)
		}

		// แปลง JSON string/bytes กลับเป็น Struct ของ Go
		if len(actionsJSON) > 0 {
			if err := json.Unmarshal(actionsJSON, &video.Actions); err != nil {
				return nil, 0, errors.InternalWrap("failed to unmarshal actions JSON", err)
			}
		}

		videos = append(videos, &video)
	}

	return videos, total, nil
}

func (r *videoRepository) CreateVideo(ctx context.Context, item *LearningItem) *errors.AppError {
	query := `
		INSERT INTO learning_items (
			id, feature_id, content, language, level, details, tags, metadata, is_active, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
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
		item.CreatedBy,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return errors.InternalWrap("failed to create video content", err)
	}

	return nil
}

func (r *videoRepository) UpdateVideo(ctx context.Context, item *LearningItem) *errors.AppError {
	query := `
		UPDATE learning_items
		SET feature_id = $1, content = $2, language = $3, level = $4, tags = $5, details = $6, metadata = $7, is_active = $8, created_by = $9
		WHERE id = $10
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
		item.CreatedBy,
		item.ID,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return errors.InternalWrap("failed to update video details", err)
	}

	return nil
}

func (r *videoRepository) ToggleSaved(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'quiz_saved', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id)
		DO UPDATE SET
			action_type = 'quiz_saved',
			deleted_at = CASE
				WHEN user_actions.action_type = 'quiz_saved' AND user_actions.deleted_at IS NULL THEN NOW()
				ELSE NULL
			END,
			updated_at = NOW()
		RETURNING id, deleted_at IS NULL
	`

	var actionID string
	var isSaved bool
	if err := r.db.Pool.QueryRow(ctx, query, userID, videoID).Scan(&actionID, &isSaved); err != nil {
		return "", false, errors.InternalWrap("failed to toggle video saved action", err)
	}

	return actionID, isSaved, nil
}

func (r *videoRepository) StartQuiz(ctx context.Context, videoID, userID string) (string, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'submit_quiz', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id)
		DO UPDATE SET
			action_type = 'submit_quiz',
			deleted_at = NULL,
			updated_at = NOW()
		RETURNING id
	`

	var actionID string
	if err := r.db.Pool.QueryRow(ctx, query, userID, videoID).Scan(&actionID); err != nil {
		return "", errors.InternalWrap("failed to start quiz action", err)
	}

	return actionID, nil
}

func (r *videoRepository) ToggleTranscript(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'quiz_transcript', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id)
		DO UPDATE SET
			action_type = 'quiz_transcript',
			deleted_at = CASE
				WHEN user_actions.action_type = 'quiz_transcript' AND user_actions.deleted_at IS NULL THEN NOW()
				ELSE NULL
			END,
			updated_at = NOW()
		RETURNING id, deleted_at IS NULL
	`

	var actionID string
	var isEnabled bool
	if err := r.db.Pool.QueryRow(ctx, query, userID, videoID).Scan(&actionID, &isEnabled); err != nil {
		return "", false, errors.InternalWrap("failed to toggle video transcript action", err)
	}

	return actionID, isEnabled, nil
}

