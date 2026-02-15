package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
)

// QuizQuestionRow represents a row in the quiz_questions table.
type QuizQuestionRow struct {
	ID           int             `json:"id"`
	LessonID     int             `json:"lesson_id"`
	Type         string          `json:"type"`
	SkillTag     string          `json:"skill_tag"`
	QuestionData json.RawMessage `json:"question_data"`
}

// QuestionData is the JSONB stored in quiz_questions.question_data.
// It holds the full question including its options and correct_order.
type QuestionData struct {
	Question     string       `json:"question"`
	Options      []QuizOption `json:"options"`
	CorrectOrder []string     `json:"correct_order,omitempty"`
}

// QuizRepository defines the interface for quiz data access.
type QuizRepository interface {
	CreateLessonFromVideo(ctx context.Context, videoID uuid.UUID, title string, videoURL string) (int, error)
	SaveQuizQuestions(ctx context.Context, lessonID int, items []QuizItem) error
	SaveRetellMissionPoints(ctx context.Context, lessonID int, items []RetellItem) error
	GetQuizQuestionsByVideoID(ctx context.Context, videoID uuid.UUID) ([]QuizQuestionRow, error)
	GetLessonIDByVideoID(ctx context.Context, videoID uuid.UUID) (int, error)
	SaveQuizLog(ctx context.Context, userID uuid.UUID, lessonID int, score int, maxScore int, answersSnapshot json.RawMessage) error
}

// PostgresQuizRepository implements QuizRepository.
type PostgresQuizRepository struct {
	db *client.PostgresClient
}

// NewPostgresQuizRepository creates a new PostgresQuizRepository.
func NewPostgresQuizRepository(db *client.PostgresClient) *PostgresQuizRepository {
	return &PostgresQuizRepository{db: db}
}

// CreateLessonFromVideo creates a lesson linked to a video. Returns the lesson ID.
func (r *PostgresQuizRepository) CreateLessonFromVideo(ctx context.Context, videoID uuid.UUID, title string, videoURL string) (int, error) {
	if r.db == nil || r.db.Pool == nil {
		return 0, fmt.Errorf("database not configured")
	}

	query := `INSERT INTO lessons (video_id, title, video_url) VALUES ($1, $2, $3) RETURNING id`
	var lessonID int
	err := r.db.Pool.QueryRow(ctx, query, videoID, title, videoURL).Scan(&lessonID)
	if err != nil {
		return 0, fmt.Errorf("failed to create lesson: %w", err)
	}
	return lessonID, nil
}

// SaveQuizQuestions inserts quiz items into the quiz_questions table.
func (r *PostgresQuizRepository) SaveQuizQuestions(ctx context.Context, lessonID int, items []QuizItem) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	for _, item := range items {
		// Map quiz type to the DB enum value
		dbType := mapQuizType(item.Type)

		// Build question_data JSONB
		qd := QuestionData{
			Question:     item.Question,
			Options:      item.Options,
			CorrectOrder: item.CorrectOrder,
		}
		qdJSON, err := json.Marshal(qd)
		if err != nil {
			return fmt.Errorf("failed to marshal question data: %w", err)
		}

		query := `INSERT INTO quiz_questions (lesson_id, type, skill_tag, question_data) VALUES ($1, $2, $3, $4)`
		_, err = r.db.Pool.Exec(ctx, query, lessonID, dbType, item.Category, qdJSON)
		if err != nil {
			return fmt.Errorf("failed to insert quiz question %d: %w", item.ID, err)
		}
	}

	return nil
}

// SaveRetellMissionPoints inserts retell check items into the retell_mission_points table.
func (r *PostgresQuizRepository) SaveRetellMissionPoints(ctx context.Context, lessonID int, items []RetellItem) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	for _, item := range items {
		// Store keywords as a JSON array with the main point text
		keywords, _ := json.Marshal([]string{item.Point})

		query := `INSERT INTO retell_mission_points (lesson_id, content, keywords, weight) VALUES ($1, $2, $3, $4)`
		_, err := r.db.Pool.Exec(ctx, query, lessonID, item.Point, keywords, 1)
		if err != nil {
			return fmt.Errorf("failed to insert retell point %d: %w", item.ID, err)
		}
	}

	return nil
}

// GetQuizQuestionsByVideoID fetches all quiz questions for a video via its lesson.
func (r *PostgresQuizRepository) GetQuizQuestionsByVideoID(ctx context.Context, videoID uuid.UUID) ([]QuizQuestionRow, error) {
	if r.db == nil || r.db.Pool == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT qq.id, qq.lesson_id, qq.type, COALESCE(qq.skill_tag, ''), qq.question_data
		FROM quiz_questions qq
		JOIN lessons l ON l.id = qq.lesson_id
		WHERE l.video_id = $1
		ORDER BY qq.id
	`
	rows, err := r.db.Pool.Query(ctx, query, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query quiz questions: %w", err)
	}
	defer rows.Close()

	var questions []QuizQuestionRow
	for rows.Next() {
		var q QuizQuestionRow
		if err := rows.Scan(&q.ID, &q.LessonID, &q.Type, &q.SkillTag, &q.QuestionData); err != nil {
			return nil, fmt.Errorf("failed to scan quiz question: %w", err)
		}
		questions = append(questions, q)
	}

	return questions, nil
}

// GetLessonIDByVideoID returns the lesson ID for a given video.
func (r *PostgresQuizRepository) GetLessonIDByVideoID(ctx context.Context, videoID uuid.UUID) (int, error) {
	if r.db == nil || r.db.Pool == nil {
		return 0, fmt.Errorf("database not configured")
	}

	query := `SELECT id FROM lessons WHERE video_id = $1`
	var lessonID int
	err := r.db.Pool.QueryRow(ctx, query, videoID).Scan(&lessonID)
	if err != nil {
		return 0, fmt.Errorf("lesson not found for video %s: %w", videoID, err)
	}
	return lessonID, nil
}

// SaveQuizLog records a user's quiz attempt.
func (r *PostgresQuizRepository) SaveQuizLog(ctx context.Context, userID uuid.UUID, lessonID int, score int, maxScore int, answersSnapshot json.RawMessage) error {
	if r.db == nil || r.db.Pool == nil {
		return fmt.Errorf("database not configured")
	}

	query := `INSERT INTO user_quiz_logs (user_id, lesson_id, score, max_score, answers_snapshot) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.Pool.Exec(ctx, query, userID, lessonID, score, maxScore, answersSnapshot)
	if err != nil {
		return fmt.Errorf("failed to save quiz log: %w", err)
	}
	return nil
}

// mapQuizType maps AI-generated quiz types to the DB enum values.
func mapQuizType(aiType string) string {
	switch aiType {
	case "single_choice":
		return "single_choice"
	case "multiple_response":
		return "multiple_response"
	case "multiple_choice":
		return "multiple_choice"
	case "ordering":
		return "ordering"
	default:
		return "multiple_choice"
	}
}
