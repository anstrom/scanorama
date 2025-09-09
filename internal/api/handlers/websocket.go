// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements WebSocket endpoints for real-time updates on scan
// and discovery job progress, status changes, and results.
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	// WebSocket configuration constants.
	writeWait       = 10 * time.Second                                   // Time allowed to write a message to the peer
	pongWait        = 60 * time.Second                                   // Time to read next pong message from peer
	pingPeriodRatio = 0.9                                                // Ratio of pongWait for pingPeriod
	pingPeriod      = time.Duration(float64(pongWait) * pingPeriodRatio) // Send pings to peer (must be < pongWait)
	maxMessageSize  = 512                                                // Maximum message size allowed from peer
	bufferSize      = 256                                                // Size of the broadcast channel buffer
)

// WebSocketHandler handles WebSocket connections for real-time updates.
type WebSocketHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
	upgrader websocket.Upgrader

	// Connection management
	scanClients        map[*websocket.Conn]bool
	discoveryClients   map[*websocket.Conn]bool
	scanBroadcast      chan []byte
	discoveryBroadcast chan []byte
	register           chan *clientRegistration
	unregister         chan *websocket.Conn
	shutdown           chan struct{}
	mutex              sync.RWMutex
}

// clientRegistration represents a new client registration.
type clientRegistration struct {
	conn     *websocket.Conn
	connType string // "scan" or "discovery"
}

// WebSocketMessage represents a WebSocket message structure.
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// ScanUpdateMessage represents a scan status update.
type ScanUpdateMessage struct {
	ScanID       int64      `json:"scan_id"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	Message      string     `json:"message,omitempty"`
	Error        string     `json:"error,omitempty"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	ResultsCount int        `json:"results_count,omitempty"`
}

// DiscoveryUpdateMessage represents a discovery job status update.
type DiscoveryUpdateMessage struct {
	JobID      int64   `json:"job_id"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
	Message    string  `json:"message,omitempty"`
	Error      string  `json:"error,omitempty"`
	HostsFound int     `json:"hosts_found,omitempty"`
	NewHosts   int     `json:"new_hosts,omitempty"`
}

// NewWebSocketHandler creates a new WebSocket handler.
func NewWebSocketHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *WebSocketHandler {
	handler := &WebSocketHandler{
		database: database,
		logger:   logger.With("handler", "websocket"),
		metrics:  metricsManager,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now
				return true
			},
		},
		scanClients:        make(map[*websocket.Conn]bool),
		discoveryClients:   make(map[*websocket.Conn]bool),
		scanBroadcast:      make(chan []byte, bufferSize),
		discoveryBroadcast: make(chan []byte, bufferSize),
		register:           make(chan *clientRegistration),
		unregister:         make(chan *websocket.Conn),
		shutdown:           make(chan struct{}),
	}

	// Start the hub goroutine
	go handler.run()

	return handler
}

// ScanWebSocket handles WebSocket connections for scan updates.
func (h *WebSocketHandler) ScanWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New scan WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Register the new client
	h.register <- &clientRegistration{
		conn:     conn,
		connType: "scan",
	}

	// Set up the connection
	h.setupConnection(conn, "scan", requestID)
}

// DiscoveryWebSocket handles WebSocket connections for discovery updates.
func (h *WebSocketHandler) DiscoveryWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New discovery WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Register the new client
	h.register <- &clientRegistration{
		conn:     conn,
		connType: "discovery",
	}

	// Set up the connection
	h.setupConnection(conn, "discovery", requestID)
}

// GeneralWebSocket handles general WebSocket connections for all updates.
func (h *WebSocketHandler) GeneralWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New general WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Register the new client for both scan and discovery updates
	h.setupGeneralConnection(conn, requestID)
}

// setupGeneralConnection configures a WebSocket connection for both scan and discovery updates.
func (h *WebSocketHandler) setupGeneralConnection(conn *websocket.Conn, requestID string) {
	defer func() {
		h.unregister <- conn
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing WebSocket connection", "request_id", requestID, "error", err)
		}
	}()

	// Register for both scan and discovery updates
	h.register <- &clientRegistration{conn: conn, connType: "scan"}
	h.register <- &clientRegistration{conn: conn, connType: "discovery"}

	// Start read and write pumps
	go h.writePump(conn, "general", requestID)
	h.readPump(conn, requestID)
}

// setupConnection configures a WebSocket connection and starts read/write pumps.
func (h *WebSocketHandler) setupConnection(conn *websocket.Conn, connType, requestID string) {
	defer func() {
		h.unregister <- conn
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing WebSocket connection", "request_id", requestID, "error", err)
		}
	}()

	// Configure connection settings
	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		h.logger.Error("Failed to set read deadline", "request_id", requestID, "error", err)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Start write pump in goroutine
	go h.writePump(conn, connType, requestID)

	// Start read pump (blocks until connection closes)
	h.readPump(conn, requestID)
}

// run manages client connections and broadcasts.
func (h *WebSocketHandler) run() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			h.logger.Debug("WebSocket handler shutting down")
			return
		case registration := <-h.register:
			h.mutex.Lock()
			switch registration.connType {
			case "scan":
				h.scanClients[registration.conn] = true
			case "discovery":
				h.discoveryClients[registration.conn] = true
			}
			h.mutex.Unlock()

			h.logger.Debug("Client registered", "type", registration.connType, "total_clients", h.getTotalClients())

		case conn := <-h.unregister:
			h.mutex.Lock()
			delete(h.scanClients, conn)
			delete(h.discoveryClients, conn)
			h.mutex.Unlock()

			h.logger.Debug("Client unregistered", "total_clients", h.getTotalClients())

		case message := <-h.scanBroadcast:
			h.broadcastToClients(h.scanClients, message, "scan")

		case message := <-h.discoveryBroadcast:
			h.broadcastToClients(h.discoveryClients, message, "discovery")

		case <-ticker.C:
			h.pingClients()
		}
	}
}

// Shutdown gracefully stops the WebSocket handler.
func (h *WebSocketHandler) Shutdown() {
	close(h.shutdown)
}

// readPump pumps messages from the WebSocket connection to the hub.
func (h *WebSocketHandler) readPump(conn *websocket.Conn, requestID string) {
	defer func() {
		h.unregister <- conn
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing connection in readPump", "request_id", requestID, "error", err)
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error("WebSocket unexpected close", "request_id", requestID, "error", err)
			}
			break
		}
		// For now, we don't process incoming messages from clients
		// In the future, this could handle subscription requests, filters, etc.
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (h *WebSocketHandler) writePump(conn *websocket.Conn, _, requestID string) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing connection in writePump", "request_id", requestID, "error", err)
		}
	}()

	for range ticker.C {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			h.logger.Error("Failed to set write deadline", "request_id", requestID, "error", err)
			return
		}
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			h.logger.Debug("Ping failed, closing connection", "request_id", requestID, "error", err)
			return
		}
	}
}

// broadcastToClients sends a message to all clients of a specific type.
func (h *WebSocketHandler) broadcastToClients(clients map[*websocket.Conn]bool, message []byte, clientType string) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for conn := range clients {
		select {
		case <-time.After(writeWait):
			h.logger.Warn("Write timeout, closing connection", "client_type", clientType)
			if err := conn.Close(); err != nil {
				h.logger.Error("Error closing timed out connection", "error", err)
			}
			delete(clients, conn)
		default:
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				h.logger.Error("Failed to set write deadline", "error", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				h.logger.Debug("Write failed, closing connection", "client_type", clientType, "error", err)
				if err := conn.Close(); err != nil {
					h.logger.Error("Error closing failed connection", "error", err)
				}
				delete(clients, conn)
			}
		}
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("websocket_messages_sent_total", map[string]string{
			"type": clientType,
		})
	}
}

// pingClients sends ping messages to all connected clients.
func (h *WebSocketHandler) pingClients() {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	allClients := make([]*websocket.Conn, 0, len(h.scanClients)+len(h.discoveryClients))
	for conn := range h.scanClients {
		allClients = append(allClients, conn)
	}
	for conn := range h.discoveryClients {
		allClients = append(allClients, conn)
	}

	for _, conn := range allClients {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			h.logger.Error("Failed to set write deadline for ping", "error", err)
			continue
		}
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			h.logger.Debug("Ping failed", "error", err)
			h.unregister <- conn
		}
	}
}

// getTotalClients returns the total number of connected clients.
func (h *WebSocketHandler) getTotalClients() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.scanClients) + len(h.discoveryClients)
}

// BroadcastScanUpdate sends a scan update to all connected scan clients.
func (h *WebSocketHandler) BroadcastScanUpdate(update *ScanUpdateMessage) error {
	message := WebSocketMessage{
		Type:      "scan_update",
		Timestamp: time.Now().UTC(),
		Data:      update,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal scan update: %w", err)
	}

	select {
	case h.scanBroadcast <- data:
		return nil
	default:
		h.logger.Warn("Scan broadcast channel full, dropping message")
		return fmt.Errorf("broadcast channel full")
	}
}

// BroadcastDiscoveryUpdate sends a discovery update to all connected discovery clients.
func (h *WebSocketHandler) BroadcastDiscoveryUpdate(update *DiscoveryUpdateMessage) error {
	message := WebSocketMessage{
		Type:      "discovery_update",
		Timestamp: time.Now().UTC(),
		Data:      update,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery update: %w", err)
	}

	select {
	case h.discoveryBroadcast <- data:
		return nil
	default:
		h.logger.Warn("Discovery broadcast channel full, dropping message")
		return fmt.Errorf("broadcast channel full")
	}
}

// BroadcastSystemMessage sends a system message to all connected clients.
func (h *WebSocketHandler) BroadcastSystemMessage(messageType, content string) error {
	message := WebSocketMessage{
		Type:      messageType,
		Timestamp: time.Now().UTC(),
		Data: map[string]string{
			"message": content,
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal system message: %w", err)
	}

	// Send to both scan and discovery clients
	select {
	case h.scanBroadcast <- data:
	default:
		h.logger.Warn("Scan broadcast channel full, dropping system message")
	}

	select {
	case h.discoveryBroadcast <- data:
	default:
		h.logger.Warn("Discovery broadcast channel full, dropping system message")
	}

	return nil
}

// GetConnectedClients returns the number of connected clients by type.
func (h *WebSocketHandler) GetConnectedClients() map[string]int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return map[string]int{
		"scan":      len(h.scanClients),
		"discovery": len(h.discoveryClients),
		"total":     len(h.scanClients) + len(h.discoveryClients),
	}
}

// Close gracefully shuts down the WebSocket handler.
func (h *WebSocketHandler) Close() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Close all scan client connections
	for conn := range h.scanClients {
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing scan client connection", "error", err)
		}
	}

	// Close all discovery client connections
	for conn := range h.discoveryClients {
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing discovery client connection", "error", err)
		}
	}

	// Clear client maps
	h.scanClients = make(map[*websocket.Conn]bool)
	h.discoveryClients = make(map[*websocket.Conn]bool)

	h.logger.Info("WebSocket handler closed")
	return nil
}

// Utility function shared with other handlers
