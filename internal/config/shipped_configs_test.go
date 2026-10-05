package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShippedConfigsScanMode loads every config file shipped in the repository
// and checks that its scanning mode resolves to what the file intends. Each of
// these once set the unread key default_scan_type, so they silently ran syn
// scans while advertising connect.
func TestShippedConfigsScanMode(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"config/environments/config.dev.yaml", validScanConnect},
		{"config/environments/config.docker-test.yaml", validScanConnect},
		{"config/environments/config.example.yaml", validScanConnect},
		{"config/environments/config.full.yaml", validScanConnect},
		{"config/environments/config.minimal.yaml", validScanConnect},
		{"config/environments/config.test.yaml", validScanConnect},
		{"config/config.template.yaml", defaultScanMode},
		{"deploy/config.example.yaml", validScanConnect},
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
