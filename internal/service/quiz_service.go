package service

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// QuizService handles quiz grading logic.
type QuizService struct {
	quizRepo repository.QuizRepository
}

// NewQuizService creates a new QuizService.
func NewQuizService(quizRepo repository.QuizRepository) *QuizService {
	return &QuizService{quizRepo: quizRepo}
}

// --- Request / Response types ---

// QuizGradeRequest is the POST body for grading a quiz.
type QuizGradeRequest struct {
	Answers []UserAnswer `json:"answers"`
}

// UserAnswer represents a single answer submitted by the user.
type UserAnswer struct {
	QuestionID      int      `json:"question_id"`
	SelectedOptions []string `json:"selected_options"`
}

// QuizGradeResponse is the response returned to the client.
type QuizGradeResponse struct {
	Summary GradeSummary     `json:"summary"`
	Results []QuestionResult `json:"results"`
}

// GradeSummary holds the overall score info.
type GradeSummary struct {
	TotalQuestions int     `json:"total_questions"`
	CorrectCount   int     `json:"correct_count"`
	TotalScore     int     `json:"total_score"`
	Percentage     float64 `json:"percentage"`
}

// QuestionResult holds the grading result for a single question.
type QuestionResult struct {
	QuestionID     int      `json:"question_id"`
	Status         string   `json:"status"` // "correct" or "incorrect"
	UserSelected   []string `json:"user_selected"`
	CorrectOptions []string `json:"correct_options"`
	Explanation    string   `json:"explanation,omitempty"`
}

// GradeQuiz loads quiz questions from the quiz_questions table and grades the user's answers.
func (s *QuizService) GradeQuiz(ctx context.Context, videoID string, req QuizGradeRequest) (*QuizGradeResponse, error) {
	vid, err := uuid.Parse(videoID)
	if err != nil {
		return nil, errors.New(errors.ErrValidation, "invalid video ID")
	}

	// Load quiz questions from quiz_questions table (via lesson → video)
	questionRows, err := s.quizRepo.GetQuizQuestionsByVideoID(ctx, vid)
	if err != nil {
		return nil, errors.New(errors.ErrNotFound, "quiz not found for this video")
	}

	if len(questionRows) == 0 {
		return nil, errors.New(errors.ErrNotFound, "quiz has no questions")
	}

	// Build answer lookup: question ID → user answer
	answerMap := make(map[int][]string, len(req.Answers))
	for _, a := range req.Answers {
		answerMap[a.QuestionID] = a.SelectedOptions
	}

	// Grade each question
	results := make([]QuestionResult, 0, len(questionRows))
	correctCount := 0

	for _, row := range questionRows {
		// Parse question_data JSONB
		var qd repository.QuestionData
		if err := json.Unmarshal(row.QuestionData, &qd); err != nil {
			continue // skip unparseable questions
		}

		userSelected := answerMap[row.ID]
		if userSelected == nil {
			userSelected = []string{}
		}

		correctOptions := getCorrectOptionsFromQD(row.Type, qd)
		isCorrect := gradeQuestionFromQD(row.Type, qd, userSelected)

		status := "incorrect"
		if isCorrect {
			status = "correct"
			correctCount++
		}

		results = append(results, QuestionResult{
			QuestionID:     row.ID,
			Status:         status,
			UserSelected:   userSelected,
			CorrectOptions: correctOptions,
		})
	}

	totalQuestions := len(questionRows)
	percentage := 0.0
	if totalQuestions > 0 {
		percentage = float64(correctCount) / float64(totalQuestions) * 100
	}

	// Save quiz log
	lessonID, _ := s.quizRepo.GetLessonIDByVideoID(ctx, vid)
	if lessonID > 0 {
		snapshot, _ := json.Marshal(req.Answers)
		_ = s.quizRepo.SaveQuizLog(ctx, uuid.Nil, lessonID, correctCount, totalQuestions, snapshot)
	}

	return &QuizGradeResponse{
		Summary: GradeSummary{
			TotalQuestions: totalQuestions,
			CorrectCount:   correctCount,
			TotalScore:     correctCount,
			Percentage:     percentage,
		},
		Results: results,
	}, nil
}

// gradeQuestionFromQD grades a question based on its DB type and parsed question_data.
func gradeQuestionFromQD(dbType string, qd repository.QuestionData, userSelected []string) bool {
	switch dbType {
	case "single_choice":
		return gradeSingleChoiceQD(qd, userSelected)
	case "multiple_response", "multiple_choice":
		return gradeMultipleResponseQD(qd, userSelected)
	case "ordering":
		return gradeOrderingQD(qd, userSelected)
	default:
		return false
	}
}

func gradeSingleChoiceQD(qd repository.QuestionData, userSelected []string) bool {
	if len(userSelected) != 1 {
		return false
	}
	for _, opt := range qd.Options {
		if opt.IsCorrect && opt.ID == userSelected[0] {
			return true
		}
	}
	return false
}

func gradeMultipleResponseQD(qd repository.QuestionData, userSelected []string) bool {
	correctSet := make(map[string]bool)
	for _, opt := range qd.Options {
		if opt.IsCorrect {
			correctSet[opt.ID] = true
		}
	}
	if len(userSelected) != len(correctSet) {
		return false
	}
	for _, id := range userSelected {
		if !correctSet[id] {
			return false
		}
	}
	return true
}

func gradeOrderingQD(qd repository.QuestionData, userSelected []string) bool {
	if len(userSelected) != len(qd.CorrectOrder) {
		return false
	}
	for i := range userSelected {
		if userSelected[i] != qd.CorrectOrder[i] {
			return false
		}
	}
	return true
}

func getCorrectOptionsFromQD(dbType string, qd repository.QuestionData) []string {
	if dbType == "ordering" {
		return qd.CorrectOrder
	}
	var correct []string
	for _, opt := range qd.Options {
		if opt.IsCorrect {
			correct = append(correct, opt.ID)
		}
	}
	sort.Strings(correct)
	return correct
}
