package profile

import (
	"net/http"

	"github.com/windfall/uwu_service/internal/infra/middleware"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// ProfileHandler handles profile HTTP endpoints.
type ProfileHandler struct {
	service *ProfileService
}

// NewProfileHandler creates a new profile handler.
func NewProfileHandler(service *ProfileService) *ProfileHandler {
	return &ProfileHandler{
		service: service,
	}
}

// GetProfile handles GET /api/v1/profile.
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		response.HandleError(w, errors.Unauthorized("user not authenticated"))
		return
	}

	profile, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.OK(w, profile)
}
