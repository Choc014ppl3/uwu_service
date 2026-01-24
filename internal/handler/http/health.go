package http

import (
	"encoding/json"
	"net/http"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	ready bool
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{ready: true}
}

// SetReady sets the ready state.
func (h *HealthHandler) SetReady(ready bool) {
	h.ready = ready
}

// Health checks if the service is healthy.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "uwu_service",
	})
}

// Ready checks if the service is ready to receive traffic.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_ready",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
	})
}

// Live checks if the service is alive (for Kubernetes liveness probe).
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "alive",
	})
}
