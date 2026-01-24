package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	wshandler "github.com/windfall/uwu_service/internal/handler/ws"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Configure appropriately for production
	},
}

// WebSocketMessage represents a WebSocket message.
type WebSocketMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Client represents a WebSocket client.
type Client struct {
	ID   string
	Hub  *WebSocketHub
	Conn *websocket.Conn
	Send chan []byte
}

// WebSocketHub manages WebSocket connections.
type WebSocketHub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	log        zerolog.Logger
}

// NewWebSocketHub creates a new WebSocket hub.
func NewWebSocketHub(log zerolog.Logger) *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		log:        log,
	}
}

// Run starts the WebSocket hub.
func (h *WebSocketHub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.log.Info().Msg("WebSocket hub shutting down")
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.log.Info().Str("client_id", client.ID).Msg("Client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			h.log.Info().Str("client_id", client.ID).Msg("Client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// HandleWebSocket handles WebSocket upgrade and connection.
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request, handler *wshandler.Handler) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to upgrade connection")
		return
	}

	// Generate client ID
	clientID := r.Header.Get("X-Request-ID")
	if clientID == "" {
		clientID = generateClientID()
	}

	client := &Client{
		ID:   clientID,
		Hub:  h,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump(handler)
}

// Broadcast sends a message to all connected clients.
func (h *WebSocketHub) Broadcast(message []byte) {
	h.broadcast <- message
}

// ClientCount returns the number of connected clients.
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (c *Client) readPump(handler *wshandler.Handler) {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Hub.log.Error().Err(err).Msg("WebSocket read error")
			}
			break
		}

		// Parse message
		var msg WebSocketMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.Hub.log.Error().Err(err).Msg("Failed to parse WebSocket message")
			continue
		}

		// Handle message
		response, err := handler.Handle(c.ID, msg.Type, msg.Payload)
		if err != nil {
			c.Hub.log.Error().Err(err).Str("type", msg.Type).Msg("Failed to handle message")
			continue
		}

		if response != nil {
			c.Send <- response
		}
	}
}

func (c *Client) writePump() {
	defer c.Conn.Close()

	for message := range c.Send {
		w, err := c.Conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return
		}
		w.Write(message)
		w.Close()
	}
}

func generateClientID() string {
	// Simple ID generation - use UUID in production
	return "client-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
