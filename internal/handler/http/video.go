package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/middleware"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// Allowed video MIME types.
var allowedVideoMIME = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
	"video/x-msvideo": true,
	"video/webm":      true,
}

// VideoHandler handles video upload HTTP endpoints.
type VideoHandler struct {
	log          zerolog.Logger
	videoService *service.VideoService
	batchService *service.BatchService
}

// NewVideoHandler creates a new VideoHandler.
func NewVideoHandler(log zerolog.Logger, videoService *service.VideoService, batchService *service.BatchService) *VideoHandler {
	return &VideoHandler{
		log:          log,
		videoService: videoService,
		batchService: batchService,
	}
}

// Upload handles POST /api/v1/videos/upload
func (h *VideoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 12MB
	const maxUploadSize = 12 << 20 // 12MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		response.BadRequest(w, "file too large, maximum size is 12MB")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("video")
	if err != nil {
		response.BadRequest(w, "video file is required (form field: 'video')")
		return
	}
	defer file.Close()

	// Validate MIME type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// Fallback: detect from filename extension
		if strings.HasSuffix(strings.ToLower(header.Filename), ".mp4") {
			contentType = "video/mp4"
		} else if strings.HasSuffix(strings.ToLower(header.Filename), ".mov") {
			contentType = "video/quicktime"
		}
	}

	if !allowedVideoMIME[contentType] {
		response.BadRequest(w, "invalid file type, allowed: mp4, mov, avi, webm")
		return
	}

	// Get user ID from auth context
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	// Process upload
	result, err := h.videoService.ProcessUpload(r.Context(), userID, file)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.Created(w, result)
}

// Get handles GET /api/v1/videos/{videoID}
func (h *VideoHandler) Get(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.BadRequest(w, "video ID is required")
		return
	}

	video, err := h.videoService.GetVideo(r.Context(), videoID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, video)
}

// GetBatchStatus handles GET /api/v1/batches/{batchID}
func (h *VideoHandler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchID")
	if batchID == "" {
		response.BadRequest(w, "batch ID is required")
		return
	}

	batch, err := h.batchService.GetBatch(r.Context(), batchID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if batch == nil {
		response.NotFound(w, "batch not found")
		return
	}

	response.JSON(w, http.StatusOK, batch)
}

func (h *VideoHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.InternalError(w, "internal server error")
}
