package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

type ConversationScenario struct {
	ID              uuid.UUID       `json:"id"`
	Topic           string          `json:"topic"`
	Description     string          `json:"description"`
	InteractionType string          `json:"interaction_type"`
	TargetLang      string          `json:"target_lang"`
	EstimatedTurns  string          `json:"estimated_turns"`
	DifficultyLevel int             `json:"difficulty_level"`
	Metadata        json.RawMessage `json:"metadata"`
	IsActive        bool            `json:"is_active"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ConversationScenarioRepository interface {
	Create(ctx context.Context, item *ConversationScenario) error
	GetByID(ctx context.Context, id uuid.UUID) (*ConversationScenario, error)
	UpdateMetadata(ctx context.Context, id uuid.UUID, metadata json.RawMessage) error
}

type PostgresScenarioRepository struct {
	db *client.PostgresClient
}

func NewPostgresScenarioRepository(db *client.PostgresClient) *PostgresScenarioRepository {
	return &PostgresScenarioRepository{db: db}
}

func (r *PostgresScenarioRepository) Create(ctx context.Context, item *ConversationScenario) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO conversation_scenarios (
			topic, description, interaction_type, target_lang, estimated_turns, difficulty_level, metadata, is_active
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		item.Topic,
		item.Description,
		item.InteractionType,
		item.TargetLang,
		item.EstimatedTurns,
		item.DifficultyLevel,
		item.Metadata,
		item.IsActive,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create scenario: %w", err)
	}

	return nil
}

func (r *PostgresScenarioRepository) GetByID(ctx context.Context, id uuid.UUID) (*ConversationScenario, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, topic, description, interaction_type, target_lang, estimated_turns, difficulty_level, metadata, is_active, created_at, updated_at
		FROM conversation_scenarios
		WHERE id = $1
	`

	var item ConversationScenario
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.Topic,
		&item.Description,
		&item.InteractionType,
		&item.TargetLang,
		&item.EstimatedTurns,
		&item.DifficultyLevel,
		&item.Metadata,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get scenario: %w", err)
	}

	return &item, nil
}

func (r *PostgresScenarioRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata json.RawMessage) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		UPDATE conversation_scenarios
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err := r.db.Pool.Exec(ctx, query, metadata, id)
	if err != nil {
		return fmt.Errorf("failed to update scenario metadata: %w", err)
	}

	return nil
}
