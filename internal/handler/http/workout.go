package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// WorkoutHandler handles workout generation endpoints.
type WorkoutHandler struct {
	log            zerolog.Logger
	workoutService *service.WorkoutService
	batchService   *service.BatchService
}

// NewWorkoutHandler creates a new WorkoutHandler.
func NewWorkoutHandler(log zerolog.Logger, workoutService *service.WorkoutService, batchService *service.BatchService) *WorkoutHandler {
	return &WorkoutHandler{
		log:            log,
		workoutService: workoutService,
		batchService:   batchService,
	}
}

// Generate handles POST /api/v1/workouts/generate
func (h *WorkoutHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req service.WorkoutGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.WorkoutTopic == "" {
		response.BadRequest(w, "workout_topic is required")
		return
	}
	if req.TargetLang == "" {
		response.BadRequest(w, "target_lang is required")
		return
	}

	result, err := h.workoutService.GenerateWorkout(r.Context(), req)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to generate workout")
		response.InternalError(w, "failed to generate workout")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GeneratePreBrief handles POST /api/v1/workouts/pre-brief
func (h *WorkoutHandler) GeneratePreBrief(w http.ResponseWriter, r *http.Request) {
	var req service.PreBriefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.WorkoutTopic == "" {
		response.BadRequest(w, "workout_topic is required")
		return
	}

	result, err := h.workoutService.GeneratePreBrief(r.Context(), req)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to generate pre-brief")
		response.InternalError(w, "failed to generate pre-brief")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GenerateConversation handles POST /api/v1/workouts/conversation
func (h *WorkoutHandler) GenerateConversation(w http.ResponseWriter, r *http.Request) {
	var req service.ConversationGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Topic == "" {
		response.BadRequest(w, "topic is required")
		return
	}
	if req.DescriptionType == "" {
		req.DescriptionType = "explanation" // default
	}
	if req.DescriptionType != "explanation" && req.DescriptionType != "transcription" {
		response.BadRequest(w, "description_type must be 'explanation' or 'transcription'")
		return
	}

	result, err := h.workoutService.GenerateConversation(r.Context(), req)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to generate conversation")
		response.InternalError(w, "failed to generate conversation")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GenerateLearningItems handles POST /api/v1/workouts/learning-items
func (h *WorkoutHandler) GenerateLearningItems(w http.ResponseWriter, r *http.Request) {
	var req service.LearningItemsGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.ScenarioID == "" {
		response.BadRequest(w, "scenario_id is required")
		return
	}

	result, err := h.workoutService.GenerateLearningItems(r.Context(), req)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to generate learning items")
		response.InternalError(w, "failed to generate learning items")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GetBatchStatus handles GET /api/v1/workouts/batches/{batchID}
func (h *WorkoutHandler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchID")
	if batchID == "" {
		response.BadRequest(w, "batch ID is required")
		return
	}

	batch, err := h.batchService.GetBatchWithJobs(r.Context(), batchID)
	if err != nil {
		h.log.Error().Err(err).Str("batch_id", batchID).Msg("Failed to get batch status")
		response.InternalError(w, "failed to get batch status")
		return
	}

	if batch == nil {
		// Batch expired from Redis — try scenarios first, then learning items
		scenarios, err := h.workoutService.GetScenariosByBatchID(r.Context(), batchID)
		if err == nil && scenarios != nil {
			response.JSON(w, http.StatusOK, scenarios)
			return
		}

		learningItems, err := h.workoutService.GetLearningItemsByBatchID(r.Context(), batchID)
		if err == nil && learningItems != nil {
			response.JSON(w, http.StatusOK, learningItems)
			return
		}

		response.JSON(w, http.StatusGone, map[string]string{
			"batch_id": batchID,
			"status":   "expired",
			"message":  "batch has been processed and cleaned up from memory",
		})
		return
	}

	response.JSON(w, http.StatusOK, batch)
}
