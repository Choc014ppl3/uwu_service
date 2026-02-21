package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// RetellMissionPoint represents a row in retell_mission_points.
type RetellMissionPoint struct {
	ID       int             `json:"id"`
	LessonID int             `json:"lesson_id"`
	Content  string          `json:"content"`
	Keywords json.RawMessage `json:"keywords"`
	Weight   int             `json:"weight"`
}

// RetellSession represents a row in user_retell_sessions.
type RetellSession struct {
	ID                int             `json:"id"`
	UserID            uuid.UUID       `json:"user_id"`
	LessonID          int             `json:"lesson_id"`
	Status            string          `json:"status"`
	AttemptCount      int             `json:"attempt_count"`
	CollectedPointIDs json.RawMessage `json:"collected_point_ids"`
	CurrentScore      float64         `json:"current_score"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// RetellAudioLog represents a row in user_retell_audio_logs.
type RetellAudioLog struct {
	ID            int             `json:"id"`
	SessionID     int             `json:"session_id"`
	AudioURL      string          `json:"audio_url"`
	Transcript    string          `json:"transcript"`
	FoundPointIDs json.RawMessage `json:"found_point_ids"`
	AIFeedback    string          `json:"ai_feedback"`
	CreatedAt     time.Time       `json:"created_at"`
}

// RetellRepository defines the interface for retell data access.
type RetellRepository interface {
	GetMissionPoints(ctx context.Context, lessonID int) ([]RetellMissionPoint, error)
	GetOrCreateSession(ctx context.Context, userID uuid.UUID, lessonID int) (*RetellSession, error)
	UpdateSession(ctx context.Context, sessionID int, collectedIDs json.RawMessage, score float64, attemptCount int, status string) error
	SaveAudioLog(ctx context.Context, sessionID int, audioURL, transcript string, foundIDs json.RawMessage, feedback string) error
	ResetSession(ctx context.Context, userID uuid.UUID, lessonID int) (*RetellSession, error)
	GetVideoTranscriptByLessonID(ctx context.Context, lessonID int) (string, error)
}

// PostgresRetellRepository implements RetellRepository.
type PostgresRetellRepository struct {
	db *client.PostgresClient
}

// NewPostgresRetellRepository creates a new PostgresRetellRepository.
func NewPostgresRetellRepository(db *client.PostgresClient) *PostgresRetellRepository {
	return &PostgresRetellRepository{db: db}
}

// GetMissionPoints loads all retell mission points for a lesson.
func (r *PostgresRetellRepository) GetMissionPoints(ctx context.Context, lessonID int) ([]RetellMissionPoint, error) {
	query := `SELECT id, lesson_id, content, COALESCE(keywords, '[]'::jsonb), weight FROM retell_mission_points WHERE lesson_id = $1 ORDER BY id`
	rows, err := r.db.Pool.Query(ctx, query, lessonID)
	if err != nil {
		return nil, fmt.Errorf("failed to query mission points: %w", err)
	}
	defer rows.Close()

	var points []RetellMissionPoint
	for rows.Next() {
		var p RetellMissionPoint
		if err := rows.Scan(&p.ID, &p.LessonID, &p.Content, &p.Keywords, &p.Weight); err != nil {
			return nil, fmt.Errorf("failed to scan mission point: %w", err)
		}
		points = append(points, p)
	}
	return points, nil
}

// GetOrCreateSession finds an active (in_progress) session or creates a new one.
func (r *PostgresRetellRepository) GetOrCreateSession(ctx context.Context, userID uuid.UUID, lessonID int) (*RetellSession, error) {
	// Try to find existing in_progress session
	query := `
		SELECT id, user_id, lesson_id, status, attempt_count, collected_point_ids, current_score, created_at, updated_at
		FROM user_retell_sessions
		WHERE user_id = $1 AND lesson_id = $2 AND status = 'in_progress'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var s RetellSession
	err := r.db.Pool.QueryRow(ctx, query, userID, lessonID).Scan(
		&s.ID, &s.UserID, &s.LessonID, &s.Status, &s.AttemptCount,
		&s.CollectedPointIDs, &s.CurrentScore, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == nil {
		return &s, nil
	}

	// No active session found — create new one
	insertQuery := `
		INSERT INTO user_retell_sessions (user_id, lesson_id, status, attempt_count, collected_point_ids, current_score)
		VALUES ($1, $2, 'in_progress', 0, '[]'::jsonb, 0.0)
		RETURNING id, user_id, lesson_id, status, attempt_count, collected_point_ids, current_score, created_at, updated_at
	`
	var ns RetellSession
	err = r.db.Pool.QueryRow(ctx, insertQuery, userID, lessonID).Scan(
		&ns.ID, &ns.UserID, &ns.LessonID, &ns.Status, &ns.AttemptCount,
		&ns.CollectedPointIDs, &ns.CurrentScore, &ns.CreatedAt, &ns.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create retell session: %w", err)
	}
	return &ns, nil
}

// UpdateSession updates session state after an attempt.
func (r *PostgresRetellRepository) UpdateSession(ctx context.Context, sessionID int, collectedIDs json.RawMessage, score float64, attemptCount int, status string) error {
	query := `
		UPDATE user_retell_sessions
		SET collected_point_ids = $1, current_score = $2, attempt_count = $3, status = $4, updated_at = NOW()
		WHERE id = $5
	`
	_, err := r.db.Pool.Exec(ctx, query, collectedIDs, score, attemptCount, status, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update retell session: %w", err)
	}
	return nil
}

// SaveAudioLog records an attempt's audio log.
func (r *PostgresRetellRepository) SaveAudioLog(ctx context.Context, sessionID int, audioURL, transcript string, foundIDs json.RawMessage, feedback string) error {
	query := `
		INSERT INTO user_retell_audio_logs (session_id, audio_url, transcript, found_point_ids, ai_feedback)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Pool.Exec(ctx, query, sessionID, audioURL, transcript, foundIDs, feedback)
	if err != nil {
		return fmt.Errorf("failed to save audio log: %w", err)
	}
	return nil
}

// ResetSession marks the current session as failed and creates a fresh one.
func (r *PostgresRetellRepository) ResetSession(ctx context.Context, userID uuid.UUID, lessonID int) (*RetellSession, error) {
	// Mark any existing in_progress session as failed
	_, _ = r.db.Pool.Exec(ctx,
		`UPDATE user_retell_sessions SET status = 'failed', updated_at = NOW() WHERE user_id = $1 AND lesson_id = $2 AND status = 'in_progress'`,
		userID, lessonID,
	)

	// Create a fresh session
	query := `
		INSERT INTO user_retell_sessions (user_id, lesson_id, status, attempt_count, collected_point_ids, current_score)
		VALUES ($1, $2, 'in_progress', 0, '[]'::jsonb, 0.0)
		RETURNING id, user_id, lesson_id, status, attempt_count, collected_point_ids, current_score, created_at, updated_at
	`
	var s RetellSession
	err := r.db.Pool.QueryRow(ctx, query, userID, lessonID).Scan(
		&s.ID, &s.UserID, &s.LessonID, &s.Status, &s.AttemptCount,
		&s.CollectedPointIDs, &s.CurrentScore, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new retell session: %w", err)
	}
	return &s, nil
}

// GetVideoTranscriptByLessonID fetches the original video transcript via lesson → video.
// The transcript is stored as a JSONB array of segments; this returns concatenated text.
func (r *PostgresRetellRepository) GetVideoTranscriptByLessonID(ctx context.Context, lessonID int) (string, error) {
	query := `
		SELECT COALESCE(v.transcript, '[]'::jsonb)
		FROM videos v
		JOIN lessons l ON l.video_id = v.id
		WHERE l.id = $1
	`
	var transcriptJSON json.RawMessage
	err := r.db.Pool.QueryRow(ctx, query, lessonID).Scan(&transcriptJSON)
	if err != nil {
		return "", fmt.Errorf("failed to get video transcript: %w", err)
	}

	// Parse JSONB segments into text
	var segments []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(transcriptJSON, &segments); err != nil {
		return "", fmt.Errorf("failed to parse transcript segments: %w", err)
	}

	var result string
	for _, seg := range segments {
		result += seg.Text + " "
	}
	return result, nil
}
