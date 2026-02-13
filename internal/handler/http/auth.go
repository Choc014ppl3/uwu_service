package http

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	log         zerolog.Logger
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(log zerolog.Logger, authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		log:         log,
		authService: authService,
	}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		response.BadRequest(w, "email and password are required")
		return
	}

	if len(req.Password) < 6 {
		response.BadRequest(w, "password must be at least 6 characters")
		return
	}

	result, err := h.authService.Register(r.Context(), req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.Created(w, result)
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		response.BadRequest(w, "email and password are required")
		return
	}

	result, err := h.authService.Login(r.Context(), req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *AuthHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.InternalError(w, "internal server error")
}
