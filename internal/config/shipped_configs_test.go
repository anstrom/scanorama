package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shippedConfigs lists every config file shipped in the repository with the
// scanning mode it is meant to resolve to.
var shippedConfigs = []struct {
	path string
	want string
}{
	{"config/environments/config.dev.yaml", validScanConnect},
	{"config/environments/config.docker-test.yaml", validScanConnect},
	{"config/environments/config.example.yaml", validScanConnect},
	{"config/environments/config.full.yaml", validScanConnect},
	{"config/environments/config.minimal.yaml", validScanConnect},
	{"config/environments/config.test.yaml", validScanConnect},
	{"config/config.template.yaml", validScanConnect},
	{"deploy/config.example.yaml", validScanConnect},
}

// TestShippedConfigsScanMode loads every shipped config, checks that it passes
// validation, that its scanning mode resolves to what the file intends, and that
// it sets no key nothing reads. Each of these once set the unread key
// default_scan_type, so they silently ran syn scans while advertising connect;
// others set api_key, which left authentication off (#1099).
func TestShippedConfigsScanMode(t *testing.T) {
	for _, tt := range shippedConfigs {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", tt.path))
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			cfg := Default()
			if err := safeYAMLUnmarshal(data, cfg); err != nil {
				t.Fatalf("decode %s: %v", tt.path, err)
			}
			if result := ValidateAndNormalize(cfg); result.HasErrors() {
				t.Errorf("validation failed: %v", result.AsError())
			}
			if cfg.Scanning.ScanMode != tt.want {
				t.Errorf("scan_mode = %q, want %q", cfg.Scanning.ScanMode, tt.want)
			}
			for _, msg := range unknownYAMLKeys(data) {
				t.Errorf("unread key: %s", msg)
			}
		})
	}
}

// TestShippedConfigsAreListed fails when a config file is added under config/
// or deploy/ without an entry in TestShippedConfigsScanMode, so the strict
// unread-key check there covers every example config.
func TestShippedConfigsAreListed(t *testing.T) {
	var found []string
	for _, pattern := range []string{"config/*.yaml", "config/environments/*.yaml", "deploy/*.yaml"} {
		matches, err := filepath.Glob(filepath.Join("..", "..", pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		found = append(found, matches...)
	}
	listed := make(map[string]bool, len(shippedConfigs))
	for _, c := range shippedConfigs {
		listed[filepath.Join("..", "..", c.path)] = true
	}
	for _, path := range found {
		if strings.HasSuffix(path, ".local.yaml") || strings.HasSuffix(path, "environments/config.yaml") {
			continue // gitignored per-developer files
		}
		if !listed[path] {
			t.Errorf("%s is not covered by TestShippedConfigsScanMode", path)
		}
	}
}
