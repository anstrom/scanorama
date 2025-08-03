package main

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

func TestParseFlags(t *testing.T) {
	// Save original args and flags
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()

	tests := []struct {
		name     string
		args     []string
		expected ScanOptions
	}{
		{
			name: "default values",
			args: []string{"cmd"},
			expected: ScanOptions{
				Targets:  "",
				Ports:    "1-1000",
				ScanType: "syn",
				Output:   "",
			},
		},
		{
			name: "all options specified",
			args: []string{
				"cmd",
				"-targets", "192.168.1.1",
				"-ports", "80,443",
				"-type", "version",

				"-output", "results.xml",
			},
			expected: ScanOptions{
				Targets:  "192.168.1.1",
				Ports:    "80,443",
				ScanType: "version",
				Output:   "results.xml",
			},
		},
		{
			name: "multiple targets",
			args: []string{
				"cmd",
				"-targets", "192.168.1.1,192.168.1.2",
				"-ports", "22-80",
			},
			expected: ScanOptions{
				Targets:  "192.168.1.1,192.168.1.2",
				Ports:    "22-80",
				ScanType: "syn",
				Output:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			// Set args
			os.Args = tt.args

			got := ParseFlags()

			if got.Targets != tt.expected.Targets {
				t.Errorf("ParseFlags().Targets = %v, want %v", got.Targets, tt.expected.Targets)
			}
			if got.Ports != tt.expected.Ports {
				t.Errorf("ParseFlags().Ports = %v, want %v", got.Ports, tt.expected.Ports)
			}
			if got.ScanType != tt.expected.ScanType {
				t.Errorf("ParseFlags().ScanType = %v, want %v", got.ScanType, tt.expected.ScanType)
			}

			if got.Output != tt.expected.Output {
				t.Errorf("ParseFlags().Output = %v, want %v", got.Output, tt.expected.Output)
			}
		})
	}
}

func validateFlags(opts ScanOptions) error {
	if opts.Targets == "" {
		return fmt.Errorf("Target hosts are required")
	}

	validScanTypes := map[string]bool{
		"syn":     true,
		"connect": true,
		"version": true,
	}
	if !validScanTypes[opts.ScanType] {
		return fmt.Errorf("Invalid scan type: %s", opts.ScanType)
	}

	return nil
}

func TestFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "valid local scan",
			args: []string{
				"cmd",
				"-targets", "localhost",
				"-ports", "8080,8443",
				"-type", "connect",
			},
			wantErr: false,
		},
		{
			name: "missing target",
			args: []string{
				"cmd",
				"-targets", "",
			},
			wantErr: true,
		},
		{
			name: "invalid scan type",
			args: []string{
				"cmd",
				"-targets", "localhost",
				"-type", "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original args/flags
			oldArgs := os.Args
			oldFlagCommandLine := flag.CommandLine
			defer func() {
				os.Args = oldArgs
				flag.CommandLine = oldFlagCommandLine
			}()

			// Set up test args
			os.Args = tt.args
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ExitOnError)

			// Get and validate options
			opts := ParseFlags()
			err := validateFlags(opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
