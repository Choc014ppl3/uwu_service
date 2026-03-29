package video

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// Constants
const FeatureID = 1

// User Action model
type UserAction struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	LearningID string          `json:"learning_id"`
	ActionType string          `json:"action_type"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	DeletedAt  *time.Time      `json:"deleted_at"`
}

// VideoActions model
type VideoActions struct {
	Type struct {
		Saved  int `json:"saved"`
		Quiz   int `json:"quiz"`
		Retell int `json:"retell"`
	} `json:"type"`
	User struct {
		Saved      bool `json:"saved"`
		Quiz       bool `json:"quiz"`
		Retell     bool `json:"retell"`
		Transcript bool `json:"transcript"`
	} `json:"user"`
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
	Actions VideoActions `json:"actions"`
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
	VideoURL     string `json:"video_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// VideoRepository interface
type VideoRepository interface {
	GetVideo(ctx context.Context, videoID, userID string) (*LearningItem, *errors.AppError)
	ListVideos(ctx context.Context, limit, offset int) ([]*LearningItem, int, *errors.AppError)
	CreateVideo(ctx context.Context, item *LearningItem) *errors.AppError
	UpdateVideo(ctx context.Context, item *LearningItem) *errors.AppError
	ToggleSaved(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError)
	StartQuiz(ctx context.Context, videoID, userID string, metadata json.RawMessage) (string, *errors.AppError)
	StartRetell(ctx context.Context, videoID, userID string, metadata json.RawMessage) (string, *errors.AppError)
	ToggleTranscript(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError)
	GetQuizAction(ctx context.Context, actionID string) (*UserAction, *errors.AppError)
	GetActionByUserID(ctx context.Context, videoID, userID, actionType string) (*UserAction, bool, *errors.AppError)
	UpdateQuizAction(ctx context.Context, actionID string, metadata json.RawMessage) *errors.AppError
}

type videoRepository struct {
	db *client.PostgresClient
}

func NewVideoRepository(db *client.PostgresClient) VideoRepository {
	return &videoRepository{db: db}
}

func (r *videoRepository) GetVideo(ctx context.Context, videoID, userID string) (*LearningItem, *errors.AppError) {
	query := `
		SELECT 
			l.id, l.feature_id, l.content, l.language, l.level,
			l.details, l.metadata, l.tags, l.is_active, l.created_by,
			l.created_at, l.updated_at,
			COALESCE(
				jsonb_agg(jsonb_build_object(
					'user_id', ua.user_id,
					'action_type', ua.action_type
				)) FILTER (WHERE ua.id IS NOT NULL),
				'[]'::jsonb
			) as actions
		FROM learning_items l
		LEFT JOIN user_actions ua
			ON l.id = ua.learning_id
			AND ua.action_type IN ('quiz_saved', 'quiz_transcript', 'submit_quiz', 'submit_retell')
			AND ua.deleted_at IS NULL
		WHERE l.id = $1 AND l.feature_id = $2
		GROUP BY l.id
	`

	var item LearningItem
	var actionsJSON []byte

	err := r.db.Pool.QueryRow(ctx, query, videoID, FeatureID).Scan(
		&item.ID,
		&item.FeatureID,
		&item.Content,
		&item.Language,
		&item.Level,
		&item.Details,
		&item.Metadata,
		&item.Tags,
		&item.IsActive,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&actionsJSON,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("video content not found")
		}
		return nil, errors.InternalWrap("failed to get video content", err)
	}

	// Calculate counts and user status from actionsJSON logic
	if len(actionsJSON) > 0 {
		var rawActions []struct {
			UserID     string `json:"user_id"`
			ActionType string `json:"action_type"`
		}
		if err := json.Unmarshal(actionsJSON, &rawActions); err == nil {
			for _, action := range rawActions {
				switch action.ActionType {
				case "quiz_saved":
					item.Actions.Type.Saved++
					if action.UserID == userID {
						item.Actions.User.Saved = true
					}
				case "quiz_transcript":
					if action.UserID == userID {
						item.Actions.User.Transcript = true
					}
				case "submit_quiz":
					item.Actions.Type.Quiz++
					if action.UserID == userID {
						item.Actions.User.Quiz = true
					}
				case "submit_retell":
					item.Actions.Type.Retell++
					if action.UserID == userID {
						item.Actions.User.Retell = true
					}
				}
			}
		}
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
			l.created_at, l.updated_at
		FROM learning_items l
		WHERE l.feature_id = $1
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
		)
		if err != nil {
			return nil, 0, errors.InternalWrap("failed to scan video content", err)
		}

		video.Actions = VideoActions{}
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

func (r *videoRepository) StartQuiz(ctx context.Context, videoID, userID string, metadata json.RawMessage) (string, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'submit_quiz', $3, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
			metadata = EXCLUDED.metadata,
			deleted_at = NULL,
			updated_at = NOW()
		RETURNING id
	`

	var actionID string
	if err := r.db.Pool.QueryRow(ctx, query, userID, videoID, metadata).Scan(&actionID); err != nil {
		return "", errors.InternalWrap("failed to start quiz action", err)
	}

	return actionID, nil
}

func (r *videoRepository) StartRetell(ctx context.Context, videoID, userID string, metadata json.RawMessage) (string, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'submit_retell', $3, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
			metadata = EXCLUDED.metadata,
			deleted_at = NULL,
			updated_at = NOW()
		RETURNING id
	`

	var actionID string
	if err := r.db.Pool.QueryRow(ctx, query, userID, videoID, metadata).Scan(&actionID); err != nil {
		return "", errors.InternalWrap("failed to start retell action", err)
	}

	return actionID, nil
}

func (r *videoRepository) ToggleSaved(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'quiz_saved', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
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

func (r *videoRepository) ToggleTranscript(ctx context.Context, videoID, userID string) (string, bool, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'quiz_transcript', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
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

func (r *videoRepository) GetQuizAction(ctx context.Context, actionID string) (*UserAction, *errors.AppError) {
	query := `
		SELECT id, user_id, learning_id, action_type, metadata, created_at, updated_at, deleted_at
		FROM user_actions
		WHERE id = $1 AND deleted_at IS NULL
	`

	var a UserAction
	row := r.db.Pool.QueryRow(ctx, query, actionID)
	if err := row.Scan(&a.ID, &a.UserID, &a.LearningID, &a.ActionType, &a.Metadata, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("quiz action not found")
		}
		return nil, errors.InternalWrap("failed to get quiz action", err)
	}

	return &a, nil
}

func (r *videoRepository) GetActionByUserID(ctx context.Context, videoID, userID, actionType string) (*UserAction, bool, *errors.AppError) {
	query := `
		SELECT id, user_id, learning_id, action_type, metadata, created_at, updated_at, deleted_at
		FROM user_actions
		WHERE learning_id = $1 AND user_id = $2 AND action_type = $3 AND deleted_at IS NULL
		LIMIT 1
	`

	var a UserAction
	err := r.db.Pool.QueryRow(ctx, query, videoID, userID, actionType).Scan(
		&a.ID, &a.UserID, &a.LearningID, &a.ActionType, &a.Metadata, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, errors.InternalWrap("failed to get quiz action by user id", err)
	}

	return &a, true, nil
}

func (r *videoRepository) UpdateQuizAction(ctx context.Context, actionID string, metadata json.RawMessage) *errors.AppError {
	query := `
		UPDATE user_actions
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err := r.db.Pool.Exec(ctx, query, metadata, actionID)
	if err != nil {
		return errors.InternalWrap("failed to update quiz action metadata", err)
	}

	return nil
}
