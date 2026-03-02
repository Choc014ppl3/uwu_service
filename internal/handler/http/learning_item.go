package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/middleware"
	"github.com/windfall/uwu_service/internal/service"
)

type LearningItemHandler struct {
	service *service.LearningService
}

func NewLearningItemHandler(service *service.LearningService) *LearningItemHandler {
	return &LearningItemHandler{service: service}
}

func (h *LearningItemHandler) CreateLearningItem(w http.ResponseWriter, r *http.Request) {
	var req service.CreateLearningItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	item, err := h.service.CreateLearningItem(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (h *LearningItemHandler) ListLearningItems(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

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

	items, total, err := h.service.ListLearningItems(r.Context(), page, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *LearningItemHandler) GetLearningItemsByFeature(w http.ResponseWriter, r *http.Request) {
	featureIDStr := r.URL.Query().Get("feature_id")
	if featureIDStr == "" {
		http.Error(w, "feature_id parameter is required", http.StatusBadRequest)
		return
	}

	featureID, err := strconv.Atoi(featureIDStr)
	if err != nil {
		http.Error(w, "invalid feature_id parameter", http.StatusBadRequest)
		return
	}

	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

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

	items, total, err := h.service.GetLearningItemsByFeature(r.Context(), featureID, page, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *LearningItemHandler) GetLearningItem(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	item, err := h.service.GetLearningItem(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (h *LearningItemHandler) UpdateLearningItem(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var req service.UpdateLearningItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	item, err := h.service.UpdateLearningItem(r.Context(), id, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (h *LearningItemHandler) DeleteLearningItem(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteLearningItem(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateActionRequest represents the request body for creating a learning item action.
type CreateActionRequest struct {
	LearningID string `json:"learning_id"`
	Type       string `json:"type"`
}

// CreateAction handles POST /api/v1/learning-items/actions
func (h *LearningItemHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	var req CreateActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.LearningID == "" {
		http.Error(w, "learning_id is required", http.StatusBadRequest)
		return
	}

	validTypes := map[string]bool{
		"quiz_passed":      true,
		"quiz_attempted":   true,
		"quiz_saved":       true,
		"dialogue_passed":  true,
		"dialogue_saved":   true,
		"chat_attempted":   true,
		"chat_passed":      true,
		"speech_attempted": true,
		"speech_passed":    true,
	}

	if !validTypes[req.Type] {
		http.Error(w, "invalid action type", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	err := h.service.CreateAction(r.Context(), userID, req.LearningID, req.Type)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
