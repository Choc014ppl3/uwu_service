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
	PocketMission   FeatureType = 4
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
	List(ctx context.Context, limit, offset int) ([]*LearningItem, int, error)
	Update(ctx context.Context, item *LearningItem) error
	Delete(ctx context.Context, id uuid.UUID) error
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
		WHERE metadata->>'batch_id' = $1
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
