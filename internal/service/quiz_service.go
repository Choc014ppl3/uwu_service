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
	videoRepo repository.VideoRepository
}

// NewQuizService creates a new QuizService.
func NewQuizService(videoRepo repository.VideoRepository) *QuizService {
	return &QuizService{videoRepo: videoRepo}
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

// GradeQuiz loads the quiz for a video and grades the user's answers.
func (s *QuizService) GradeQuiz(ctx context.Context, videoID string, req QuizGradeRequest) (*QuizGradeResponse, error) {
	// Parse video ID
	vid, err := uuid.Parse(videoID)
	if err != nil {
		return nil, errors.New(errors.ErrValidation, "invalid video ID")
	}

	// Fetch video record
	video, err := s.videoRepo.GetByID(ctx, vid)
	if err != nil {
		return nil, errors.New(errors.ErrNotFound, "video not found")
	}

	if video.QuizData == nil {
		return nil, errors.New(errors.ErrNotFound, "quiz not yet generated for this video")
	}

	// Parse quiz data
	var quizContent repository.QuizContent
	if err := json.Unmarshal(*video.QuizData, &quizContent); err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to parse quiz data")
	}

	if len(quizContent.Quiz) == 0 {
		return nil, errors.New(errors.ErrNotFound, "quiz has no questions")
	}

	// Build lookup map: question ID → QuizItem
	quizMap := make(map[int]repository.QuizItem, len(quizContent.Quiz))
	for _, q := range quizContent.Quiz {
		quizMap[q.ID] = q
	}

	// Build answer lookup: question ID → user answer
	answerMap := make(map[int][]string, len(req.Answers))
	for _, a := range req.Answers {
		answerMap[a.QuestionID] = a.SelectedOptions
	}

	// Grade each question in the quiz
	results := make([]QuestionResult, 0, len(quizContent.Quiz))
	correctCount := 0

	for _, q := range quizContent.Quiz {
		userSelected := answerMap[q.ID]
		if userSelected == nil {
			userSelected = []string{} // unanswered
		}

		correctOptions := getCorrectOptions(q)
		isCorrect := gradeQuestion(q, userSelected)

		status := "incorrect"
		explanation := ""
		if isCorrect {
			status = "correct"
			correctCount++
		}

		results = append(results, QuestionResult{
			QuestionID:     q.ID,
			Status:         status,
			UserSelected:   userSelected,
			CorrectOptions: correctOptions,
			Explanation:    explanation,
		})
	}

	totalQuestions := len(quizContent.Quiz)
	percentage := 0.0
	if totalQuestions > 0 {
		percentage = float64(correctCount) / float64(totalQuestions) * 100
	}

	return &QuizGradeResponse{
		Summary: GradeSummary{
			TotalQuestions: totalQuestions,
			CorrectCount:   correctCount,
			TotalScore:     correctCount, // 1 point per correct question
			Percentage:     percentage,
		},
		Results: results,
	}, nil
}

// gradeQuestion checks if the user's answer is correct for a given question type.
func gradeQuestion(q repository.QuizItem, userSelected []string) bool {
	switch q.Type {
	case "single_choice":
		return gradeSingleChoice(q, userSelected)
	case "multiple_response":
		return gradeMultipleResponse(q, userSelected)
	case "ordering":
		return gradeOrdering(q, userSelected)
	default:
		return false
	}
}

// gradeSingleChoice checks if the user selected exactly the one correct option.
func gradeSingleChoice(q repository.QuizItem, userSelected []string) bool {
	if len(userSelected) != 1 {
		return false
	}
	for _, opt := range q.Options {
		if opt.IsCorrect && opt.ID == userSelected[0] {
			return true
		}
	}
	return false
}

// gradeMultipleResponse uses STRICT MATCH: user must select ALL correct and NO incorrect.
func gradeMultipleResponse(q repository.QuizItem, userSelected []string) bool {
	correctSet := make(map[string]bool)
	for _, opt := range q.Options {
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

// gradeOrdering checks if the user's sequence exactly matches correct_order.
func gradeOrdering(q repository.QuizItem, userSelected []string) bool {
	if len(userSelected) != len(q.CorrectOrder) {
		return false
	}
	for i := range userSelected {
		if userSelected[i] != q.CorrectOrder[i] {
			return false
		}
	}
	return true
}

// getCorrectOptions returns the correct option IDs for a question.
func getCorrectOptions(q repository.QuizItem) []string {
	if q.Type == "ordering" {
		return q.CorrectOrder
	}
	var correct []string
	for _, opt := range q.Options {
		if opt.IsCorrect {
			correct = append(correct, opt.ID)
		}
	}
	sort.Strings(correct)
	return correct
}
