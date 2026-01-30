package http

import (
	"encoding/json"
	"net/http"

	"github.com/windfall/uwu_service/internal/service"
)

type LearningItemHandler struct {
	service *service.LearningService
}

func NewLearningItemHandler(service *service.LearningService) *LearningItemHandler {
	return &LearningItemHandler{service: service}
}

func (h *LearningItemHandler) Create(w http.ResponseWriter, r *http.Request) {
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
