package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/middleware"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// RetellHandler handles retell check HTTP endpoints.
type RetellHandler struct {
	log           zerolog.Logger
	retellService *service.RetellService
}

// NewRetellHandler creates a new RetellHandler.
func NewRetellHandler(log zerolog.Logger, retellService *service.RetellService) *RetellHandler {
	return &RetellHandler{
		log:           log,
		retellService: retellService,
	}
}

// SubmitAttempt handles POST /api/v1/quiz/{lessonID}/retell
// Accepts multipart form with "audio" file field.
func (h *RetellHandler) SubmitAttempt(w http.ResponseWriter, r *http.Request) {
	lessonID, err := strconv.Atoi(chi.URLParam(r, "lessonID"))
	if err != nil || lessonID <= 0 {
		response.BadRequest(w, "invalid lesson ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	// Limit request body to 20MB for audio
	const maxAudioSize = 20 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioSize)

	if err := r.ParseMultipartForm(maxAudioSize); err != nil {
		response.BadRequest(w, "file too large, maximum size is 20MB")
		return
	}

	file, _, err := r.FormFile("audio")
	if err != nil {
		response.BadRequest(w, "audio file is required (field: 'audio')")
		return
	}
	defer file.Close()

	result, err := h.retellService.SubmitAttempt(r.Context(), userID, lessonID, file)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GetStatus handles GET /api/v1/quiz/{lessonID}/retell
func (h *RetellHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	lessonID, err := strconv.Atoi(chi.URLParam(r, "lessonID"))
	if err != nil || lessonID <= 0 {
		response.BadRequest(w, "invalid lesson ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	result, err := h.retellService.GetSessionStatus(r.Context(), userID, lessonID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// Reset handles POST /api/v1/quiz/{lessonID}/retell/reset
func (h *RetellHandler) Reset(w http.ResponseWriter, r *http.Request) {
	lessonID, err := strconv.Atoi(chi.URLParam(r, "lessonID"))
	if err != nil || lessonID <= 0 {
		response.BadRequest(w, "invalid lesson ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	result, err := h.retellService.ResetSession(r.Context(), userID, lessonID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *RetellHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.InternalError(w, "internal server error")
}
