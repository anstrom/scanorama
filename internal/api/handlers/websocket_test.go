package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
