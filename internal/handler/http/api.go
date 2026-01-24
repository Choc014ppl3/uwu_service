package http

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// APIHandler handles REST API endpoints.
type APIHandler struct {
	log            zerolog.Logger
	aiService      *service.AIService
	exampleService *service.ExampleService
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(
	log zerolog.Logger,
	aiService *service.AIService,
	exampleService *service.ExampleService,
) *APIHandler {
	return &APIHandler{
		log:            log,
		aiService:      aiService,
		exampleService: exampleService,
	}
}

// GetExample handles GET /api/v1/example
func (h *APIHandler) GetExample(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := h.exampleService.GetExample(ctx, "example-id")
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// CreateExampleRequest represents the request body for creating an example.
type CreateExampleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateExample handles POST /api/v1/example
func (h *APIHandler) CreateExample(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateExampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Name == "" {
		h.handleError(w, errors.Validation("name is required"))
		return
	}

	result, err := h.exampleService.CreateExample(ctx, req.Name, req.Description)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, result)
}

// ChatRequest represents the request body for AI chat.
type ChatRequest struct {
	Message  string `json:"message"`
	Provider string `json:"provider"` // "openai" or "gemini"
}

// Chat handles POST /api/v1/ai/chat
func (h *APIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Message == "" {
		h.handleError(w, errors.Validation("message is required"))
		return
	}

	result, err := h.aiService.Chat(ctx, req.Message, req.Provider)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"response": result,
		"provider": req.Provider,
	})
}

// CompleteRequest represents the request body for AI completion.
type CompleteRequest struct {
	Prompt   string `json:"prompt"`
	Provider string `json:"provider"` // "openai" or "gemini"
}

// Complete handles POST /api/v1/ai/complete
func (h *APIHandler) Complete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Prompt == "" {
		h.handleError(w, errors.Validation("prompt is required"))
		return
	}

	result, err := h.aiService.Complete(ctx, req.Prompt, req.Provider)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"completion": result,
		"provider":   req.Provider,
	})
}

func (h *APIHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	response.Error(w, http.StatusInternalServerError, errors.Internal("internal server error"))
}
