package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShippedConfigsScanMode loads every config file shipped in the repository,
// checks that it passes validation, and that its scanning mode resolves to what
// the file intends. Each of these once set the unread key default_scan_type, so
// they silently ran syn scans while advertising connect.
func TestShippedConfigsScanMode(t *testing.T) {
	tests := []struct {
		path string
		want string
		// skipValidation exempts files that don't yet load as a whole;
		// config.template.yaml uses database key names nothing reads (#1099).
		skipValidation bool
	}{
		{"config/environments/config.dev.yaml", validScanConnect, false},
		{"config/environments/config.docker-test.yaml", validScanConnect, false},
		{"config/environments/config.example.yaml", validScanConnect, false},
		{"config/environments/config.full.yaml", validScanConnect, false},
		{"config/environments/config.minimal.yaml", validScanConnect, false},
		{"config/environments/config.test.yaml", validScanConnect, false},
		{"config/config.template.yaml", defaultScanMode, true},
		{"deploy/config.example.yaml", validScanConnect, false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", tt.path))
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			cfg := Default()
			if err := safeYAMLUnmarshal(data, cfg); err != nil {
				t.Fatalf("decode %s: %v", tt.path, err)
			}
			if result := ValidateAndNormalize(cfg); !tt.skipValidation && result.HasErrors() {
				t.Errorf("validation failed: %v", result.AsError())
			}
			if cfg.Scanning.ScanMode != tt.want {
				t.Errorf("scan_mode = %q, want %q", cfg.Scanning.ScanMode, tt.want)
			}
			for _, msg := range unknownYAMLKeys(data) {
				if strings.Contains(msg, "default_scan_type") {
					t.Errorf("unread key still present: %s", msg)
				}
			}
		})
	}
}
