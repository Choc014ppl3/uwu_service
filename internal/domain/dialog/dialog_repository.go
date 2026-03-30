package dialog

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
const FeatureID = 2

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

// DialogActions model
type DialogActions struct {
	Type struct {
		Saved  int `json:"saved"`
		Chat   int `json:"chat"`
		Speech int `json:"speech"`
		Passed int `json:"passed"`
	} `json:"type"`
	User struct {
		Saved  bool `json:"saved"`
		Chat   bool `json:"chat"`
		Speech bool `json:"speech"`
	} `json:"user"`
}

// LearningItem model
type LearningItem struct {
	ID        uuid.UUID       `json:"id"`
	FeatureID int             `json:"feature_id"`
	Content   string          `json:"content"`
	Language  string          `json:"language"`
	Level     string          `json:"level"`
	Tags      json.RawMessage `json:"tags"`
	Details   json.RawMessage `json:"details"`
	Metadata  json.RawMessage `json:"metadata"`
	IsActive  bool            `json:"is_active"`
	CreatedBy string          `json:"created_by"`
	CreatedAt *time.Time      `json:"created_at"`
	UpdatedAt *time.Time      `json:"updated_at"`
	// Learning Item Actions
	Actions DialogActions `json:"actions"`
}

// DialogDetails is the structure of the details field in LearningItem model
type DialogDetails struct {
	Topic       string     `json:"topic"`
	Description string     `json:"description"`
	Language    string     `json:"language"`
	Level       string     `json:"level"`
	Tags        []string   `json:"tags"`
	ImagePrompt string     `json:"image_prompt,omitempty"`
	ImageURL    string     `json:"image_url,omitempty"`
	AudioURL    string     `json:"audio_url,omitempty"`
	SpeechMode  SpeechMode `json:"speech_mode"`
	ChatMode    ChatMode   `json:"chat_mode"`
}

// DialogRepository interface
type DialogRepository interface {
	GetDialog(ctx context.Context, dialogID, userID string) (*LearningItem, *errors.AppError)
	ListDialogs(ctx context.Context, limit, offset int) ([]*LearningItem, int, *errors.AppError)
	CreateDialog(ctx context.Context, item *LearningItem) *errors.AppError
	UpdateDialog(ctx context.Context, item *LearningItem) *errors.AppError
	GetActionByUserID(ctx context.Context, learningID, userID, actionType string) (*UserAction, bool, *errors.AppError)
	ToggleSaved(ctx context.Context, dialogID, userID string) (string, bool, *errors.AppError)
	StartSpeech(ctx context.Context, dialogID, userID string, metadata json.RawMessage) (string, *errors.AppError)
	StartChat(ctx context.Context, dialogID, userID string, metadata json.RawMessage) (string, *errors.AppError)
	SubmitSpeechAction(ctx context.Context, actionID, userID string, metadataJSON []byte) *errors.AppError
	GetChatAction(ctx context.Context, actionID, userID string) (*UserAction, *errors.AppError)
	UpdateChatAction(ctx context.Context, actionID, userID string, metadataJSON []byte) *errors.AppError
}

type dialogRepository struct {
	db *client.PostgresClient
}

func NewDialogRepository(db *client.PostgresClient) DialogRepository {
	return &dialogRepository{db: db}
}

func (r *dialogRepository) GetDialog(ctx context.Context, dialogID, userID string) (*LearningItem, *errors.AppError) {
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
			AND ua.action_type IN ('dialogue_saved', 'submit_chat', 'submit_speech')
			AND ua.deleted_at IS NULL
		WHERE l.id = $1 AND l.feature_id = $2
		GROUP BY l.id
	`

	var item LearningItem
	var actionsJSON []byte

	err := r.db.Pool.QueryRow(ctx, query, dialogID, FeatureID).Scan(
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
			return nil, errors.NotFound("dialog content not found")
		}
		return nil, errors.InternalWrap("failed to get dialog content", err)
	}

	// Calculate counts and user status from actionsJSON logic
	if len(actionsJSON) > 0 {
		var rawActions []struct {
			UserID     string `json:"user_id"`
			ActionType string `json:"action_type"`
		}
		if err := json.Unmarshal(actionsJSON, &rawActions); err == nil {
			userActionsMap := make(map[string]map[string]bool)
			for _, action := range rawActions {
				if _, ok := userActionsMap[action.UserID]; !ok {
					userActionsMap[action.UserID] = make(map[string]bool)
				}
				userActionsMap[action.UserID][action.ActionType] = true

				switch action.ActionType {
				case "dialogue_saved":
					item.Actions.Type.Saved++
					if action.UserID == userID {
						item.Actions.User.Saved = true
					}
				case "submit_chat":
					item.Actions.Type.Chat++
					if action.UserID == userID {
						item.Actions.User.Chat = true
					}
				case "submit_speech":
					item.Actions.Type.Speech++
					if action.UserID == userID {
						item.Actions.User.Speech = true
					}
				}
			}

			// Intersection: submit_chat AND submit_speech
			for _, actions := range userActionsMap {
				if actions["submit_chat"] && actions["submit_speech"] {
					item.Actions.Type.Passed++
				}
			}
		}
	}

	return &item, nil
}

func (r *dialogRepository) ListDialogs(ctx context.Context, limit, offset int) ([]*LearningItem, int, *errors.AppError) {
	// 1. Get total count
	countQuery := `SELECT COUNT(*) FROM learning_items WHERE feature_id = $1`
	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, FeatureID).Scan(&total)
	if err != nil {
		return nil, 0, errors.InternalWrap("failed to count dialog contents", err)
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
		return nil, 0, errors.InternalWrap("failed to list dialog contents", err)
	}
	defer rows.Close()

	var dialogs []*LearningItem
	for rows.Next() {
		var dialog LearningItem

		err := rows.Scan(
			&dialog.ID,
			&dialog.FeatureID,
			&dialog.Content,
			&dialog.Language,
			&dialog.Level,
			&dialog.Details,
			&dialog.Metadata,
			&dialog.Tags,
			&dialog.IsActive,
			&dialog.CreatedBy,
			&dialog.CreatedAt,
			&dialog.UpdatedAt,
		)
		if err != nil {
			return nil, 0, errors.InternalWrap("failed to scan dialog content", err)
		}

		dialog.Actions = DialogActions{}
		dialogs = append(dialogs, &dialog)
	}

	return dialogs, total, nil
}

func (r *dialogRepository) CreateDialog(ctx context.Context, item *LearningItem) *errors.AppError {
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
		return errors.InternalWrap("failed to create dialog content", err)
	}

	return nil
}

func (r *dialogRepository) UpdateDialog(ctx context.Context, item *LearningItem) *errors.AppError {
	query := `
		UPDATE learning_items
		SET feature_id = $1, content = $2, language = $3, level = $4, tags = $5, details = $6, metadata = $7, is_active = $8, created_by = $9
		WHERE id = $10
	`

	cmdTag, err := r.db.Pool.Exec(ctx, query,
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
	)

	if err != nil {
		return errors.InternalWrap("failed to update dialog details", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.NotFound("dialog content not found")
	}

	return nil
}

func (r *dialogRepository) GetActionByUserID(ctx context.Context, learningID, userID, actionType string) (*UserAction, bool, *errors.AppError) {
	query := `
		SELECT id, user_id, learning_id, action_type, metadata, created_at, updated_at
		FROM user_actions
		WHERE learning_id = $1 AND user_id = $2 AND action_type = $3 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var action UserAction
	err := r.db.Pool.QueryRow(ctx, query, learningID, userID, actionType).Scan(
		&action.ID,
		&action.UserID,
		&action.LearningID,
		&action.ActionType,
		&action.Metadata,
		&action.CreatedAt,
		&action.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, errors.InternalWrap("failed to get action by user ID", err)
	}

	return &action, true, nil
}

func (r *dialogRepository) ToggleSaved(ctx context.Context, dialogID, userID string) (string, bool, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'dialogue_saved', '{}'::jsonb, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
			action_type = 'dialogue_saved',
			deleted_at = CASE
				WHEN user_actions.action_type = 'dialogue_saved' AND user_actions.deleted_at IS NULL THEN NOW()
				ELSE NULL
			END,
			updated_at = NOW()
		RETURNING id, deleted_at IS NULL
	`

	var actionID string
	var isSaved bool
	if err := r.db.Pool.QueryRow(ctx, query, userID, dialogID).Scan(&actionID, &isSaved); err != nil {
		return "", false, errors.InternalWrap("failed to toggle dialog saved action", err)
	}

	return actionID, isSaved, nil
}

func (r *dialogRepository) StartSpeech(ctx context.Context, dialogID, userID string, metadata json.RawMessage) (string, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'submit_speech', $3, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
			metadata = EXCLUDED.metadata,
			deleted_at = NULL,
			updated_at = NOW()
		RETURNING id
	`

	var actionID string
	if err := r.db.Pool.QueryRow(ctx, query, userID, dialogID, metadata).Scan(&actionID); err != nil {
		return "", errors.InternalWrap("failed to start speech action", err)
	}

	return actionID, nil
}

func (r *dialogRepository) StartChat(ctx context.Context, dialogID, userID string, metadata json.RawMessage) (string, *errors.AppError) {
	query := `
		INSERT INTO user_actions (user_id, learning_id, action_type, metadata, deleted_at)
		VALUES ($1, $2, 'submit_chat', $3, NULL)
		ON CONFLICT (learning_id, user_id, action_type)
		DO UPDATE SET
			metadata = EXCLUDED.metadata,
			deleted_at = NULL,
			updated_at = NOW()
		RETURNING id
	`

	var actionID string
	if err := r.db.Pool.QueryRow(ctx, query, userID, dialogID, metadata).Scan(&actionID); err != nil {
		return "", errors.InternalWrap("failed to start chat action", err)
	}

	return actionID, nil
}

func (r *dialogRepository) SubmitSpeechAction(ctx context.Context, actionID, userID string, metadataJSON []byte) *errors.AppError {
	query := `
		UPDATE user_actions
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND action_type = 'submit_speech'
	`

	cmdTag, err := r.db.Pool.Exec(ctx, query, metadataJSON, actionID, userID)
	if err != nil {
		return errors.InternalWrap("failed to submit speech action", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.NotFound("speech action not found or unauthorized")
	}

	return nil
}

func (r *dialogRepository) GetChatAction(ctx context.Context, actionID, userID string) (*UserAction, *errors.AppError) {
	query := `
		SELECT id, user_id, action_type, metadata, created_at, updated_at
		FROM user_actions
		WHERE id = $1 AND user_id = $2 AND action_type = 'submit_chat' AND deleted_at IS NULL
	`

	var action UserAction
	err := r.db.Pool.QueryRow(ctx, query, actionID, userID).Scan(
		&action.ID,
		&action.UserID,
		&action.ActionType,
		&action.Metadata,
		&action.CreatedAt,
		&action.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("chat action not found or unauthorized")
		}
		return nil, errors.InternalWrap("failed to get chat action", err)
	}

	return &action, nil
}

func (r *dialogRepository) UpdateChatAction(ctx context.Context, actionID, userID string, metadataJSON []byte) *errors.AppError {
	query := `
		UPDATE user_actions
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND action_type = 'submit_chat'
	`

	cmdTag, err := r.db.Pool.Exec(ctx, query, metadataJSON, actionID, userID)
	if err != nil {
		return errors.InternalWrap("failed to update chat action", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.NotFound("chat action not found or unauthorized")
	}

	return nil
}
