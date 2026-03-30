package auth

import (
	"log/slog"
	"net/http"

	"github.com/windfall/uwu_service/pkg/response"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	service *AuthService
	log     *slog.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(service *AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		service: service,
		log:     log,
	}
}

// -------------------------------------------------------------------------
// Register handles POST /api/v1/auth/register
// -------------------------------------------------------------------------

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest

	// 1. Parse & Validate
	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	// 2. เรียกใช้งาน Business Logic พร้อมสั่ง Map DTO จบในบรรทัดเดียว!
	result, err := h.service.Register(r.Context(), req.ToInput())
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to register user", slog.Any("error", err))
		response.HandleError(w, err)
		return
	}

	response.Created(w, result)
}

// -------------------------------------------------------------------------
// Login handles POST /api/v1/auth/login
// -------------------------------------------------------------------------

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := req.ParseAndValidate(r); err != nil {
		response.HandleError(w, err)
		return
	}

	// แมปข้อมูลและส่งเข้า Service ไปเลย
	result, err := h.service.Login(r.Context(), req.ToInput())
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to login user", slog.Any("error", err))
		response.HandleError(w, err)
		return
	}

	response.OK(w, result)
}
