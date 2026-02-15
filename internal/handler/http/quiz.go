package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// QuizHandler handles quiz grading HTTP endpoints.
type QuizHandler struct {
	log         zerolog.Logger
	quizService *service.QuizService
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(log zerolog.Logger, quizService *service.QuizService) *QuizHandler {
	return &QuizHandler{
		log:         log,
		quizService: quizService,
	}
}

// Grade handles POST /api/v1/videos/{videoID}/quiz/grade
func (h *QuizHandler) Grade(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.BadRequest(w, "video ID is required")
		return
	}

	var req service.QuizGradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if len(req.Answers) == 0 {
		response.BadRequest(w, "answers are required")
		return
	}

	result, err := h.quizService.GradeQuiz(r.Context(), videoID, req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *QuizHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.InternalError(w, "internal server error")
}
