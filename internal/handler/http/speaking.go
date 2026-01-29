package http

import (
	"io"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// SpeakingHandler handles the 2-step async voice chat endpoints.
type SpeakingHandler struct {
	log             zerolog.Logger
	speakingService *service.SpeakingService
}

// NewSpeakingHandler creates a new Speaking handler.
func NewSpeakingHandler(log zerolog.Logger, speakingService *service.SpeakingService) *SpeakingHandler {
	return &SpeakingHandler{
		log:             log,
		speakingService: speakingService,
	}
}

// Analyze handles POST /api/v1/speaking/analyze
// This is the PRODUCER endpoint - accepts audio, returns transcript immediately,
// and spawns background AI processing.
//
// Request: multipart/form-data with "audio_file" field
// Response: { "request_id": "req_xxx", "transcript": "...", "score": 95.5 }
func (h *SpeakingHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form (10 MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.handleError(w, errors.Validation("failed to parse multipart form"))
		return
	}

	// Get audio file
	file, _, err := r.FormFile("audio_file")
	if err != nil {
		h.handleError(w, errors.Validation("audio_file is required"))
		return
	}
	defer file.Close()

	// Read file content
	audioData, err := io.ReadAll(file)
	if err != nil {
		h.handleError(w, errors.Validation("failed to read audio file"))
		return
	}

	// Call service - this returns immediately with transcript
	// while spawning background goroutine for AI processing
	result, err := h.speakingService.AnalyzeSpeaking(ctx, audioData)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GetReply handles GET /api/v1/speaking/reply
// This is the CONSUMER endpoint - uses BLPOP to wait for AI result.
//
// Query param: request_id
// Response (success): { "ai_text": "...", "audio_url": "..." }
// Response (timeout): 504 Gateway Timeout
func (h *SpeakingHandler) GetReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestID := r.URL.Query().Get("request_id")
	if requestID == "" {
		h.handleError(w, errors.Validation("request_id is required"))
		return
	}

	// Call service - this blocks until result is available or timeout
	result, err := h.speakingService.GetReply(ctx, requestID)
	if err != nil {
		// Check if it's a timeout error - return 504
		if appErr, ok := err.(*errors.AppError); ok && appErr.Code == errors.ErrTimeout {
			response.Error(w, http.StatusGatewayTimeout, appErr)
			return
		}
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *SpeakingHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	response.Error(w, http.StatusInternalServerError, errors.Internal("internal server error"))
}
