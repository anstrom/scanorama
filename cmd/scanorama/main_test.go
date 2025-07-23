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
		expected scanOptions
	}{
		{
			name: "default values",
			args: []string{"cmd"},
			expected: scanOptions{
				targets:  "",
				ports:    "1-1000",
				scanType: "syn",
				output:   "",
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
			expected: scanOptions{
				targets:  "192.168.1.1",
				ports:    "80,443",
				scanType: "version",
				output:   "results.xml",
			},
		},
		{
			name: "multiple targets",
			args: []string{
				"cmd",
				"-targets", "192.168.1.1,192.168.1.2",
				"-ports", "22-80",
			},
			expected: scanOptions{
				targets:  "192.168.1.1,192.168.1.2",
				ports:    "22-80",
				scanType: "syn",
				output:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			// Set args
			os.Args = tt.args

			got := parseFlags()

			if got.targets != tt.expected.targets {
				t.Errorf("parseFlags().targets = %v, want %v", got.targets, tt.expected.targets)
			}
			if got.ports != tt.expected.ports {
				t.Errorf("parseFlags().ports = %v, want %v", got.ports, tt.expected.ports)
			}
			if got.scanType != tt.expected.scanType {
				t.Errorf("parseFlags().scanType = %v, want %v", got.scanType, tt.expected.scanType)
			}

			if got.output != tt.expected.output {
				t.Errorf("parseFlags().output = %v, want %v", got.output, tt.expected.output)
			}
		})
	}
}

func validateFlags(opts scanOptions) error {
	if opts.targets == "" {
		return fmt.Errorf("Target hosts are required")
	}

	validScanTypes := map[string]bool{
		"syn":     true,
		"connect": true,
		"version": true,
	}
	if !validScanTypes[opts.scanType] {
		return fmt.Errorf("Invalid scan type: %s", opts.scanType)
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
			opts := parseFlags()
			err := validateFlags(opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
