package video

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/internal/infra/middleware"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// VideoHandler handles video HTTP endpoints.
type VideoHandler struct {
	service *VideoService
	queue   *client.QueueClient
}

// NewVideoHandler creates a new VideoHandler.
func NewVideoHandler(service *VideoService, queue *client.QueueClient) *VideoHandler {
	return &VideoHandler{
		service: service,
		queue:   queue,
	}
}

// -------------------------------------------------------------------------
// ListVideoContents handles GET /api/v1/videos
// -------------------------------------------------------------------------

func (h *VideoHandler) ListVideoContents(w http.ResponseWriter, r *http.Request) {
	// 1. parse pagination params
	var req ListVideoContentsRequest
	req.Parse(r)

	// 2. get video contents from database
	result, err := h.service.ListVideoContents(r.Context(), req.ToInput())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 3. response success
	response.OKWithMeta(w, result.Data, result.Meta)
}

// -------------------------------------------------------------------------
// UploadVideo handles POST /api/v1/videos/upload
// -------------------------------------------------------------------------

func (h *VideoHandler) UploadVideo(w http.ResponseWriter, r *http.Request) {
	// 1. limit max upload size
	const maxUploadSize = 30 << 20 // 30MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	// 2. declare request struct and defer close
	var req UploadVideoRequest
	defer req.Close()

	// 3. parse and validate request
	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	// 4. generate payload once
	payload := req.ToPayload()

	// 5. send job to queue
	qErr := h.queue.Enqueue(client.Job{
		Type:    JOB_UPLOAD_VIDEO,
		Payload: payload,
	})
	if qErr != nil {
		response.HandleError(w, qErr)
		return
	}

	// 6. create video record
	result, err := h.service.CreateVideoContent(r.Context(), payload)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 6. response accepted
	response.AcceptedWithMeta(w, result.Data, result.Meta)
}

// -------------------------------------------------------------------------
// GetVideo handles GET /api/v1/videos/{videoID}/details
// -------------------------------------------------------------------------

func (h *VideoHandler) GetVideoDetails(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.HandleError(w, errors.Validation("Video ID is required"))
		return
	}

	// 2. get video from batch or database
	video, err := h.service.GetVideoDetails(r.Context(), videoID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 3. response success
	response.OKWithMeta(w, video.Data, video.Meta)
}

// ToggleSaved handles POST /api/v1/videos/{videoID}/toggle-saved
func (h *VideoHandler) ToggleSaved(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.HandleError(w, errors.Validation("Video ID is required"))
		return
	}

	result, err := h.service.ToggleSaved(r.Context(), videoID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}

// StartQuiz handles POST /api/v1/videos/{videoID}/start-quiz
func (h *VideoHandler) StartQuiz(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.HandleError(w, errors.Validation("Video ID is required"))
		return
	}

	result, err := h.service.StartQuiz(r.Context(), videoID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}

// ToggleTranscript handles POST /api/v1/videos/{videoID}/toggle-transcript
func (h *VideoHandler) ToggleTranscript(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		response.HandleError(w, errors.Validation("Video ID is required"))
		return
	}

	result, err := h.service.ToggleTranscript(r.Context(), videoID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}
