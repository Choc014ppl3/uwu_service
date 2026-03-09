package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

type LearningSummary struct {
	NewWords           int `json:"new_words"`
	NewSentences       int `json:"new_sentences"`
	PassWords          int `json:"pass_words"`
	PassSentences      int `json:"pass_sentences"`
	RecognizeWords     int `json:"recognize_words"`
	RecognizeSentences int `json:"recognize_sentences"`
}

type UserStatsRepository interface {
	GetLearningSummary(ctx context.Context, userID, language string, statuses []string) (*LearningSummary, error)
	UpsertUserStat(ctx context.Context, sourceID, userID uuid.UUID, content, language, sourceType string, listen, speak, read, write float64) error
}

type PostgresUserStatsRepository struct {
	db *client.PostgresClient
}

func NewPostgresUserStatsRepository(db *client.PostgresClient) *PostgresUserStatsRepository {
	return &PostgresUserStatsRepository{db: db}
}

func (r *PostgresUserStatsRepository) GetLearningSummary(ctx context.Context, userID, language string, statuses []string) (*LearningSummary, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Determine matching criteria based on requested statuses.
	// If statuses is empty, we act as 'Total' (all 4 skills).
	checkListen := true
	checkSpeak := true
	checkRead := true
	checkWrite := true

	if len(statuses) > 0 {
		hasListen, hasSpeak, hasRead, hasWrite := false, false, false, false
		for _, s := range statuses {
			switch s {
			case "listen":
				hasListen = true
			case "speak":
				hasSpeak = true
			case "read":
				hasRead = true
			case "write":
				hasWrite = true
			}
		}
		// Only check the ones provided
		checkListen = hasListen
		checkSpeak = hasSpeak
		checkRead = hasRead
		checkWrite = hasWrite
	}

	// Building the exact condition for recognize
	recCond := "1=1"
	if checkListen {
		recCond += " AND listen_count >= 9"
	}
	if checkSpeak {
		recCond += " AND speak_count >= 9"
	}
	if checkRead {
		recCond += " AND read_count >= 9"
	}
	if checkWrite {
		recCond += " AND write_count >= 9"
	}

	// Building the exact condition for pass
	passCond := "1=1"
	if checkListen {
		passCond += " AND listen_count >= 4 AND listen_count < 9"
	}
	if checkSpeak {
		passCond += " AND speak_count >= 4 AND speak_count < 9"
	}
	if checkRead {
		passCond += " AND read_count >= 4 AND read_count < 9"
	}
	if checkWrite {
		passCond += " AND write_count >= 4 AND write_count < 9"
	}

	// Building the exact condition for new
	newCond := "1=1"
	if checkListen {
		newCond += " AND listen_count < 4"
	}
	if checkSpeak {
		newCond += " AND speak_count < 4"
	}
	if checkRead {
		newCond += " AND read_count < 4"
	}
	if checkWrite {
		newCond += " AND write_count < 4"
	}

	query := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(CASE WHEN type = 'word' AND %s THEN 1 ELSE 0 END), 0) AS new_words,
			COALESCE(SUM(CASE WHEN type = 'sentence' AND %s THEN 1 ELSE 0 END), 0) AS new_sentences,
			COALESCE(SUM(CASE WHEN type = 'word' AND %s THEN 1 ELSE 0 END), 0) AS pass_words,
			COALESCE(SUM(CASE WHEN type = 'sentence' AND %s THEN 1 ELSE 0 END), 0) AS pass_sentences,
			COALESCE(SUM(CASE WHEN type = 'word' AND %s THEN 1 ELSE 0 END), 0) AS recognize_words,
			COALESCE(SUM(CASE WHEN type = 'sentence' AND %s THEN 1 ELSE 0 END), 0) AS recognize_sentences
		FROM user_stats
		WHERE user_id = $1 AND language = $2 AND deleted_at IS NULL
	`, newCond, newCond, passCond, passCond, recCond, recCond)

	var summary LearningSummary
	err := r.db.Pool.QueryRow(ctx, query, userID, language).Scan(
		&summary.NewWords,
		&summary.NewSentences,
		&summary.PassWords,
		&summary.PassSentences,
		&summary.RecognizeWords,
		&summary.RecognizeSentences,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get learning summary: %w", err)
	}

	return &summary, nil
}

func (r *PostgresUserStatsRepository) UpsertUserStat(ctx context.Context, sourceID, userID uuid.UUID, content, language, sourceType string, listen, speak, read, write float64) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `
		INSERT INTO user_stats (
			source_id, user_id, content, language, type, 
			listen_count, speak_count, read_count, write_count, status
		) VALUES (
			$1, $2, $3, $4, $5, 
			$6, $7, $8, $9,
			CASE 
				WHEN ($6::numeric + $7::numeric + $8::numeric + $9::numeric) > 9 THEN 'recognized'::user_stat_status_enum
				WHEN ($6::numeric + $7::numeric + $8::numeric + $9::numeric) > 4 THEN 'passed'::user_stat_status_enum
				ELSE 'new'::user_stat_status_enum
			END
		)
		ON CONFLICT (source_id, user_id) DO UPDATE SET
			listen_count = user_stats.listen_count + EXCLUDED.listen_count,
			speak_count  = user_stats.speak_count + EXCLUDED.speak_count,
			read_count   = user_stats.read_count + EXCLUDED.read_count,
			write_count  = user_stats.write_count + EXCLUDED.write_count,
			updated_at   = NOW(),
			status = CASE 
				WHEN (user_stats.listen_count + EXCLUDED.listen_count) + 
					(user_stats.speak_count + EXCLUDED.speak_count) + 
					(user_stats.read_count + EXCLUDED.read_count) + 
					(user_stats.write_count + EXCLUDED.write_count) > 9 
				THEN 'recognized'::user_stat_status_enum
				
				WHEN (user_stats.listen_count + EXCLUDED.listen_count) + 
					(user_stats.speak_count + EXCLUDED.speak_count) + 
					(user_stats.read_count + EXCLUDED.read_count) + 
					(user_stats.write_count + EXCLUDED.write_count) > 4 
				THEN 'passed'::user_stat_status_enum
				
				ELSE 'new'::user_stat_status_enum
			END
	`

	_, err := r.db.Pool.Exec(ctx, query,
		sourceID, userID, content, language, sourceType,
		listen, speak, read, write,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert user stat: %w", err)
	}

	return nil
}
