// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements WebSocket endpoints for real-time updates on scan
// and discovery job progress, status changes, and results.
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	// WebSocket configuration constants.
	writeWait        = 10 * time.Second                                   // Time allowed to write a message to the peer
	pongWait         = 60 * time.Second                                   // Time to read next pong message from peer
	pingPeriodRatio  = 0.9                                                // Ratio of pongWait for pingPeriod
	pingPeriod       = time.Duration(float64(pongWait) * pingPeriodRatio) // Send pings to peer (must be < pongWait)
	maxMessageSize   = 512                                                // Maximum message size allowed from peer
	bufferSize       = 256                                                // Size of the broadcast channel buffer
	logsHistoryBurst = 100                                                // Recent log entries sent on connect

	loopbackHostname = "localhost" // hostname alias for loopback; IPs checked via net.IP.IsLoopback
)

// checkOrigin returns true if the WebSocket upgrade request should be accepted.
// It allows same-host origins and treats all loopback addresses as equivalent
// so that a Vite dev proxy (which rewrites Host but not Origin) works correctly.
func checkOrigin(origin, host string) bool {
	if origin == "" {
		return true // non-browser clients send no Origin
	}
	if origin == "http://"+host || origin == "https://"+host {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := originURL.Hostname()
	hostName, _, _ := net.SplitHostPort(host)
	if hostName == "" {
		hostName = host
	}
	isLoopback := func(h string) bool {
		if ip := net.ParseIP(h); ip != nil {
			return ip.IsLoopback()
		}
		return h == loopbackHostname
	}
	return isLoopback(originHost) && isLoopback(hostName)
}

// WebSocketHandler handles WebSocket connections for real-time updates.
type WebSocketHandler struct {
	logger   *slog.Logger
	metrics  *metrics.Registry
	upgrader websocket.Upgrader

	// Ring buffer for log streaming
	ringBuffer *logging.RingBuffer

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
	JobID        string  `json:"job_id"`
	Status       string  `json:"status"`
	Progress     float64 `json:"progress"`
	Message      string  `json:"message,omitempty"`
	Error        string  `json:"error,omitempty"`
	HostsFound   int     `json:"hosts_found,omitempty"`
	NewHosts     int     `json:"new_hosts,omitempty"`
	GoneHosts    int     `json:"gone_hosts,omitempty"`
	ChangedHosts int     `json:"changed_hosts,omitempty"`
}

// NewWebSocketHandler creates a new WebSocket handler.
func NewWebSocketHandler(logger *slog.Logger, metricsManager *metrics.Registry) *WebSocketHandler {
	handler := &WebSocketHandler{
		logger:  logger.With("handler", "websocket"),
		metrics: metricsManager,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return checkOrigin(r.Header.Get("Origin"), r.Host)
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

	conn, err := h.upgrader.Upgrade(w, r, nil) // nosemgrep
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Set up the connection for scan updates only
	h.setupConnection(conn, []string{"scan"}, requestID)
}

// DiscoveryWebSocket handles WebSocket connections for discovery updates.
func (h *WebSocketHandler) DiscoveryWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New discovery WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil) // nosemgrep
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Set up the connection for discovery updates only
	h.setupConnection(conn, []string{"discovery"}, requestID)
}

// GeneralWebSocket handles general WebSocket connections for all updates.
func (h *WebSocketHandler) GeneralWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New general WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil) // nosemgrep
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "request_id", requestID, "error", err)
		return
	}

	// Set up the connection for both scan and discovery updates
	h.setupConnection(conn, []string{"scan", "discovery"}, requestID)
}

// setupConnection configures a WebSocket connection for the specified connection types and starts read/write pumps.
func (h *WebSocketHandler) setupConnection(conn *websocket.Conn, connTypes []string, requestID string) {
	defer func() {
		h.unregister <- conn
		if err := conn.Close(); err != nil {
			h.logger.Error("Error closing WebSocket connection", "request_id", requestID, "error", err)
		}
	}()

	// Configure connection settings for security and resource management.
	// These settings are critical to prevent resource exhaustion and ensure proper cleanup:
	// - SetReadLimit prevents oversized messages from consuming excessive memory
	// - SetReadDeadline establishes initial timeout to prevent stalled connections
	// - SetPongHandler refreshes deadlines when pings are acknowledged, maintaining active connections
	// Without these, malicious or broken clients can linger indefinitely and consume resources.
	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		h.logger.Error("Failed to set read deadline", "request_id", requestID, "error", err)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Register client for all specified connection types
	for _, connType := range connTypes {
		h.register <- &clientRegistration{conn: conn, connType: connType}
	}

	// Determine connection type label for logging
	connTypeLabel := "single"
	if len(connTypes) > 1 {
		connTypeLabel = "general"
	} else if len(connTypes) == 1 {
		connTypeLabel = connTypes[0]
	}

	// Start write pump in goroutine
	go h.writePump(conn, connTypeLabel, requestID)

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
// Cleanup (unregister + close) is owned by the setupConnection defer, not here.
func (h *WebSocketHandler) readPump(conn *websocket.Conn, requestID string) {
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
// It must only be called from the run() goroutine.
// Dead connections are removed directly from the map (no unregister channel)
// to avoid a deadlock: run() cannot receive from h.unregister while it is
// blocked inside this function.
func (h *WebSocketHandler) broadcastToClients(clients map[*websocket.Conn]bool, message []byte, clientType string) {
	// Collect dead connections under a read lock so writes to the map happen separately.
	var dead []*websocket.Conn

	h.mutex.RLock()
	for conn := range clients {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			h.logger.Error("Failed to set write deadline", "error", err)
			dead = append(dead, conn)
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			h.logger.Debug("Write failed, closing connection", "client_type", clientType, "error", err)
			dead = append(dead, conn)
		}
	}
	h.mutex.RUnlock()

	if len(dead) > 0 {
		h.mutex.Lock()
		for _, conn := range dead {
			delete(h.scanClients, conn)
			delete(h.discoveryClients, conn)
			if err := conn.Close(); err != nil {
				h.logger.Debug("Error closing dead connection", "error", err)
			}
		}
		h.mutex.Unlock()
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("websocket_messages_sent_total", map[string]string{
			"type": clientType,
		})
	}
}

// pingClients sends ping messages to all connected clients.
// Dead connections are removed directly from the map (no unregister channel)
// to avoid a deadlock: run() cannot receive from h.unregister while it is
// blocked inside this function.
func (h *WebSocketHandler) pingClients() {
	var dead []*websocket.Conn

	h.mutex.RLock()
	for conn := range h.scanClients {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			h.logger.Error("Failed to set write deadline for ping", "error", err)
			dead = append(dead, conn)
			continue
		}
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			h.logger.Debug("Ping failed", "error", err)
			dead = append(dead, conn)
		}
	}
	for conn := range h.discoveryClients {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			h.logger.Error("Failed to set write deadline for ping", "error", err)
			dead = append(dead, conn)
			continue
		}
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			h.logger.Debug("Ping failed", "error", err)
			dead = append(dead, conn)
		}
	}
	h.mutex.RUnlock()

	if len(dead) > 0 {
		h.mutex.Lock()
		for _, conn := range dead {
			delete(h.scanClients, conn)
			delete(h.discoveryClients, conn)
			if err := conn.Close(); err != nil {
				h.logger.Debug("Error closing dead connection", "error", err)
			}
		}
		h.mutex.Unlock()
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

// WithRingBuffer sets the ring buffer used for log streaming and returns the
// handler for method chaining.
func (h *WebSocketHandler) WithRingBuffer(rb *logging.RingBuffer) *WebSocketHandler {
	h.ringBuffer = rb
	return h
}

// LogsWebSocket handles WebSocket connections at /api/v1/ws/logs.
//
// On connect it immediately sends a "log_history" burst of the 100 most recent
// log entries, then streams every future entry as individual "log_entry"
// messages until the client disconnects.
// sendLogsHistory sends the recent log history burst to the client on connect.
func (h *WebSocketHandler) sendLogsHistory(conn *websocket.Conn) {
	if h.ringBuffer == nil {
		return
	}
	recent := h.ringBuffer.Recent(logsHistoryBurst)
	msg := WebSocketMessage{
		Type:      "log_history",
		Timestamp: time.Now().UTC(),
		Data:      map[string]interface{}{"entries": recent, "total": len(recent)},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// runLogsWriteLoop runs the ping/write select loop until the client
// disconnects or the subscription channel closes.
func (h *WebSocketHandler) runLogsWriteLoop(
	conn *websocket.Conn,
	done <-chan struct{},
	subCh <-chan logging.LogEntry,
	requestID string,
) {
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-pingTicker.C:
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.logger.Debug("Logs WebSocket ping failed", "request_id", requestID, "error", err)
				return
			}
		case entry, ok := <-subCh:
			if !ok {
				return
			}
			msg := WebSocketMessage{
				Type:      "log_entry",
				Timestamp: time.Now().UTC(),
				Data:      entry,
			}
			data, marshalErr := json.Marshal(msg)
			if marshalErr != nil {
				continue
			}
			if writeErr := conn.SetWriteDeadline(time.Now().Add(writeWait)); writeErr != nil {
				return
			}
			if writeErr := conn.WriteMessage(websocket.TextMessage, data); writeErr != nil {
				h.logger.Debug("Logs WebSocket write failed", "request_id", requestID, "error", writeErr)
				return
			}
		}
	}
}

func (h *WebSocketHandler) LogsWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("New logs WebSocket connection", "request_id", requestID, "remote_addr", r.RemoteAddr)

	conn, err := h.upgrader.Upgrade(w, r, nil) // nosemgrep
	if err != nil {
		h.logger.Error("Failed to upgrade logs WebSocket connection", "request_id", requestID, "error", err)
		return
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			h.logger.Debug("Error closing logs WebSocket connection", "error", closeErr)
		}
	}()

	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		h.logger.Error("Failed to set read deadline", "request_id", requestID, "error", err)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	var sub chan logging.LogEntry
	if h.ringBuffer != nil {
		sub = h.ringBuffer.Subscribe()
		defer h.ringBuffer.Unsubscribe(sub)
	}

	h.sendLogsHistory(conn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, readErr := conn.ReadMessage(); readErr != nil {
				return
			}
		}
	}()

	var subCh <-chan logging.LogEntry
	if sub != nil {
		subCh = sub
	}

	h.runLogsWriteLoop(conn, done, subCh, requestID)
}

// Utility function shared with other handlers
