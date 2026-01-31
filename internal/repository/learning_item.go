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
	ID        uuid.UUID       `json:"id"`
	Content   string          `json:"content"`
	LangCode  string          `json:"lang_code"`
	Meanings  json.RawMessage `json:"meanings"`
	Reading   json.RawMessage `json:"reading"`
	Type      string          `json:"type"`
	Tags      []string        `json:"tags"`
	Media     json.RawMessage `json:"media"`
	Metadata  json.RawMessage `json:"metadata"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type LearningItemRepository interface {
	Create(ctx context.Context, item *LearningItem) error
	GetByID(ctx context.Context, id uuid.UUID) (*LearningItem, error)
	List(ctx context.Context, limit, offset int) ([]*LearningItem, int, error)
	Update(ctx context.Context, item *LearningItem) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateMedia(ctx context.Context, id uuid.UUID, media json.RawMessage) error
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
			content, lang_code, meanings, reading, type, tags, media, metadata, is_active
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.Content,
		item.LangCode,
		item.Meanings,
		item.Reading,
		item.Type,
		item.Tags,
		item.Media,
		item.Metadata,
		item.IsActive,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create learning item: %w", err)
	}

	return nil
}

func (r *PostgresLearningItemRepository) UpdateMedia(ctx context.Context, id uuid.UUID, media json.RawMessage) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		UPDATE learning_items
		SET media = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Pool.Exec(ctx, query, media, id)
	if err != nil {
		return fmt.Errorf("failed to update learning item media: %w", err)
	}
	return nil
}

func (r *PostgresLearningItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*LearningItem, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, content, lang_code, meanings, reading, type, tags, media, metadata, is_active, created_at, updated_at
		FROM learning_items
		WHERE id = $1
	`

	var item LearningItem
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.Content,
		&item.LangCode,
		&item.Meanings,
		&item.Reading,
		&item.Type,
		&item.Tags,
		&item.Media,
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
		SELECT id, content, lang_code, meanings, reading, type, tags, media, metadata, is_active, created_at, updated_at
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
			&item.Content,
			&item.LangCode,
			&item.Meanings,
			&item.Reading,
			&item.Type,
			&item.Tags,
			&item.Media,
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
		SET content = $1, lang_code = $2, meanings = $3, reading = $4, type = $5, 
		    tags = $6, media = $7, metadata = $8, is_active = $9, updated_at = NOW()
		WHERE id = $10
		RETURNING updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		item.Content,
		item.LangCode,
		item.Meanings,
		item.Reading,
		item.Type,
		item.Tags,
		item.Media,
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
