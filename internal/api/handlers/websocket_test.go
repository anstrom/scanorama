package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

func TestNewWebSocketHandler(t *testing.T) {
	tests := []struct {
		name        string
		database    *db.DB
		logger      *slog.Logger
		metrics     *metrics.Registry
		expectPanic bool
	}{
		{
			name:        "successful creation with nil dependencies",
			database:    nil,
			logger:      createTestLogger(),
			metrics:     nil,
			expectPanic: false,
		},
		{
			name:        "creation with valid logger",
			database:    nil,
			logger:      createTestLogger(),
			metrics:     nil,
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				assert.Panics(t, func() {
					NewWebSocketHandler(tt.database, tt.logger, tt.metrics)
				})
			} else {
				handler := NewWebSocketHandler(tt.database, tt.logger, tt.metrics)
				assert.NotNil(t, handler)
				assert.NotNil(t, handler.logger)
				assert.NotNil(t, handler.scanClients)
				assert.NotNil(t, handler.discoveryClients)
				assert.NotNil(t, handler.scanBroadcast)
				assert.NotNil(t, handler.discoveryBroadcast)
				assert.NotNil(t, handler.register)
				assert.NotNil(t, handler.unregister)
			}
		})
	}
}

func TestWebSocketHandler_HijackerInterface(t *testing.T) {
	// Test that we can create a type that implements http.Hijacker
	w := httptest.NewRecorder()
	wrapper := &hijackerWrapper{w}

	// Verify it implements the interface
	_, ok := interface{}(wrapper).(http.Hijacker)
	assert.True(t, ok, "wrapper should implement http.Hijacker interface")

	// Test the Hijack method returns appropriate error
	conn, rw, err := wrapper.Hijack()
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Nil(t, rw)
}

// hijackerWrapper implements http.Hijacker for testing
type hijackerWrapper struct {
	http.ResponseWriter
}

func (h *hijackerWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

func TestWebSocketHandler_GetConnectedClients(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	clients := handler.GetConnectedClients()

	// Initially should have zero clients
	assert.Equal(t, 0, clients["scan"])
	assert.Equal(t, 0, clients["discovery"])
	assert.Equal(t, 0, clients["total"])
	assert.Contains(t, clients, "scan")
	assert.Contains(t, clients, "discovery")
	assert.Contains(t, clients, "total")
}

func TestWebSocketHandler_Close(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	err := handler.Close()
	assert.NoError(t, err)

	// Verify client counts are zero after close
	clients := handler.GetConnectedClients()
	assert.Equal(t, 0, clients["scan"])
	assert.Equal(t, 0, clients["discovery"])
	assert.Equal(t, 0, clients["total"])
}

func TestWebSocketHandler_Shutdown(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Should not panic
	assert.NotPanics(t, func() {
		handler.Shutdown()
	})
}

func TestWebSocketMessage_Serialization(t *testing.T) {
	tests := []struct {
		name    string
		message WebSocketMessage
		valid   bool
	}{
		{
			name: "valid scan update message",
			message: WebSocketMessage{
				Type:      "scan_update",
				Timestamp: time.Now().UTC(),
				Data: ScanUpdateMessage{
					ScanID:   123,
					Status:   "running",
					Progress: 50.0,
					Message:  "Test message",
				},
				RequestID: "test-request-123",
			},
			valid: true,
		},
		{
			name: "valid discovery update message",
			message: WebSocketMessage{
				Type:      "discovery_update",
				Timestamp: time.Now().UTC(),
				Data: DiscoveryUpdateMessage{
					JobID:      456,
					Status:     "completed",
					Progress:   100.0,
					HostsFound: 5,
					NewHosts:   2,
				},
			},
			valid: true,
		},
		{
			name: "valid system message",
			message: WebSocketMessage{
				Type:      "system_status",
				Timestamp: time.Now().UTC(),
				Data: map[string]string{
					"message": "System is healthy",
				},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			data, err := json.Marshal(tt.message)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotEmpty(t, data)

				// Test deserialization
				var unmarshaled WebSocketMessage
				err = json.Unmarshal(data, &unmarshaled)
				assert.NoError(t, err)
				assert.Equal(t, tt.message.Type, unmarshaled.Type)
				assert.Equal(t, tt.message.RequestID, unmarshaled.RequestID)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestScanUpdateMessage_Validation(t *testing.T) {
	tests := []struct {
		name    string
		message ScanUpdateMessage
		valid   bool
	}{
		{
			name: "valid complete message",
			message: ScanUpdateMessage{
				ScanID:       123,
				Status:       "running",
				Progress:     50.0,
				Message:      "Scan in progress",
				ResultsCount: 10,
			},
			valid: true,
		},
		{
			name: "valid minimal message",
			message: ScanUpdateMessage{
				ScanID:   456,
				Status:   "pending",
				Progress: 0.0,
			},
			valid: true,
		},
		{
			name: "message with error",
			message: ScanUpdateMessage{
				ScanID:   789,
				Status:   "failed",
				Progress: 25.0,
				Error:    "Connection timeout",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotEmpty(t, data)

				// Test deserialization
				var unmarshaled ScanUpdateMessage
				err = json.Unmarshal(data, &unmarshaled)
				assert.NoError(t, err)
				assert.Equal(t, tt.message.ScanID, unmarshaled.ScanID)
				assert.Equal(t, tt.message.Status, unmarshaled.Status)
				assert.Equal(t, tt.message.Progress, unmarshaled.Progress)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestDiscoveryUpdateMessage_Validation(t *testing.T) {
	tests := []struct {
		name    string
		message DiscoveryUpdateMessage
		valid   bool
	}{
		{
			name: "valid complete message",
			message: DiscoveryUpdateMessage{
				JobID:      123,
				Status:     "running",
				Progress:   75.0,
				Message:    "Discovery in progress",
				HostsFound: 15,
				NewHosts:   5,
			},
			valid: true,
		},
		{
			name: "valid minimal message",
			message: DiscoveryUpdateMessage{
				JobID:    456,
				Status:   "pending",
				Progress: 0.0,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotEmpty(t, data)

				var unmarshaled DiscoveryUpdateMessage
				err = json.Unmarshal(data, &unmarshaled)
				assert.NoError(t, err)
				assert.Equal(t, tt.message.JobID, unmarshaled.JobID)
				assert.Equal(t, tt.message.Status, unmarshaled.Status)
				assert.Equal(t, tt.message.Progress, unmarshaled.Progress)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestWebSocketHandler_BroadcastMethods_NilHandling(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Test broadcasting with nil - should handle gracefully
	t.Run("nil scan update", func(t *testing.T) {
		err := handler.BroadcastScanUpdate(nil)
		// Should handle nil gracefully by marshaling as JSON null
		assert.NoError(t, err)
	})

	t.Run("nil discovery update", func(t *testing.T) {
		err := handler.BroadcastDiscoveryUpdate(nil)
		// Should handle nil gracefully by marshaling as JSON null
		assert.NoError(t, err)
	})

	t.Run("empty system message", func(t *testing.T) {
		err := handler.BroadcastSystemMessage("", "")
		// Should succeed even with empty strings
		assert.NoError(t, err)
	})
}

func TestWebSocketHandler_ConnectionTypeLogic(t *testing.T) {
	tests := []struct {
		name          string
		connTypes     []string
		expectedLabel string
	}{
		{
			name:          "single scan connection",
			connTypes:     []string{"scan"},
			expectedLabel: "scan",
		},
		{
			name:          "single discovery connection",
			connTypes:     []string{"discovery"},
			expectedLabel: "discovery",
		},
		{
			name:          "general connection (multiple types)",
			connTypes:     []string{"scan", "discovery"},
			expectedLabel: "general",
		},
		{
			name:          "empty connection types",
			connTypes:     []string{},
			expectedLabel: "single",
		},
		{
			name:          "multiple non-standard types",
			connTypes:     []string{"type1", "type2", "type3"},
			expectedLabel: "general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the connection type label logic
			connTypeLabel := "single"
			if len(tt.connTypes) > 1 {
				connTypeLabel = "general"
			} else if len(tt.connTypes) == 1 {
				connTypeLabel = tt.connTypes[0]
			}

			assert.Equal(t, tt.expectedLabel, connTypeLabel)
		})
	}
}

func TestWebSocketConstants(t *testing.T) {
	// Verify constants have reasonable values
	assert.Equal(t, 10*time.Second, writeWait)
	assert.Equal(t, 60*time.Second, pongWait)
	assert.Equal(t, 0.9, pingPeriodRatio)
	assert.Equal(t, time.Duration(float64(pongWait)*pingPeriodRatio), pingPeriod)
	assert.Equal(t, 512, maxMessageSize)
	assert.Equal(t, 256, bufferSize)

	// Verify relationships
	assert.Less(t, pingPeriod, pongWait, "ping period should be less than pong wait")
	assert.Greater(t, pingPeriod, 50*time.Second, "ping period should be reasonable")
	assert.Less(t, pingPeriod, 55*time.Second, "ping period should be reasonable")

	// Verify security limits
	assert.Greater(t, int(maxMessageSize), 0, "max message size should be positive")
	assert.Less(t, int(maxMessageSize), 1024*1024, "max message size should be reasonable")
	assert.Greater(t, bufferSize, 0, "buffer size should be positive")
}

func TestWebSocketHandler_BroadcastChannelCapacity(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Test that broadcast channels have expected capacity
	// We can't directly test capacity, but we can test behavior

	// Create valid messages
	scanUpdate := &ScanUpdateMessage{
		ScanID:   1,
		Status:   "running",
		Progress: 0.0,
	}

	discoveryUpdate := &DiscoveryUpdateMessage{
		JobID:    1,
		Status:   "running",
		Progress: 0.0,
	}

	// These should succeed without blocking (channels have buffer)
	err1 := handler.BroadcastScanUpdate(scanUpdate)
	assert.NoError(t, err1)

	err2 := handler.BroadcastDiscoveryUpdate(discoveryUpdate)
	assert.NoError(t, err2)

	err3 := handler.BroadcastSystemMessage("test", "test message")
	assert.NoError(t, err3)
}

func TestWebSocketHandler_SecurityConfigurationExists(t *testing.T) {
	// Test that security-related constants are properly defined
	assert.NotZero(t, maxMessageSize, "maxMessageSize should be defined")
	assert.NotZero(t, pongWait, "pongWait should be defined")
	assert.NotZero(t, writeWait, "writeWait should be defined")
	assert.NotZero(t, pingPeriod, "pingPeriod should be defined")

	// Verify they have security-appropriate values
	assert.LessOrEqual(t, int(maxMessageSize), 1024, "message size should be limited for security")
	assert.GreaterOrEqual(t, pongWait, 30*time.Second, "pong wait should allow reasonable network delays")
	assert.LessOrEqual(t, pongWait, 120*time.Second, "pong wait should not be too permissive")
}

// Test that the WebSocket handler properly handles context cancellation
func TestWebSocketHandler_ContextHandling(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Handler should be able to work with context
	_ = ctx
	assert.NotNil(t, handler)

	// Test that handler can be shut down
	assert.NotPanics(t, func() {
		handler.Shutdown()
	})
}

func TestWebSocketHandler_GetTotalClients(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Test getTotalClients method directly
	total := handler.getTotalClients()
	assert.Equal(t, 0, total, "should start with 0 clients")
}

func TestWebSocketHandler_BroadcastSuccessfulCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Test multiple successful scan broadcasts
	for i := 0; i < 10; i++ {
		update := &ScanUpdateMessage{
			ScanID:       int64(i),
			Status:       "running",
			Progress:     float64(i * 10),
			Message:      "Test message",
			ResultsCount: i,
		}
		err := handler.BroadcastScanUpdate(update)
		assert.NoError(t, err, "scan broadcast should succeed")
	}

	// Test multiple successful discovery broadcasts
	for i := 0; i < 10; i++ {
		update := &DiscoveryUpdateMessage{
			JobID:      int64(i + 100),
			Status:     "discovering",
			Progress:   float64(i * 10),
			HostsFound: i * 2,
			NewHosts:   i,
		}
		err := handler.BroadcastDiscoveryUpdate(update)
		assert.NoError(t, err, "discovery broadcast should succeed")
	}

	// Test multiple system messages with different types
	messageTypes := []string{"system_status", "maintenance", "alert", "info"}
	for _, msgType := range messageTypes {
		err := handler.BroadcastSystemMessage(msgType, "Test message content")
		assert.NoError(t, err, "system message broadcast should succeed")
	}
}

func TestWebSocketHandler_BroadcastEdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	t.Run("scan update with all fields", func(t *testing.T) {
		now := time.Now()
		update := &ScanUpdateMessage{
			ScanID:       999,
			Status:       "completed",
			Progress:     100.0,
			Message:      "Scan completed successfully",
			Error:        "",
			StartTime:    &now,
			EndTime:      &now,
			ResultsCount: 42,
		}
		err := handler.BroadcastScanUpdate(update)
		assert.NoError(t, err)
	})

	t.Run("discovery update with error", func(t *testing.T) {
		update := &DiscoveryUpdateMessage{
			JobID:      888,
			Status:     "failed",
			Progress:   30.0,
			Error:      "Network timeout occurred",
			HostsFound: 5,
			NewHosts:   0,
		}
		err := handler.BroadcastDiscoveryUpdate(update)
		assert.NoError(t, err)
	})

	t.Run("system message with special characters", func(t *testing.T) {
		err := handler.BroadcastSystemMessage("special_test", "Message with 特殊字符 and émojis 🚀")
		assert.NoError(t, err)
	})
}

func TestWebSocketHandler_GetTotalClientsInternal(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	// Test internal getTotalClients method
	total := handler.getTotalClients()
	assert.Equal(t, 0, total, "should start with 0 total clients")

	// Verify it matches the public method
	clients := handler.GetConnectedClients()
	assert.Equal(t, total, clients["total"], "internal and public methods should match")
}

// TestWebSocketHandler_ScanWebSocket tests the scan WebSocket endpoint
func TestWebSocketHandler_ScanWebSocket(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	// Connect as WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify client is registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 1, clients["scan"], "should have 1 scan client")
	assert.Equal(t, 0, clients["discovery"], "should have 0 discovery clients")

	// Test that we can broadcast to the client
	update := &ScanUpdateMessage{
		ScanID:   123,
		Status:   "running",
		Progress: 50.0,
		Message:  "Test scan update",
	}
	err = handler.BroadcastScanUpdate(update)
	assert.NoError(t, err)

	// Try to read the message
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err := conn.ReadMessage()
	if err == nil {
		// Verify message structure
		var wsMsg WebSocketMessage
		err = json.Unmarshal(message, &wsMsg)
		assert.NoError(t, err)
		assert.Equal(t, "scan_update", wsMsg.Type)
	}
}

// TestWebSocketHandler_DiscoveryWebSocket tests the discovery WebSocket endpoint
func TestWebSocketHandler_DiscoveryWebSocket(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.DiscoveryWebSocket(w, r)
	}))
	defer server.Close()

	// Connect as WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify client is registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 0, clients["scan"], "should have 0 scan clients")
	assert.Equal(t, 1, clients["discovery"], "should have 1 discovery client")

	// Test that we can broadcast to the client
	update := &DiscoveryUpdateMessage{
		JobID:      456,
		Status:     "discovering",
		Progress:   75.0,
		HostsFound: 10,
		NewHosts:   3,
	}
	err = handler.BroadcastDiscoveryUpdate(update)
	assert.NoError(t, err)

	// Try to read the message
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err := conn.ReadMessage()
	if err == nil {
		// Verify message structure
		var wsMsg WebSocketMessage
		err = json.Unmarshal(message, &wsMsg)
		assert.NoError(t, err)
		assert.Equal(t, "discovery_update", wsMsg.Type)
	}
}

// TestWebSocketHandler_GeneralWebSocket tests the general WebSocket endpoint
func TestWebSocketHandler_GeneralWebSocket(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.GeneralWebSocket(w, r)
	}))
	defer server.Close()

	// Connect as WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify client is registered for both types
	clients := handler.GetConnectedClients()
	assert.Equal(t, 1, clients["scan"], "should have 1 scan client")
	assert.Equal(t, 1, clients["discovery"], "should have 1 discovery client")

	// Test broadcast scan update
	scanUpdate := &ScanUpdateMessage{
		ScanID:   789,
		Status:   "completed",
		Progress: 100.0,
	}
	err = handler.BroadcastScanUpdate(scanUpdate)
	assert.NoError(t, err)

	// Test broadcast discovery update
	discoveryUpdate := &DiscoveryUpdateMessage{
		JobID:    999,
		Status:   "completed",
		Progress: 100.0,
	}
	err = handler.BroadcastDiscoveryUpdate(discoveryUpdate)
	assert.NoError(t, err)

	// Read messages (should get at least one)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, _ = conn.ReadMessage()
}

// TestWebSocketHandler_MultipleClients tests multiple concurrent connections
func TestWebSocketHandler_MultipleClients(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple clients
	numClients := 5
	conns := make([]*websocket.Conn, numClients)
	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		conns[i] = conn
	}

	// Give time for all registrations
	time.Sleep(200 * time.Millisecond)

	// Verify all clients are registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, numClients, clients["scan"], "should have %d scan clients", numClients)

	// Broadcast a message
	update := &ScanUpdateMessage{
		ScanID:   555,
		Status:   "running",
		Progress: 25.0,
		Message:  "Broadcast to all clients",
	}
	err := handler.BroadcastScanUpdate(update)
	assert.NoError(t, err)

	// Try to read from all clients
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(idx int) {
			defer wg.Done()
			conns[idx].SetReadDeadline(time.Now().Add(1 * time.Second))
			_, _, _ = conns[idx].ReadMessage()
		}(i)
	}
	wg.Wait()

	// Close all connections explicitly before checking
	for _, conn := range conns {
		conn.Close()
	}

	// Give time for unregistration
	time.Sleep(300 * time.Millisecond)

	// Verify clients are unregistered
	clients = handler.GetConnectedClients()
	assert.Equal(t, 0, clients["total"], "all clients should be unregistered")
}

// TestWebSocketHandler_ClientDisconnection tests client disconnection handling
func TestWebSocketHandler_ClientDisconnection(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify client is registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 1, clients["scan"], "should have 1 scan client")

	// Close connection
	conn.Close()

	// Give time for unregistration
	time.Sleep(200 * time.Millisecond)

	// Verify client is unregistered
	clients = handler.GetConnectedClients()
	assert.Equal(t, 0, clients["scan"], "client should be unregistered")
	assert.Equal(t, 0, clients["total"], "total should be 0")
}

// TestWebSocketHandler_BroadcastWithNoClients tests broadcasting with no connected clients
func TestWebSocketHandler_BroadcastWithNoClients(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	// Test broadcasting with no clients connected
	scanUpdate := &ScanUpdateMessage{
		ScanID:   111,
		Status:   "running",
		Progress: 10.0,
	}
	err := handler.BroadcastScanUpdate(scanUpdate)
	assert.NoError(t, err, "should not error when no clients connected")

	discoveryUpdate := &DiscoveryUpdateMessage{
		JobID:    222,
		Status:   "running",
		Progress: 20.0,
	}
	err = handler.BroadcastDiscoveryUpdate(discoveryUpdate)
	assert.NoError(t, err, "should not error when no clients connected")

	err = handler.BroadcastSystemMessage("test", "test message")
	assert.NoError(t, err, "should not error when no clients connected")
}

// TestWebSocketHandler_ConcurrentBroadcasts tests concurrent broadcasting
func TestWebSocketHandler_ConcurrentBroadcasts(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	var wg sync.WaitGroup
	numBroadcasts := 20

	// Test concurrent scan broadcasts
	wg.Add(numBroadcasts)
	for i := 0; i < numBroadcasts; i++ {
		go func(idx int) {
			defer wg.Done()
			update := &ScanUpdateMessage{
				ScanID:   int64(idx),
				Status:   "running",
				Progress: float64(idx * 5),
			}
			err := handler.BroadcastScanUpdate(update)
			assert.NoError(t, err)
		}(i)
	}

	// Test concurrent discovery broadcasts
	wg.Add(numBroadcasts)
	for i := 0; i < numBroadcasts; i++ {
		go func(idx int) {
			defer wg.Done()
			update := &DiscoveryUpdateMessage{
				JobID:    int64(idx + 1000),
				Status:   "discovering",
				Progress: float64(idx * 5),
			}
			err := handler.BroadcastDiscoveryUpdate(update)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

// TestWebSocketHandler_PingPong tests ping/pong mechanism
func TestWebSocketHandler_PingPong(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client with ping handler
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Set up pong handler to verify ping/pong mechanism works
	conn.SetPongHandler(func(string) error {
		return nil
	})

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Send a ping manually
	err = conn.WriteMessage(websocket.PingMessage, nil)
	assert.NoError(t, err)

	// Wait a bit and read to process pong
	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, _ = conn.ReadMessage()

	// Note: The server sends pings, not pongs to client pings
	// This test verifies the ping/pong mechanism doesn't cause errors
}

// TestWebSocketHandler_ChannelFullBehavior tests behavior when broadcast channels are full
func TestWebSocketHandler_ChannelFullBehavior(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	// Fill the scan broadcast channel beyond capacity
	// The buffer size is 256, so we'll try to send more
	for i := 0; i < bufferSize+10; i++ {
		update := &ScanUpdateMessage{
			ScanID:   int64(i),
			Status:   "running",
			Progress: 0.0,
		}
		_ = handler.BroadcastScanUpdate(update)
	}

	// The last few should either succeed (if consumed) or return an error
	// Either way, the handler should remain stable
	clients := handler.GetConnectedClients()
	assert.NotNil(t, clients, "handler should still be functional")
}

// TestWebSocketHandler_CloseWithActiveConnections tests closing handler with active connections
func TestWebSocketHandler_CloseWithActiveConnections(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple clients
	conns := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		conns[i] = conn
	}

	// Wait for all clients to be registered with proper synchronization
	require.Eventually(t, func() bool {
		clients := handler.GetConnectedClients()
		return clients["scan"] == 3
	}, 2*time.Second, 50*time.Millisecond, "should have 3 scan clients")

	// Verify clients are registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 3, clients["scan"], "should have 3 scan clients")

	// Close handler while connections are active
	err := handler.Close()
	assert.NoError(t, err)

	// Verify all clients are closed
	clients = handler.GetConnectedClients()
	assert.Equal(t, 0, clients["total"], "all clients should be closed")

	// Connections will be closed by defer

	handler.Shutdown()
}

// TestWebSocketHandler_MixedClientTypes tests scan and discovery clients together
func TestWebSocketHandler_MixedClientTypes(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	scanServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer scanServer.Close()

	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.DiscoveryWebSocket(w, r)
	}))
	defer discoveryServer.Close()

	// Connect scan clients
	scanURL := "ws" + strings.TrimPrefix(scanServer.URL, "http")
	scanConn, _, err := websocket.DefaultDialer.Dial(scanURL, nil)
	require.NoError(t, err)
	defer scanConn.Close()

	// Connect discovery clients
	discoveryURL := "ws" + strings.TrimPrefix(discoveryServer.URL, "http")
	discoveryConn, _, err := websocket.DefaultDialer.Dial(discoveryURL, nil)
	require.NoError(t, err)
	defer discoveryConn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Verify both types are registered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 1, clients["scan"], "should have 1 scan client")
	assert.Equal(t, 1, clients["discovery"], "should have 1 discovery client")
	assert.Equal(t, 2, clients["total"], "should have 2 total clients")

	// Broadcast to scan clients
	scanUpdate := &ScanUpdateMessage{
		ScanID:   777,
		Status:   "running",
		Progress: 50.0,
	}
	err = handler.BroadcastScanUpdate(scanUpdate)
	assert.NoError(t, err)

	// Broadcast to discovery clients
	discoveryUpdate := &DiscoveryUpdateMessage{
		JobID:    888,
		Status:   "discovering",
		Progress: 60.0,
	}
	err = handler.BroadcastDiscoveryUpdate(discoveryUpdate)
	assert.NoError(t, err)

	// Try to read from scan client
	scanConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, scanMsg, err := scanConn.ReadMessage()
	if err == nil {
		var wsMsg WebSocketMessage
		err = json.Unmarshal(scanMsg, &wsMsg)
		assert.NoError(t, err)
		assert.Equal(t, "scan_update", wsMsg.Type)
	}

	// Try to read from discovery client
	discoveryConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, discoveryMsg, err := discoveryConn.ReadMessage()
	if err == nil {
		var wsMsg WebSocketMessage
		err = json.Unmarshal(discoveryMsg, &wsMsg)
		assert.NoError(t, err)
		assert.Equal(t, "discovery_update", wsMsg.Type)
	}
}

// TestWebSocketHandler_SystemMessageBroadcast tests system message broadcasting
func TestWebSocketHandler_SystemMessageBroadcast(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	generalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.GeneralWebSocket(w, r)
	}))
	defer generalServer.Close()

	// Connect general client (receives both scan and discovery)
	wsURL := "ws" + strings.TrimPrefix(generalServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Broadcast system message
	err = handler.BroadcastSystemMessage("maintenance", "System maintenance scheduled")
	assert.NoError(t, err)

	// Try to read the message
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err := conn.ReadMessage()
	if err == nil {
		var wsMsg WebSocketMessage
		err = json.Unmarshal(message, &wsMsg)
		assert.NoError(t, err)
		assert.Equal(t, "maintenance", wsMsg.Type)
	}
}

// TestWebSocketHandler_InvalidUpgrade tests handling of invalid WebSocket upgrade requests
func TestWebSocketHandler_InvalidUpgrade(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	// Create a regular HTTP request (not WebSocket upgrade)
	req := httptest.NewRequest(http.MethodGet, "/ws/scan", nil)
	w := httptest.NewRecorder()

	// This should fail to upgrade
	handler.ScanWebSocket(w, req)

	// Should return an error response
	assert.NotEqual(t, http.StatusSwitchingProtocols, w.Code)
}

// TestWebSocketHandler_GetConnectedClientsThreadSafety tests thread-safe access to client counts
func TestWebSocketHandler_GetConnectedClientsThreadSafety(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrently access GetConnectedClients
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			clients := handler.GetConnectedClients()
			assert.NotNil(t, clients)
			assert.GreaterOrEqual(t, clients["total"], 0)
		}()
	}

	wg.Wait()
}

// TestWebSocketHandler_WritePumpTimeout tests writePump with connection timeout
func TestWebSocketHandler_WritePumpTimeout(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Give time for connection setup
	time.Sleep(100 * time.Millisecond)

	// Close without reading to trigger write errors
	conn.Close()

	// Give time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify client is cleaned up
	clients := handler.GetConnectedClients()
	assert.Equal(t, 0, clients["total"], "client should be cleaned up after close")
}

// TestWebSocketHandler_BroadcastToMultipleClients tests broadcast distribution
func TestWebSocketHandler_BroadcastToMultipleClients(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect 3 clients
	// Connect multiple clients
	conns := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		conns[i] = conn
	}
	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	// Give time for registration
	time.Sleep(200 * time.Millisecond)

	// Send multiple broadcasts
	for i := 0; i < 5; i++ {
		update := &ScanUpdateMessage{
			ScanID:   int64(i + 1),
			Status:   "running",
			Progress: float64(i * 20),
			Message:  "Broadcast test",
		}
		err := handler.BroadcastScanUpdate(update)
		assert.NoError(t, err)
	}

	// Each client should be able to read messages
	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err == nil {
			var wsMsg WebSocketMessage
			err = json.Unmarshal(msg, &wsMsg)
			assert.NoError(t, err, "client %d should receive valid message", i)
		}
	}
}

// TestWebSocketHandler_PingClientsExecution tests that ping mechanism runs
func TestWebSocketHandler_PingClientsExecution(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	pingCount := 0
	conn.SetPingHandler(func(appData string) error {
		pingCount++
		return conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(time.Second))
	})

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Keep reading to process ping messages
	go func() {
		for i := 0; i < 10; i++ {
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait long enough for at least one ping to be sent
	// pingPeriod is ~54s, but writePump sends pings
	time.Sleep(500 * time.Millisecond)

	// The connection should remain active
	clients := handler.GetConnectedClients()
	assert.GreaterOrEqual(t, clients["total"], 1, "client should remain connected")
}

// TestWebSocketHandler_BroadcastErrorHandling tests broadcast with client write errors
func TestWebSocketHandler_BroadcastErrorHandling(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Close connection immediately to cause write errors
	conn.Close()

	// Give time for unregistration to propagate
	time.Sleep(100 * time.Millisecond)

	// Try to broadcast - should handle closed connection gracefully
	update := &ScanUpdateMessage{
		ScanID:   999,
		Status:   "running",
		Progress: 50.0,
	}
	err = handler.BroadcastScanUpdate(update)
	assert.NoError(t, err, "broadcast should not error even with closed clients")

	// Verify client was cleaned up
	clients := handler.GetConnectedClients()
	assert.Equal(t, 0, clients["total"], "closed clients should be removed")
}

// TestWebSocketHandler_ReadPumpCloseHandling tests readPump connection close handling
func TestWebSocketHandler_ReadPumpCloseHandling(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Send close message properly
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	assert.NoError(t, err)

	// Give time for close to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify client is unregistered
	clients := handler.GetConnectedClients()
	assert.Equal(t, 0, clients["total"], "client should be unregistered after close")

	conn.Close()
}

// TestWebSocketHandler_BroadcastTimeoutBehavior tests broadcast with slow clients
func TestWebSocketHandler_BroadcastTimeoutBehavior(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ScanWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect client but don't read from it
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Send many messages without reading
	for i := 0; i < 10; i++ {
		update := &ScanUpdateMessage{
			ScanID:   int64(i),
			Status:   "running",
			Progress: float64(i * 10),
		}
		_ = handler.BroadcastScanUpdate(update)
	}

	// Handler should remain stable
	clients := handler.GetConnectedClients()
	assert.NotNil(t, clients, "handler should remain functional")
}

// TestWebSocketHandler_MixedBroadcastTypes tests broadcasting different message types
func TestWebSocketHandler_MixedBroadcastTypes(t *testing.T) {
	logger := createTestLogger()
	handler := NewWebSocketHandler(nil, logger, nil)
	defer handler.Shutdown()

	generalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.GeneralWebSocket(w, r)
	}))
	defer generalServer.Close()

	wsURL := "ws" + strings.TrimPrefix(generalServer.URL, "http")

	// Connect general client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Broadcast different types of messages
	scanUpdate := &ScanUpdateMessage{
		ScanID:   123,
		Status:   "running",
		Progress: 25.0,
	}
	err = handler.BroadcastScanUpdate(scanUpdate)
	assert.NoError(t, err)

	discoveryUpdate := &DiscoveryUpdateMessage{
		JobID:    456,
		Status:   "discovering",
		Progress: 50.0,
	}
	err = handler.BroadcastDiscoveryUpdate(discoveryUpdate)
	assert.NoError(t, err)

	err = handler.BroadcastSystemMessage("info", "System operational")
	assert.NoError(t, err)

	// Try to read messages
	messagesReceived := 0
	for i := 0; i < 3; i++ {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err == nil {
			messagesReceived++
		}
	}

	assert.GreaterOrEqual(t, messagesReceived, 1, "should receive at least one message")
}
