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
// GetVideo handles GET /api/v1/videos/{videoID}/details
// -------------------------------------------------------------------------

func (h *VideoHandler) GetVideoDetails(w http.ResponseWriter, r *http.Request) {
	// 1. Get user ID from auth context
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

	// 2. get video from batch or database
	video, err := h.service.GetVideoDetails(r.Context(), videoID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 3. response success
	response.OK(w, video)
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

	// 4. send job to queue
	qErr := h.queue.Enqueue(client.Job{
		Type:    JobUploadVideo,
		Payload: req.ToPayload(),
	})
	if qErr != nil {
		response.HandleError(w, qErr)
		return
	}

	// 5. create video record
	result, err := h.service.CreateVideoContent(r.Context(), req.ToPayload())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 6. response accepted
	response.Accepted(w, result)
}
