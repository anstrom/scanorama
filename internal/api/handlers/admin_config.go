// Package handlers - admin configuration retrieval and extraction.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// getCurrentConfig retrieves current configuration.
func (h *AdminHandler) getCurrentConfig(_ context.Context, section string) (map[string]interface{}, error) {
	// This would get the actual configuration from the config manager
	// For now, return mock configuration data

	config := ConfigResponse{
		API: map[string]interface{}{
			"enabled":             true,
			"host":                "127.0.0.1",
			"port":                8080,
			"auth_enabled":        false,
			"rate_limit_enabled":  true,
			"rate_limit_requests": 100,
			"read_timeout":        "10s",
			"write_timeout":       "10s",
		},
		Database: map[string]interface{}{
			"host":            "localhost",
			"port":            5432,
			"database":        "scanorama",
			"ssl_mode":        "require",
			"max_connections": 25,
		},
		Scanning: map[string]interface{}{
			"worker_pool_size":         10,
			"scan_mode":                "syn",
			"max_concurrent_targets":   100,
			"default_ports":            "22,80,443,8080,8443",
			"enable_service_detection": true,
		},
		Logging: map[string]interface{}{
			"level":      "info",
			"format":     "text",
			"output":     "stdout",
			"structured": true,
		},
		Daemon: map[string]interface{}{
			"pid_file":         "/tmp/scanorama.pid",
			"shutdown_timeout": "30s",
			"daemonize":        true,
		},
	}

	// Return specific section if requested
	if section != "" {
		switch section {
		case "api":
			return config.API.(map[string]interface{}), nil
		case "database":
			return config.Database.(map[string]interface{}), nil
		case "scanning":
			return config.Scanning.(map[string]interface{}), nil
		case "logging":
			return config.Logging.(map[string]interface{}), nil
		case "daemon":
			return config.Daemon.(map[string]interface{}), nil
		default:
			return nil, fmt.Errorf("unknown configuration section: %s", section)
		}
	}

	// Return entire config as map
	return map[string]interface{}{
		"api":      config.API,
		"database": config.Database,
		"scanning": config.Scanning,
		"logging":  config.Logging,
		"daemon":   config.Daemon,
	}, nil
}

// parseConfigJSON safely parses JSON with size limits and security constraints.
func parseConfigJSON(r *http.Request, dest interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}

	// Enforce maximum request size
	r.Body = http.MaxBytesReader(nil, r.Body, maxConfigSize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	// Use strict number handling to prevent precision issues
	decoder.UseNumber()

	if err := decoder.Decode(dest); err != nil {
		if err.Error() == "http: request body too large" {
			return fmt.Errorf("configuration data too large (max 1MB)")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// structToMap safely converts a struct to map[string]interface{} for processing.
func structToMap(v interface{}) (map[string]interface{}, error) {
	// Use JSON marshaling/unmarshaling for safe conversion
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Remove nil values to avoid overwriting with empty values
	cleaned := make(map[string]interface{})
	for k, v := range result {
		if v != nil {
			cleaned[k] = v
		}
	}

	return cleaned, nil
}

// extractConfigSection safely extracts the configuration data for the specified section.
func (h *AdminHandler) extractConfigSection(req *ConfigUpdateRequest) (map[string]interface{}, error) {
	return h.extractConfigData(req)
}

// extractConfigData extracts configuration data by section (extracted to reduce complexity).
func (h *AdminHandler) extractConfigData(req *ConfigUpdateRequest) (map[string]interface{}, error) {
	switch req.Section {
	case configSectionAPI:
		return h.extractAPIConfigData(req.Config.API)
	case configSectionDatabase:
		return h.extractDatabaseConfigData(req.Config.Database)
	case configSectionScanning:
		return h.extractScanningConfigData(req.Config.Scanning)
	case configSectionLogging:
		return h.extractLoggingConfigData(req.Config.Logging)
	case configSectionDaemon:
		return h.extractDaemonConfigData(req.Config.Daemon)
	default:
		return nil, fmt.Errorf("unsupported configuration section: %s", req.Section)
	}
}

// extractAPIConfigData safely extracts API configuration data.
func (h *AdminHandler) extractAPIConfigData(config *APIConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("api configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process api config: %w", err)
	}
	return data, nil
}

// extractDatabaseConfigData safely extracts database configuration data.
func (h *AdminHandler) extractDatabaseConfigData(config *DatabaseConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process database config: %w", err)
	}
	return data, nil
}

// extractScanningConfigData safely extracts scanning configuration data.
func (h *AdminHandler) extractScanningConfigData(config *ScanningConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("scanning configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process scanning config: %w", err)
	}
	return data, nil
}

// extractLoggingConfigData safely extracts logging configuration data.
func (h *AdminHandler) extractLoggingConfigData(config *LoggingConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("logging configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process logging config: %w", err)
	}
	return data, nil
}

// extractDaemonConfigData safely extracts daemon configuration data.
func (h *AdminHandler) extractDaemonConfigData(config *DaemonConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("daemon configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process daemon config: %w", err)
	}
	return data, nil
}
