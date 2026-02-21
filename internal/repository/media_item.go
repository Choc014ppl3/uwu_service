package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// MediaItem represents a record in the media_items table.
type MediaItem struct {
	ID        uuid.UUID       `json:"id"`
	FilePath  string          `json:"file_path"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// MediaItemRepository defines the interface for media item data access.
type MediaItemRepository interface {
	Create(ctx context.Context, item *MediaItem) error
	GetBySystemID(ctx context.Context, systemID uuid.UUID) ([]*MediaItem, error)
}

// PostgresMediaItemRepository implements MediaItemRepository with PostgreSQL.
type PostgresMediaItemRepository struct {
	db *client.PostgresClient
}

// NewPostgresMediaItemRepository creates a new PostgresMediaItemRepository.
func NewPostgresMediaItemRepository(db *client.PostgresClient) *PostgresMediaItemRepository {
	return &PostgresMediaItemRepository{db: db}
}

// Create inserts a new media item record.
func (r *PostgresMediaItemRepository) Create(ctx context.Context, item *MediaItem) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO media_items (
			file_path, metadata, created_by
		) VALUES (
			$1, $2, $3
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.FilePath,
		item.Metadata,
		item.CreatedBy,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create media item: %w", err)
	}

	return nil
}

// GetBySystemID retrieves all media items linked to a specific system ID (e.g., learning_item_id).
func (r *PostgresMediaItemRepository) GetBySystemID(ctx context.Context, systemID uuid.UUID) ([]*MediaItem, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, file_path, metadata, created_by, created_at, updated_at
		FROM media_items
		WHERE system_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, systemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media items: %w", err)
	}
	defer rows.Close()

	var items []*MediaItem
	for rows.Next() {
		var item MediaItem
		if err := rows.Scan(
			&item.ID,
			&item.FilePath,
			&item.Metadata,
			&item.CreatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan media item: %w", err)
		}
		items = append(items, &item)
	}

	return items, nil
}
