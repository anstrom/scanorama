package internal

import (
	"context"
	"testing"
	"time"

	"github.com/Ullaakut/nmap/v3"
)

// TestNmapBasicFunctionality tests basic nmap functionality without external dependencies.
func TestNmapBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	t.Run("scanner_creation", func(t *testing.T) {
		// Test that we can create an nmap scanner
		scanner, err := nmap.NewScanner(ctx,
			nmap.WithTargets("127.0.0.1"),
			nmap.WithPingScan(),
		)
		if err != nil {
			t.Fatalf("Failed to create nmap scanner: %v", err)
		}
		if scanner == nil {
			t.Fatal("Scanner is nil")
		}
	})

	t.Run("localhost_ping_scan", func(t *testing.T) {
		// Test basic ping scan against localhost
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		scanner, err := nmap.NewScanner(ctx,
			nmap.WithTargets("127.0.0.1"),
			nmap.WithPingScan(),
		)
		if err != nil {
			t.Fatalf("Failed to create scanner: %v", err)
		}

		result, warnings, err := scanner.Run()
		if err != nil {
			t.Fatalf("Nmap scan failed: %v", err)
		}

		if warnings != nil && len(*warnings) > 0 {
			t.Logf("Scan completed with warnings: %v", *warnings)
		}

		if result == nil {
			t.Fatal("Scan result is nil")
		}

		t.Logf("Scan completed successfully. Found %d hosts", len(result.Hosts))
	})

	t.Run("nmap_version_check", func(t *testing.T) {
		// Test that nmap binary is available and working
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Create a minimal scanner just to check nmap is available
		scanner, err := nmap.NewScanner(ctx,
			nmap.WithTargets("127.0.0.1"),
			nmap.WithPingScan(),
		)
		if err != nil {
			t.Fatalf("Nmap binary not available or scanner creation failed: %v", err)
		}

		// We don't need to run it, just verify it can be created
		if scanner == nil {
			t.Fatal("Scanner creation returned nil")
		}

		t.Log("Nmap scanner created successfully - nmap binary is available")
	})
}

// TestNmapOptions tests that we can build nmap options like in discovery.
func TestNmapOptions(t *testing.T) {
	testCases := []struct {
		name    string
		targets []string
		timeout time.Duration
	}{
		{
			name:    "localhost_quick",
			targets: []string{"127.0.0.1"},
			timeout: 5 * time.Second,
		},
		{
			name:    "localhost_normal",
			targets: []string{"127.0.0.1"},
			timeout: 15 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build options similar to discovery process
			options := []nmap.Option{
				nmap.WithTargets(tc.targets...),
				nmap.WithPingScan(),
			}

			// Add timing based on timeout like in discovery
			if tc.timeout <= 5*time.Second {
				options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
			} else if tc.timeout <= 15*time.Second {
				options = append(options, nmap.WithTimingTemplate(nmap.TimingNormal))
			} else {
				options = append(options, nmap.WithTimingTemplate(nmap.TimingPolite))
			}

			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout*2)
			defer cancel()

			scanner, err := nmap.NewScanner(ctx, options...)
			if err != nil {
				t.Fatalf("Failed to create scanner with options: %v", err)
			}

			if scanner == nil {
				t.Fatal("Scanner is nil")
			}

			t.Logf("Successfully created scanner for %s with %d second timeout",
				tc.targets[0], int(tc.timeout.Seconds()))
		})
	}
}
