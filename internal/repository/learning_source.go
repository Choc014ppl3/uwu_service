package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// LearningSourceType represents the enum type for learning sources
type LearningSourceType string

const (
	LearningSourceTypeWord     LearningSourceType = "word"
	LearningSourceTypeSentence LearningSourceType = "sentence"
)

// LearningSource represents the learning_sources database table
type LearningSource struct {
	ID        uuid.UUID          `json:"id"`
	Content   string             `json:"content"`
	Language  string             `json:"language"`
	Type      LearningSourceType `json:"type"`
	Level     *string            `json:"level"`
	Tags      json.RawMessage    `json:"tags"`
	Media     json.RawMessage    `json:"media"`
	Metadata  json.RawMessage    `json:"metadata"`
	Translate json.RawMessage    `json:"translate"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type LearningSourceRepository interface {
	Create(ctx context.Context, item *LearningSource) error
	GetByBatchID(ctx context.Context, batchID string) ([]*LearningSource, error)
}

type PostgresLearningSourceRepository struct {
	db *client.PostgresClient
}

func NewPostgresLearningSourceRepository(db *client.PostgresClient) *PostgresLearningSourceRepository {
	return &PostgresLearningSourceRepository{db: db}
}

func (r *PostgresLearningSourceRepository) Create(ctx context.Context, item *LearningSource) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO learning_sources (
			content, language, type, level, tags, media, metadata, translate
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.Content,
		item.Language,
		item.Type,
		item.Level,
		item.Tags,
		item.Media,
		item.Metadata,
		item.Translate,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create learning source: %w", err)
	}

	return nil
}

func (r *PostgresLearningSourceRepository) GetByBatchID(ctx context.Context, batchID string) ([]*LearningSource, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, content, language, type, level, tags, media, metadata, translate, created_at, updated_at
		FROM learning_sources
		WHERE metadata->>'batch_id' = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get learning sources by batch_id: %w", err)
	}
	defer rows.Close()

	var items []*LearningSource
	for rows.Next() {
		var item LearningSource
		if err := rows.Scan(
			&item.ID, &item.Content, &item.Language, &item.Type, &item.Level,
			&item.Tags, &item.Media, &item.Metadata, &item.Translate,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan learning source: %w", err)
		}
		items = append(items, &item)
	}
	return items, nil
}
