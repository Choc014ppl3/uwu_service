package dialog

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/internal/infra/middleware"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// DialogHandler handles dialog HTTP endpoints.
type DialogHandler struct {
	service *DialogService
	queue   *client.QueueClient
}

// NewDialogHandler creates a new DialogHandler.
func NewDialogHandler(service *DialogService, queue *client.QueueClient) *DialogHandler {
	return &DialogHandler{
		service: service,
		queue:   queue,
	}
}

// -------------------------------------------------------------------------
// ListDialogContents handles GET /api/v1/dialogs/contents
// -------------------------------------------------------------------------

func (h *DialogHandler) ListDialogContents(w http.ResponseWriter, r *http.Request) {
	// 1. parse pagination params
	var req ListDialogContentsRequest
	req.Parse(r)

	// 2. get dialog contents from database
	result, err := h.service.ListDialogContents(r.Context(), req.ToInput())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 3. response success
	response.OKWithMeta(w, result.Data, result.Meta)
}

// -------------------------------------------------------------------------
// GenerateDialog handles POST /api/v1/dialogs/generate
// -------------------------------------------------------------------------

func (h *DialogHandler) GenerateDialog(w http.ResponseWriter, r *http.Request) {
	// 1. parse and validate request
	var req GenerateDialogRequest
	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	// 2. generate payload once
	payload := req.ToPayload()

	// 3. send job to queue
	qErr := h.queue.Enqueue(client.Job{
		Type:    JOB_GENERATE_DIALOG,
		Payload: payload,
	})
	if qErr != nil {
		response.HandleError(w, qErr)
		return
	}

	// 4. create dialog record
	result, err := h.service.CreateDialogContent(r.Context(), payload)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 5. response accepted
	response.AcceptedWithMeta(w, result.Data, result.Meta)
}

// -------------------------------------------------------------------------
// GetDialogDetails handles GET /api/v1/dialogs/{dialogID}/details
// -------------------------------------------------------------------------

func (h *DialogHandler) GetDialogDetails(w http.ResponseWriter, r *http.Request) {
	dialogID := chi.URLParam(r, "dialogID")
	if dialogID == "" {
		response.HandleError(w, errors.Validation("Dialog ID is required"))
		return
	}

	// 2. get dialog details from service
	dialog, err := h.service.GetDialogDetails(r.Context(), dialogID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	// 3. response success
	response.OKWithMeta(w, dialog.Data, dialog.Meta)
}

// ToggleSaved handles POST /api/v1/dialogs/{dialogID}/toggle-saved
func (h *DialogHandler) ToggleSaved(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	dialogID := chi.URLParam(r, "dialogID")
	if dialogID == "" {
		response.HandleError(w, errors.Validation("Dialog ID is required"))
		return
	}

	result, err := h.service.ToggleSaved(r.Context(), dialogID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}

// StartSpeech handles POST /api/v1/dialogs/{dialogID}/start-speech
func (h *DialogHandler) StartSpeech(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	dialogID := chi.URLParam(r, "dialogID")
	if dialogID == "" {
		response.HandleError(w, errors.Validation("Dialog ID is required"))
		return
	}

	result, err := h.service.StartSpeech(r.Context(), dialogID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}

// SubmitSpeech handles POST /api/v1/dialogs/{dialogID}/submit-speech
func (h *DialogHandler) SubmitSpeech(w http.ResponseWriter, r *http.Request) {
	var req SubmitSpeechRequest
	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	result, err := h.service.SubmitSpeech(r.Context(), req.ToInput())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.Created(w, result)
}

// StartChat handles POST /api/v1/dialogs/{dialogID}/start-chat
func (h *DialogHandler) StartChat(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	dialogID := chi.URLParam(r, "dialogID")
	if dialogID == "" {
		response.HandleError(w, errors.Validation("Dialog ID is required"))
		return
	}

	result, err := h.service.StartChat(r.Context(), dialogID, userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}

// SubmitChat handles POST /api/v1/dialogs/{dialogID}/submit-chat
func (h *DialogHandler) SubmitChat(w http.ResponseWriter, r *http.Request) {
	var req SubmitChatRequest
	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	result, err := h.service.SubmitChat(r.Context(), req.ToInput())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}
