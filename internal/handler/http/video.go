package http

import (
	"net/http"
	"strings"

	"encoding/json"
	"strconv"

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

// Allowed image MIME types for thumbnails.
var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
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

// UploadNativeVideo handles POST /api/v1/native-videos/upload
func (h *VideoHandler) UploadNativeVideo(w http.ResponseWriter, r *http.Request) {
	// Get user ID from auth context
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	// Limit request body to 30MB
	const maxUploadSize = 30 << 20 // 30MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		response.BadRequest(w, "file too large, maximum size is 30MB")
		return
	}

	// Get file from form
	vFile, vHeader, err := r.FormFile("video")
	if err != nil {
		response.BadRequest(w, "video file is required (form field: 'video')")
		return
	}
	defer vFile.Close()

	// Validate MIME type
	vContentType := vHeader.Header.Get("Content-Type")
	if vContentType == "" {
		if strings.HasSuffix(strings.ToLower(vHeader.Filename), ".mp4") {
			vContentType = "video/mp4"
		} else if strings.HasSuffix(strings.ToLower(vHeader.Filename), ".mov") {
			vContentType = "video/quicktime"
		}
	}

	if !allowedVideoMIME[vContentType] {
		response.BadRequest(w, "invalid file type, allowed: mp4, mov, avi, webm")
		return
	}

	// Get thumbnail (required)
	tFile, tHeader, tErr := r.FormFile("thumbnail")
	if tErr != nil {
		response.BadRequest(w, "thumbnail file is required (form field: 'thumbnail')")
		return
	}
	defer tFile.Close()

	tContentType := tHeader.Header.Get("Content-Type")
	if tContentType == "" {
		if strings.HasSuffix(strings.ToLower(tHeader.Filename), ".jpg") || strings.HasSuffix(strings.ToLower(tHeader.Filename), ".jpeg") {
			tContentType = "image/jpeg"
		} else if strings.HasSuffix(strings.ToLower(tHeader.Filename), ".png") {
			tContentType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(tHeader.Filename), ".webp") {
			tContentType = "image/webp"
		}
	}

	if !allowedImageMIME[tContentType] {
		response.BadRequest(w, "invalid thumbnail type, allowed: jpeg, png, webp")
		return
	}

	// Get language from headers (optional but recommended)
	language := r.Header.Get("Language")

	// Process upload
	result, err := h.videoService.ProcessUpload(r.Context(), userID, vFile, vContentType, tFile, tContentType, language)
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

// GetVideoPlaylist handles GET /api/v1/videos/playlist
func (h *VideoHandler) GetVideoPlaylist(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "user not authenticated")
		return
	}

	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")
	status := r.URL.Query().Get("status")

	page := 1
	limit := 20

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	items, total, err := h.videoService.GetVideoPlaylist(r.Context(), userID, status, limit, offset)
	if err != nil {
		h.handleError(w, err)
		return
	}

	responsePayload := map[string]interface{}{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responsePayload)
}

// GetBatchStatus handles GET /api/v1/batches/{batchID}
func (h *VideoHandler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchID")
	if batchID == "" {
		response.BadRequest(w, "batch ID is required")
		return
	}

	batch, err := h.batchService.GetBatchWithJobs(r.Context(), batchID)
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

// GetUploadProgress handles GET /api/v1/native-videos/upload/{batchID}
// Returns the batch progress for a native video upload.
func (h *VideoHandler) GetUploadProgress(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchID")
	if batchID == "" {
		response.BadRequest(w, "batch ID is required")
		return
	}

	batch, err := h.batchService.GetBatchWithJobs(r.Context(), batchID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if batch != nil {
		// If batch is completed, fetch the full video item from DB
		if batch.Status == "completed" {
			video, dbErr := h.videoService.GetVideoByBatchID(r.Context(), batchID)
			if dbErr == nil && video != nil {
				responsePayload := map[string]interface{}{
					"batch": batch,
					"video": video,
				}
				response.JSON(w, http.StatusOK, responsePayload)
				return
			}
		}

		response.JSON(w, http.StatusOK, batch)
		return
	}

	// 2. If Redis expired or missing, fallback to check DB directly
	video, dbErr := h.videoService.GetVideoByBatchID(r.Context(), batchID)
	if dbErr == nil && video != nil {
		responsePayload := map[string]interface{}{
			"batch": map[string]string{"id": batchID, "status": "completed"},
			"video": video,
		}
		response.JSON(w, http.StatusOK, responsePayload)
		return
	}

	response.NotFound(w, "batch not found")
}

func (h *VideoHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.InternalError(w, "internal server error")
}
