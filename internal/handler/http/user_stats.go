package http

import (
	"encoding/json"
	"net/http"

	"github.com/windfall/uwu_service/internal/middleware"
	"github.com/windfall/uwu_service/internal/service"
)

type UserStatsHandler struct {
	service *service.UserStatsService
}

func NewUserStatsHandler(service *service.UserStatsService) *UserStatsHandler {
	return &UserStatsHandler{service: service}
}

func (h *UserStatsHandler) GetLearningSummary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		http.Error(w, "language query parameter is required", http.StatusBadRequest)
		return
	}

	// Parse array of statuses
	r.ParseForm()
	statuses := r.Form["status"]

	summary, err := h.service.GetLearningSummary(r.Context(), userID, language, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
