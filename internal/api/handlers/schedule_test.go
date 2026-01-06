package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/metrics"
)

func createTestScheduleHandler(t *testing.T) *ScheduleHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := metrics.NewRegistry()

	return NewScheduleHandler(nil, logger, registry)
}

func TestNewScheduleHandler(t *testing.T) {
	t.Run("initializes with dependencies", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		registry := metrics.NewRegistry()

		handler := NewScheduleHandler(nil, logger, registry)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.logger)
		assert.Equal(t, registry, handler.metrics)
	})
}

func TestScheduleHandler_validateBasicScheduleFields(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		request     *ScheduleRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid basic fields",
			request: &ScheduleRequest{
				Name:        "test-schedule",
				Description: "test description",
			},
			expectError: false,
		},
		{
			name: "empty name",
			request: &ScheduleRequest{
				Name:        "",
				Description: "test description",
			},
			expectError: true,
			errorMsg:    "schedule name is required",
		},
		{
			name: "name too long",
			request: &ScheduleRequest{
				Name:        string(make([]byte, maxScheduleNameLength+1)),
				Description: "test description",
			},
			expectError: true,
			errorMsg:    "schedule name too long",
		},
		{
			name: "description too long",
			request: &ScheduleRequest{
				Name:        "test-schedule",
				Description: string(make([]byte, maxScheduleDescLength+1)),
			},
			expectError: true,
			errorMsg:    "description too long",
		},
		{
			name: "maximum valid length fields",
			request: &ScheduleRequest{
				Name:        string(make([]byte, maxScheduleNameLength)),
				Description: string(make([]byte, maxScheduleDescLength)),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateBasicScheduleFields(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateScheduleCron(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		cronExpr    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid cron expression - every hour",
			cronExpr:    "0 * * * *",
			expectError: false,
		},
		{
			name:        "valid cron expression - every 6 hours",
			cronExpr:    "0 */6 * * *",
			expectError: false,
		},
		{
			name:        "valid cron expression - daily at midnight",
			cronExpr:    "0 0 * * *",
			expectError: false,
		},
		{
			name:        "valid cron expression - every minute",
			cronExpr:    "* * * * *",
			expectError: false,
		},
		{
			name:        "valid cron expression - weekdays at 9am",
			cronExpr:    "0 9 * * 1-5",
			expectError: false,
		},
		{
			name:        "empty cron expression",
			cronExpr:    "",
			expectError: true,
			errorMsg:    "cron expression is required",
		},
		{
			name:        "invalid cron expression - too few fields",
			cronExpr:    "0 * *",
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
		{
			name:        "invalid cron expression - invalid character",
			cronExpr:    "0 * * * x",
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
		{
			name:        "invalid cron expression - out of range",
			cronExpr:    "0 25 * * *",
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScheduleCron(tt.cronExpr)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateScheduleType(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name         string
		scheduleType string
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid type - scan",
			scheduleType: "scan",
			expectError:  false,
		},
		{
			name:         "valid type - discovery",
			scheduleType: "discovery",
			expectError:  false,
		},
		{
			name:         "invalid type - empty",
			scheduleType: "",
			expectError:  true,
			errorMsg:     "invalid schedule type",
		},
		{
			name:         "invalid type - unknown",
			scheduleType: "unknown",
			expectError:  true,
			errorMsg:     "invalid schedule type",
		},
		{
			name:         "invalid type - wrong case",
			scheduleType: "Scan",
			expectError:  true,
			errorMsg:     "invalid schedule type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScheduleType(tt.scheduleType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateScheduleOptions(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		request     *ScheduleRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid options",
			request: &ScheduleRequest{
				TargetID:     1,
				MaxRunTime:   30 * time.Minute,
				MaxRetries:   3,
				RetryDelay:   5 * time.Minute,
				NotifyEmails: []string{"test@example.com"},
			},
			expectError: false,
		},
		{
			name: "zero target ID",
			request: &ScheduleRequest{
				TargetID: 0,
			},
			expectError: true,
			errorMsg:    "target ID must be positive",
		},
		{
			name: "negative target ID",
			request: &ScheduleRequest{
				TargetID: -1,
			},
			expectError: true,
			errorMsg:    "target ID must be positive",
		},
		{
			name: "negative max run time",
			request: &ScheduleRequest{
				TargetID:   1,
				MaxRunTime: -1 * time.Second,
			},
			expectError: true,
			errorMsg:    "max run time cannot be negative",
		},
		{
			name: "max run time too long",
			request: &ScheduleRequest{
				TargetID:   1,
				MaxRunTime: 25 * time.Hour,
			},
			expectError: true,
			errorMsg:    "max run time too long",
		},
		{
			name: "negative max retries",
			request: &ScheduleRequest{
				TargetID:   1,
				MaxRetries: -1,
			},
			expectError: true,
			errorMsg:    "max retries cannot be negative",
		},
		{
			name: "max retries too high",
			request: &ScheduleRequest{
				TargetID:   1,
				MaxRetries: maxScheduleRetries + 1,
			},
			expectError: true,
			errorMsg:    "max retries too high",
		},
		{
			name: "negative retry delay",
			request: &ScheduleRequest{
				TargetID:   1,
				RetryDelay: -1 * time.Second,
			},
			expectError: true,
			errorMsg:    "retry delay cannot be negative",
		},
		{
			name: "retry delay too long",
			request: &ScheduleRequest{
				TargetID:   1,
				RetryDelay: 2 * time.Hour,
			},
			expectError: true,
			errorMsg:    "retry delay too long",
		},
		{
			name: "empty notification email",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{""},
			},
			expectError: true,
			errorMsg:    "notification email 1 is empty",
		},
		{
			name: "notification email too long",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{string(make([]byte, maxScheduleNameLength+1)) + "@example.com"},
			},
			expectError: true,
			errorMsg:    "notification email 1 too long",
		},
		{
			name: "invalid email format - no @",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{"invalid.email.com"},
			},
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name: "invalid email format - no dot",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{"invalid@email"},
			},
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name: "multiple valid emails",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{"user1@example.com", "user2@example.org"},
			},
			expectError: false,
		},
		{
			name: "second email invalid",
			request: &ScheduleRequest{
				TargetID:     1,
				NotifyEmails: []string{"valid@example.com", "invalid"},
			},
			expectError: true,
			errorMsg:    "notification email 2 has invalid format",
		},
		{
			name: "boundary values - max allowed",
			request: &ScheduleRequest{
				TargetID:   1,
				MaxRunTime: 24 * time.Hour,
				MaxRetries: maxScheduleRetries,
				RetryDelay: time.Hour,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScheduleOptions(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateScheduleTags(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		tags        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid tags",
			tags:        []string{"production", "critical"},
			expectError: false,
		},
		{
			name:        "empty tag list",
			tags:        []string{},
			expectError: false,
		},
		{
			name:        "nil tag list",
			tags:        nil,
			expectError: false,
		},
		{
			name:        "single valid tag",
			tags:        []string{"production"},
			expectError: false,
		},
		{
			name:        "empty tag in list",
			tags:        []string{"production", ""},
			expectError: true,
			errorMsg:    "tag 2 is empty",
		},
		{
			name:        "tag too long",
			tags:        []string{string(make([]byte, maxScheduleTagLength+1))},
			expectError: true,
			errorMsg:    "tag 1 too long",
		},
		{
			name:        "second tag too long",
			tags:        []string{"valid", string(make([]byte, maxScheduleTagLength+1))},
			expectError: true,
			errorMsg:    "tag 2 too long",
		},
		{
			name:        "maximum valid tag length",
			tags:        []string{string(make([]byte, maxScheduleTagLength))},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScheduleTags(tt.tags)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateCronExpression(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		cronExpr    string
		expectError bool
	}{
		{
			name:        "valid - every minute",
			cronExpr:    "* * * * *",
			expectError: false,
		},
		{
			name:        "valid - hourly",
			cronExpr:    "0 * * * *",
			expectError: false,
		},
		{
			name:        "valid - daily",
			cronExpr:    "0 0 * * *",
			expectError: false,
		},
		{
			name:        "valid - weekly",
			cronExpr:    "0 0 * * 0",
			expectError: false,
		},
		{
			name:        "valid - complex expression",
			cronExpr:    "15,45 9-17 * * 1-5",
			expectError: false,
		},
		{
			name:        "empty expression",
			cronExpr:    "",
			expectError: true,
		},
		{
			name:        "whitespace only",
			cronExpr:    "   ",
			expectError: true,
		},
		{
			name:        "too few fields",
			cronExpr:    "* * *",
			expectError: true,
		},
		{
			name:        "invalid field value",
			cronExpr:    "60 * * * *",
			expectError: true,
		},
		{
			name:        "invalid character",
			cronExpr:    "* * * * x",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateCronExpression(tt.cronExpr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_validateScheduleRequest(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name        string
		request     *ScheduleRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid complete request",
			request: &ScheduleRequest{
				Name:         "test-schedule",
				Description:  "test description",
				CronExpr:     "0 * * * *",
				Type:         "scan",
				TargetID:     1,
				Enabled:      true,
				MaxRunTime:   30 * time.Minute,
				RetryOnError: true,
				MaxRetries:   3,
				RetryDelay:   5 * time.Minute,
				Options:      map[string]string{"opt1": "val1"},
				Tags:         []string{"production"},
				NotifyOnFail: true,
				NotifyEmails: []string{"admin@example.com"},
			},
			expectError: false,
		},
		{
			name: "minimal valid request",
			request: &ScheduleRequest{
				Name:     "test",
				CronExpr: "* * * * *",
				Type:     "discovery",
				TargetID: 1,
			},
			expectError: false,
		},
		{
			name: "fails on invalid name",
			request: &ScheduleRequest{
				Name:     "",
				CronExpr: "* * * * *",
				Type:     "scan",
				TargetID: 1,
			},
			expectError: true,
			errorMsg:    "schedule name is required",
		},
		{
			name: "fails on invalid cron",
			request: &ScheduleRequest{
				Name:     "test",
				CronExpr: "invalid",
				Type:     "scan",
				TargetID: 1,
			},
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
		{
			name: "fails on invalid type",
			request: &ScheduleRequest{
				Name:     "test",
				CronExpr: "* * * * *",
				Type:     "invalid",
				TargetID: 1,
			},
			expectError: true,
			errorMsg:    "invalid schedule type",
		},
		{
			name: "fails on invalid target ID",
			request: &ScheduleRequest{
				Name:     "test",
				CronExpr: "* * * * *",
				Type:     "scan",
				TargetID: 0,
			},
			expectError: true,
			errorMsg:    "target ID must be positive",
		},
		{
			name: "fails on invalid tags",
			request: &ScheduleRequest{
				Name:     "test",
				CronExpr: "* * * * *",
				Type:     "scan",
				TargetID: 1,
				Tags:     []string{""},
			},
			expectError: true,
			errorMsg:    "tag 1 is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScheduleRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduleHandler_getScheduleFilters(t *testing.T) {
	handler := createTestScheduleHandler(t)

	tests := []struct {
		name            string
		queryParams     map[string]string
		expectedType    string
		expectedEnabled bool
	}{
		{
			name:            "no filters",
			queryParams:     map[string]string{},
			expectedType:    "",
			expectedEnabled: false,
		},
		{
			name:            "type filter only",
			queryParams:     map[string]string{"type": "scan"},
			expectedType:    "scan",
			expectedEnabled: false,
		},
		{
			name:            "enabled filter true",
			queryParams:     map[string]string{"enabled": "true"},
			expectedType:    "",
			expectedEnabled: true,
		},
		{
			name:            "enabled filter false",
			queryParams:     map[string]string{"enabled": "false"},
			expectedType:    "",
			expectedEnabled: false,
		},
		{
			name:            "both filters",
			queryParams:     map[string]string{"type": "discovery", "enabled": "true"},
			expectedType:    "discovery",
			expectedEnabled: true,
		},
		{
			name:            "invalid enabled value ignored",
			queryParams:     map[string]string{"enabled": "invalid"},
			expectedType:    "",
			expectedEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			filters := handler.getScheduleFilters(req)

			assert.Equal(t, tt.expectedType, filters.JobType)
			assert.Equal(t, tt.expectedEnabled, filters.Enabled)
		})
	}
}

func TestScheduleHandler_requestToDBSchedule(t *testing.T) {
	handler := createTestScheduleHandler(t)

	req := &ScheduleRequest{
		Name:         "test-schedule",
		Description:  "test description",
		CronExpr:     "0 * * * *",
		Type:         "scan",
		TargetID:     123,
		Enabled:      true,
		MaxRunTime:   30 * time.Minute,
		RetryOnError: true,
		MaxRetries:   3,
		RetryDelay:   5 * time.Minute,
		Options:      map[string]string{"key": "value"},
		Tags:         []string{"tag1", "tag2"},
		NotifyOnFail: true,
		NotifyEmails: []string{"admin@example.com"},
	}

	result := handler.requestToDBSchedule(req)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map")

	assert.Equal(t, req.Name, resultMap["name"])
	assert.Equal(t, req.Description, resultMap["description"])
	assert.Equal(t, req.CronExpr, resultMap["cron_expr"])
	assert.Equal(t, req.Type, resultMap["type"])
	assert.Equal(t, req.TargetID, resultMap["target_id"])
	assert.Equal(t, req.Enabled, resultMap["enabled"])
	assert.Equal(t, req.MaxRunTime, resultMap["max_run_time"])
	assert.Equal(t, req.RetryOnError, resultMap["retry_on_error"])
	assert.Equal(t, req.MaxRetries, resultMap["max_retries"])
	assert.Equal(t, req.RetryDelay, resultMap["retry_delay"])
	assert.Equal(t, req.Options, resultMap["options"])
	assert.Equal(t, req.Tags, resultMap["tags"])
	assert.Equal(t, req.NotifyOnFail, resultMap["notify_on_fail"])
	assert.Equal(t, req.NotifyEmails, resultMap["notify_emails"])
	assert.Equal(t, "pending", resultMap["status"])
	assert.Equal(t, 0, resultMap["run_count"])
	assert.Equal(t, 0, resultMap["success_count"])
	assert.Equal(t, 0, resultMap["error_count"])
	assert.NotNil(t, resultMap["created_at"])
	assert.NotNil(t, resultMap["updated_at"])
}

func TestScheduleHandler_scheduleToResponse(t *testing.T) {
	handler := createTestScheduleHandler(t)

	// The current implementation returns a placeholder
	result := handler.scheduleToResponse(nil)

	assert.NotZero(t, result.ID)
	assert.NotEmpty(t, result.CronExpr)
	assert.NotEmpty(t, result.Type)
	assert.NotZero(t, result.TargetID)
	assert.NotEmpty(t, result.Status)
	assert.NotZero(t, result.CreatedAt)
	assert.NotZero(t, result.UpdatedAt)
}

func TestScheduleRequest_JSONMarshaling(t *testing.T) {
	req := &ScheduleRequest{
		Name:         "test-schedule",
		Description:  "test description",
		CronExpr:     "0 * * * *",
		Type:         "scan",
		TargetID:     1,
		Enabled:      true,
		MaxRunTime:   30 * time.Minute,
		RetryOnError: true,
		MaxRetries:   3,
		RetryDelay:   5 * time.Minute,
		Options:      map[string]string{"key": "value"},
		Tags:         []string{"tag1"},
		NotifyOnFail: true,
		NotifyEmails: []string{"admin@example.com"},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded ScheduleRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.Name, decoded.Name)
	assert.Equal(t, req.Description, decoded.Description)
	assert.Equal(t, req.CronExpr, decoded.CronExpr)
	assert.Equal(t, req.Type, decoded.Type)
	assert.Equal(t, req.TargetID, decoded.TargetID)
	assert.Equal(t, req.Enabled, decoded.Enabled)
	assert.Equal(t, req.MaxRunTime, decoded.MaxRunTime)
	assert.Equal(t, req.RetryOnError, decoded.RetryOnError)
	assert.Equal(t, req.MaxRetries, decoded.MaxRetries)
	assert.Equal(t, req.RetryDelay, decoded.RetryDelay)
	assert.Equal(t, req.NotifyOnFail, decoded.NotifyOnFail)
}
