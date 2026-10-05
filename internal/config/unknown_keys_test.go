package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestUnknownYAMLKeys(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "known keys only",
			yaml: "scanning:\n  scan_mode: connect\n",
			want: nil,
		},
		{
			name: "unknown nested key",
			yaml: "scanning:\n  default_scan_type: connect\n",
			want: []string{"default_scan_type"},
		},
		{
			name: "unknown top-level and nested keys",
			yaml: "monitoring: {}\napi:\n  cors: true\n",
			want: []string{"monitoring", "cors"},
		},
		{
			name: "invalid yaml reports nothing",
			yaml: "scanning: [unclosed\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unknownYAMLKeys([]byte(tt.yaml))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d unknown keys %q, want %d (%q)", len(got), got, len(tt.want), tt.want)
			}
			for i, key := range tt.want {
				if !strings.Contains(got[i], "field "+key+" not found") {
					t.Errorf("entry %d = %q, want it to name %q", i, got[i], key)
				}
			}
		})
	}
}

func TestDecodeYAMLConfigWarnsOnUnknownKeys(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := Default()
	data := []byte("scanning:\n  scan_mode: connect\n  default_scan_type: syn\n")
	if err := decodeYAMLConfig("test.yaml", data, cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if cfg.Scanning.ScanMode != validScanConnect {
		t.Errorf("scan_mode = %q, want %q", cfg.Scanning.ScanMode, validScanConnect)
	}
	out := buf.String()
	if !strings.Contains(out, "Ignoring unknown config key") || !strings.Contains(out, "default_scan_type") {
		t.Errorf("expected a warning naming default_scan_type, got log output: %q", out)
	}
	if strings.Contains(out, "scan_mode") {
		t.Errorf("known key scan_mode should not be reported: %q", out)
	}
}
