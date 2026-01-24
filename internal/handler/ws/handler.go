package ws

import (
	"encoding/json"

	"github.com/rs/zerolog"
)

// MessageType constants
const (
	TypePing    = "ping"
	TypePong    = "pong"
	TypeChat    = "chat"
	TypeError   = "error"
	TypeSuccess = "success"
)

// Handler handles WebSocket messages.
type Handler struct {
	log zerolog.Logger
}

// NewHandler creates a new WebSocket handler.
func NewHandler(log zerolog.Logger) *Handler {
	return &Handler{log: log}
}

// Response represents a WebSocket response.
type Response struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Handle processes incoming WebSocket messages.
func (h *Handler) Handle(clientID string, msgType string, payload json.RawMessage) ([]byte, error) {
	h.log.Debug().
		Str("client_id", clientID).
		Str("type", msgType).
		Msg("Handling WebSocket message")

	switch msgType {
	case TypePing:
		return h.handlePing()

	case TypeChat:
		return h.handleChat(clientID, payload)

	default:
		return h.errorResponse("unknown message type: " + msgType)
	}
}

func (h *Handler) handlePing() ([]byte, error) {
	return h.response(TypePong, map[string]string{
		"message": "pong",
	})
}

// ChatPayload represents a chat message payload.
type ChatPayload struct {
	Message string `json:"message"`
}

func (h *Handler) handleChat(clientID string, payload json.RawMessage) ([]byte, error) {
	var chat ChatPayload
	if err := json.Unmarshal(payload, &chat); err != nil {
		return h.errorResponse("invalid chat payload")
	}

	h.log.Info().
		Str("client_id", clientID).
		Str("message", chat.Message).
		Msg("Received chat message")

	// Echo back for now - integrate with AI service as needed
	return h.response(TypeChat, map[string]interface{}{
		"from":    "server",
		"message": "Received: " + chat.Message,
	})
}

func (h *Handler) response(msgType string, payload interface{}) ([]byte, error) {
	resp := Response{
		Type:    msgType,
		Payload: payload,
	}
	return json.Marshal(resp)
}

func (h *Handler) errorResponse(message string) ([]byte, error) {
	return h.response(TypeError, map[string]string{
		"error": message,
	})
}
