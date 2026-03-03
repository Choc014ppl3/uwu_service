package repository

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/client"
)

type LearningSummary struct {
	NewWords      int `json:"new_words"`
	NewSentences  int `json:"new_sentences"`
	PassWords     int `json:"pass_words"`
	PassSentences int `json:"pass_sentences"`
}

type UserStatsRepository interface {
	GetLearningSummary(ctx context.Context, userID, language string) (*LearningSummary, error)
}

type PostgresUserStatsRepository struct {
	db *client.PostgresClient
}

func NewPostgresUserStatsRepository(db *client.PostgresClient) *PostgresUserStatsRepository {
	return &PostgresUserStatsRepository{db: db}
}

func (r *PostgresUserStatsRepository) GetLearningSummary(ctx context.Context, userID, language string) (*LearningSummary, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN type = 'word' AND status = 'new' THEN 1 ELSE 0 END), 0) AS new_words,
			COALESCE(SUM(CASE WHEN type = 'sentence' AND status = 'new' THEN 1 ELSE 0 END), 0) AS new_sentences,
			COALESCE(SUM(CASE WHEN type = 'word' AND status = 'pass' THEN 1 ELSE 0 END), 0) AS pass_words,
			COALESCE(SUM(CASE WHEN type = 'sentence' AND status = 'pass' THEN 1 ELSE 0 END), 0) AS pass_sentences
		FROM user_stats
		WHERE user_id = $1 AND language = $2 AND deleted_at IS NULL
	`

	var summary LearningSummary
	err := r.db.Pool.QueryRow(ctx, query, userID, language).Scan(
		&summary.NewWords,
		&summary.NewSentences,
		&summary.PassWords,
		&summary.PassSentences,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get learning summary: %w", err)
	}

	return &summary, nil
}
