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
