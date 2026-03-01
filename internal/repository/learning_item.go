package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

type LearningItem struct {
	ID             uuid.UUID       `json:"id"`
	FeatureID      *FeatureType    `json:"feature_id"`
	Content        string          `json:"content"`
	LangCode       string          `json:"lang_code"`
	EstimatedLevel *string         `json:"estimated_level"`
	Details        json.RawMessage `json:"details"`
	Metadata       json.RawMessage `json:"metadata"`
	Tags           json.RawMessage `json:"tags"`
	IsActive       bool            `json:"is_active"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type FeatureType int

const (
	NativeImmersion FeatureType = 1
	GistQuiz        FeatureType = 2
	RetellStory     FeatureType = 3
	DialogueGuide   FeatureType = 4
	RhythmAndFlow   FeatureType = 5
	VocabularyReps  FeatureType = 6
	PrecisionCheck  FeatureType = 7
	StructureDrill  FeatureType = 8
	SparringMode    FeatureType = 9
	MissionGuide    FeatureType = 10
)

type LearningItemRepository interface {
	Create(ctx context.Context, item *LearningItem) error
	GetByID(ctx context.Context, id uuid.UUID) (*LearningItem, error)
	GetByBatchID(ctx context.Context, batchID string) ([]*LearningItem, error)
	GetByFeatureID(ctx context.Context, featureID int, limit, offset int) ([]*LearningItem, int, error)
	GetVideoPlaylist(ctx context.Context, userID string, statusFilter string, limit, offset int) ([]*LearningItem, int, error)
	List(ctx context.Context, limit, offset int) ([]*LearningItem, int, error)
	Update(ctx context.Context, item *LearningItem) error
	Delete(ctx context.Context, id uuid.UUID) error
	AddVideoAction(ctx context.Context, videoID uuid.UUID, userID uuid.UUID, actionType string) error
}

type PostgresLearningItemRepository struct {
	db *client.PostgresClient
}

func NewPostgresLearningItemRepository(db *client.PostgresClient) *PostgresLearningItemRepository {
	return &PostgresLearningItemRepository{db: db}
}

func (r *PostgresLearningItemRepository) Create(ctx context.Context, item *LearningItem) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO learning_items (
			feature_id, content, lang_code, estimated_level, details, tags, metadata, is_active
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.FeatureID,
		item.Content,
		item.LangCode,
		item.EstimatedLevel,
		item.Details,
		item.Tags,
		item.Metadata,
		item.IsActive,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create learning item: %w", err)
	}

	return nil
}

func (r *PostgresLearningItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*LearningItem, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, feature_id, content, lang_code, estimated_level, details, tags, metadata, is_active, created_at, updated_at
		FROM learning_items
		WHERE id = $1
	`

	var item LearningItem
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.FeatureID,
		&item.Content,
		&item.LangCode,
		&item.EstimatedLevel,
		&item.Details,
		&item.Tags,
		&item.Metadata,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get learning item: %w", err)
	}
	return &item, nil
}

func (r *PostgresLearningItemRepository) List(ctx context.Context, limit, offset int) ([]*LearningItem, int, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, 0, fmt.Errorf("database not configured")
	}

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM learning_items`
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count learning items: %w", err)
	}

	// Get paginated items
	query := `
		SELECT id, feature_id, content, lang_code, estimated_level, details, tags, metadata, is_active, created_at, updated_at
		FROM learning_items
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list learning items: %w", err)
	}
	defer rows.Close()

	var items []*LearningItem
	for rows.Next() {
		var item LearningItem
		if err := rows.Scan(
			&item.ID,
			&item.FeatureID,
			&item.Content,
			&item.LangCode,
			&item.EstimatedLevel,
			&item.Details,
			&item.Tags,
			&item.Metadata,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan learning item: %w", err)
		}
		items = append(items, &item)
	}

	return items, total, nil
}

func (r *PostgresLearningItemRepository) GetByFeatureID(ctx context.Context, featureID int, limit, offset int) ([]*LearningItem, int, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, 0, fmt.Errorf("database not configured")
	}

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM learning_items WHERE feature_id = $1`
	if err := r.db.Pool.QueryRow(ctx, countQuery, featureID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count learning items by feature: %w", err)
	}

	// Get paginated items
	query := `
		SELECT id, feature_id, content, lang_code, estimated_level, details, tags, metadata, is_active, created_at, updated_at
		FROM learning_items
		WHERE feature_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, featureID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list learning items by feature: %w", err)
	}
	defer rows.Close()

	var items []*LearningItem
	for rows.Next() {
		var item LearningItem
		if err := rows.Scan(
			&item.ID,
			&item.FeatureID,
			&item.Content,
			&item.LangCode,
			&item.EstimatedLevel,
			&item.Details,
			&item.Tags,
			&item.Metadata,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan learning item: %w", err)
		}
		items = append(items, &item)
	}

	return items, total, nil
}

// GetVideoPlaylist fetches new, saved, and done learning items within the past 2 weeks
func (r *PostgresLearningItemRepository) GetVideoPlaylist(ctx context.Context, userID string, statusFilter string, limit, offset int) ([]*LearningItem, int, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, 0, fmt.Errorf("database not configured")
	}

	// Build the base query based on statusFilter
	var condition string
	var args []interface{}
	args = append(args, userID)

	if statusFilter == "new" {
		condition = "AND ua.type IS NULL"
	} else if statusFilter == "saved" {
		condition = "AND ua.type = 'saved'"
	} else if statusFilter == "done" {
		condition = "AND ua.type = 'done'"
	} else {
		// No specific status filter or invalid filter, return all (new, saved, done)
		condition = ""
	}

	// This query fetches learning items that are videos (feature_id = 1) from the past 2 weeks
	// It joins with user_actions to determine if a video is "new" (no action), "saved", or "done"
	query := fmt.Sprintf(`
		SELECT 
			li.id, li.feature_id, li.content, li.lang_code, li.estimated_level, li.details, li.tags, li.metadata, li.is_active, li.created_at, li.updated_at,
			COALESCE(ua.type::text, 'new') as status
		FROM learning_items li
		LEFT JOIN user_actions ua ON li.id = ua.learning_item_id AND ua.user_id = $1
		WHERE li.feature_id = 1 AND li.created_at >= NOW() - INTERVAL '14 days' %s
		ORDER BY li.created_at DESC
		LIMIT $2 OFFSET $3
	`, condition)

	args = append(args, limit, offset)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get video playlist: %w", err)
	}
	defer rows.Close()

	var items []*LearningItem
	for rows.Next() {
		var item LearningItem
		var status string
		err := rows.Scan(
			&item.ID,
			&item.FeatureID,
			&item.Content,
			&item.LangCode,
			&item.EstimatedLevel,
			&item.Details,
			&item.Tags,
			&item.Metadata,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&status,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan playlist item: %w", err)
		}

		// Add status to metadata for frontend consumption
		if item.Metadata != nil {
			var metadataMap map[string]interface{}
			if err := json.Unmarshal(item.Metadata, &metadataMap); err == nil {
				metadataMap["status"] = status
				if updatedMetadata, err := json.Marshal(metadataMap); err == nil {
					item.Metadata = updatedMetadata
				}
			}
		} else {
			metadataMap := map[string]interface{}{"status": status}
			if newMetadata, err := json.Marshal(metadataMap); err == nil {
				item.Metadata = newMetadata
			}
		}

		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Get total count (using same condition and arguments)
	var total int
	countQuery := fmt.Sprintf(`
		SELECT COUNT(li.id) 
		FROM learning_items li
		LEFT JOIN user_actions ua ON li.id = ua.learning_item_id AND ua.user_id = $1
		WHERE li.feature_id = 1 AND li.created_at >= NOW() - INTERVAL '14 days' %s
	`, condition)

	err = r.db.Pool.QueryRow(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *PostgresLearningItemRepository) Update(ctx context.Context, item *LearningItem) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		UPDATE learning_items
		SET feature_id = $1, content = $2, lang_code = $3, estimated_level = $4, details = $5,
		    tags = $6, metadata = $7, is_active = $8, updated_at = NOW()
		WHERE id = $9
		RETURNING updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		item.FeatureID,
		item.Content,
		item.LangCode,
		item.EstimatedLevel,
		item.Details,
		item.Tags,
		item.Metadata,
		item.IsActive,
		item.ID,
	).Scan(&item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to update learning item: %w", err)
	}
	return nil
}

func (r *PostgresLearningItemRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `DELETE FROM learning_items WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete learning item: %w", err)
	}
	return nil
}

func (r *PostgresLearningItemRepository) GetByBatchID(ctx context.Context, batchID string) ([]*LearningItem, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, feature_id, content, lang_code, estimated_level, details, tags, metadata, is_active, created_at, updated_at
		FROM learning_items
		WHERE details->>'batch_id' = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get learning items by batch_id: %w", err)
	}
	defer rows.Close()

	var items []*LearningItem
	for rows.Next() {
		var item LearningItem
		if err := rows.Scan(
			&item.ID, &item.FeatureID, &item.Content, &item.LangCode, &item.EstimatedLevel,
			&item.Details, &item.Tags, &item.Metadata, &item.IsActive,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan learning item: %w", err)
		}
		items = append(items, &item)
	}
	return items, nil
}

// AddVideoAction adds or updates a video action.
func (r *PostgresLearningItemRepository) AddVideoAction(ctx context.Context, videoID uuid.UUID, userID uuid.UUID, actionType string) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	var query string
	switch actionType {
	case "passed":
		query = `
			INSERT INTO video_actions (video_id, user_id, type, total_pass, total_fail)
			VALUES ($1, $2, $3, 1, 0)
			ON CONFLICT (video_id, user_id) DO UPDATE 
			SET type = $3, total_pass = video_actions.total_pass + 1, updated_at = NOW(), deleted_at = NULL
		`
	case "failed":
		query = `
			INSERT INTO video_actions (video_id, user_id, type, total_pass, total_fail)
			VALUES ($1, $2, $3, 0, 1)
			ON CONFLICT (video_id, user_id) DO UPDATE 
			SET type = $3, total_fail = video_actions.total_fail + 1, updated_at = NOW(), deleted_at = NULL
		`
	case "saved":
		query = `
			INSERT INTO video_actions (video_id, user_id, type, deleted_at)
			VALUES ($1, $2, $3, NULL)
			ON CONFLICT (video_id, user_id) DO UPDATE 
			SET type = $3, 
				deleted_at = CASE WHEN video_actions.deleted_at IS NULL THEN NOW() ELSE NULL END, 
				updated_at = NOW()
		`
	default:
		return fmt.Errorf("invalid action type: %s", actionType)
	}

	_, err := r.db.Pool.Exec(ctx, query, videoID, userID, actionType)
	if err != nil {
		return fmt.Errorf("failed to add video action: %w", err)
	}

	return nil
}
