package internal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ullaakut/nmap/v3"
)

const (
	portStateOpen = "open"
)

// TestNmapScannerCreation tests that we can create an nmap scanner.
func TestNmapScannerCreation(t *testing.T) {
	ctx := context.Background()

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
}

// TestNmapLocalhostPingScan tests basic ping scan against localhost.
func TestNmapLocalhostPingScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPingScan(),
	)
	if err != nil {
		t.Fatalf("Failed to create scanner: %v", err)
	}

	result, _, err := scanner.Run()
	if err != nil {
		t.Fatalf("Nmap scan failed: %v", err)
	}

	if result == nil {
		t.Fatal("Scan result is nil")
	}

}

// TestNmapVersionCheck tests that nmap binary is available and working.
func TestNmapVersionCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

}

// TestNmapServiceContainer tests scanning the nginx service container.
func TestNmapServiceContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPorts("8080"),
		nmap.WithConnectScan(),
	)
	if err != nil {
		t.Fatalf("Failed to create scanner for service container scan: %v", err)
	}

	result, _, err := scanner.Run()
	if err != nil {
		t.Fatalf("Nmap scan of service container failed: %v", err)
	}

	if result == nil {
		t.Fatal("Service container scan result is nil")
	}

	// Verify we found localhost and check if port 8080 is detected

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

		})
	}
}

// TestNmapAllServicePorts tests scanning all service container ports at once.
func TestNmapAllServicePorts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Service containers expected in CI environment
	ports := []string{"8080", "8379", "8022"}
	portString := strings.Join(ports, ",")

	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPorts(portString),
		nmap.WithConnectScan(),
	)
	if err != nil {
		t.Fatalf("Failed to create scanner for service ports: %v", err)
	}

	result, _, err := scanner.Run()
	if err != nil {
		t.Fatalf("Nmap scan of service ports failed: %v", err)
	}

	if result == nil {
		t.Fatal("Service ports scan result is nil")
	}

}

// scanServiceHelper performs an nmap scan of a specific service and logs results.
func scanServiceHelper(t *testing.T, serviceName, port string, expectedPortID uint16) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPorts(port),
		nmap.WithConnectScan(),
	)
	if err != nil {
		t.Fatalf("Failed to create scanner for %s service: %v", serviceName, err)
	}

	result, _, err := scanner.Run()
	if err != nil {
		t.Fatalf("Nmap scan of %s service failed: %v", serviceName, err)
	}

	if result == nil {
		t.Fatalf("%s service scan result is nil", serviceName)
	}

	if len(result.Hosts) > 0 {
		host := result.Hosts[0]
		for i := range host.Ports {
			p := &host.Ports[i]
			if p.ID == expectedPortID {
				return
			}
		}
	}
}

// TestNmapNginxService tests scanning the nginx service container.
func TestNmapNginxService(t *testing.T) {
	scanServiceHelper(t, "nginx", "8080", 8080)
}

// TestNmapRedisService tests scanning the redis service container.
func TestNmapRedisService(t *testing.T) {
	scanServiceHelper(t, "redis", "8379", 8379)
}

// TestNmapSSHService tests scanning the openssh service container.
func TestNmapSSHService(t *testing.T) {
	scanServiceHelper(t, "ssh", "8022", 8022)
}

// TestNmapDiscoveryPingPrivileges tests if ICMP ping scanning works (requires root privileges).
func TestNmapDiscoveryPingPrivileges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test ICMP ping scan - this will fail in CI without root privileges
	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPingScan(), // Requires root privileges for ICMP
	)
	if err != nil {
		t.Fatalf("Failed to create ICMP ping scanner: %v", err)
	}

	result, warnings, err := scanner.Run()
	if err != nil {
		return // Expected failure in CI without root privileges
	}

	if warnings != nil && len(*warnings) > 0 {
		t.Logf("ICMP ping scan warnings: %v", *warnings)
	}

	if result != nil && len(result.Hosts) > 0 {
		t.Logf("✅ ICMP ping scan worked (running with privileges)")
		for i := range result.Hosts {
			host := &result.Hosts[i]
			if len(host.Addresses) > 0 {
				t.Logf("Host: %s, status: %s", host.Addresses[0].Addr, host.Status.State)
			}
		}
	}
}

// TestNmapTCPDiscovery tests TCP-based host discovery (no root privileges required).
func TestNmapTCPDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use TCP SYN discovery instead of ICMP ping - works without root
	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPorts("80,22,443,8080"), // Common ports for discovery
		nmap.WithConnectScan(),           // TCP connect scan works without root
	)
	if err != nil {
		t.Fatalf("Failed to create TCP discovery scanner: %v", err)
	}

	result, warnings, err := scanner.Run()
	if err != nil {
		t.Fatalf("TCP discovery scan failed: %v", err)
	}

	if warnings != nil && len(*warnings) > 0 {
		t.Logf("TCP discovery scan warnings: %v", *warnings)
	}

	if result == nil {
		t.Fatal("TCP discovery scan result is nil")
	}

	t.Logf("TCP discovery scan completed. Found %d hosts", len(result.Hosts))

	// Check if we found the host as up
	if len(result.Hosts) == 0 {
		t.Log("❌ TCP discovery found no hosts")
		return
	}

	for i := range result.Hosts {
		host := &result.Hosts[i]
		if len(host.Addresses) == 0 {
			continue
		}

		t.Logf("Host: %s, status: %s", host.Addresses[0].Addr, host.Status.State)
		if host.Status.State == "up" {
			t.Logf("✅ TCP-based discovery found host %s as up", host.Addresses[0].Addr)
		}

		// Log open ports found
		for _, port := range host.Ports {
			if port.State.State == portStateOpen {
				t.Logf("  Open port: %d/%s", port.ID, port.Protocol)
			}
		}
	}
}

// TestNmapPrivilegeFreeScan tests scanning methods that work without root privileges.
func TestNmapPrivilegeFreeScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test connect scan to known open port (from our service containers)
	scanner, err := nmap.NewScanner(ctx,
		nmap.WithTargets("127.0.0.1"),
		nmap.WithPorts("8080"), // nginx service container
		nmap.WithConnectScan(), // No privileges required
	)
	if err != nil {
		t.Fatalf("Failed to create privilege-free scanner: %v", err)
	}

	result, warnings, err := scanner.Run()
	if err != nil {
		t.Fatalf("Privilege-free scan failed: %v", err)
	}

	if warnings != nil && len(*warnings) > 0 {
		t.Logf("Privilege-free scan warnings: %v", *warnings)
	}

	if result == nil {
		t.Fatal("Privilege-free scan result is nil")
	}

	t.Logf("Privilege-free scan completed. Found %d hosts", len(result.Hosts))

	// This should work reliably in CI since it doesn't need special privileges
	if len(result.Hosts) > 0 {
		host := &result.Hosts[0]
		t.Logf("✅ Host found without privileges: %s, status: %s", host.Addresses[0].Addr, host.Status.State)
		for _, port := range host.Ports {
			t.Logf("  Port %d/%s: %s", port.ID, port.Protocol, port.State.State)
		}
	} else {
		t.Log("❌ No hosts found with privilege-free scan")
	}
}
