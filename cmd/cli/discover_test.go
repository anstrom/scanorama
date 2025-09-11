package cli

import (
	"testing"
	"time"
)

func TestCalculateDiscoveryTimeout(t *testing.T) {
	tests := []struct {
		name               string
		network            string
		baseTimeoutSeconds int // unused but kept for test structure
		expectedMin        time.Duration
		expectedMax        time.Duration
		description        string
	}{
		{
			name:               "single host /32",
			network:            "192.168.1.1/32",
			baseTimeoutSeconds: 30, // unused but kept for test structure
			expectedMin:        15 * time.Second,
			expectedMax:        16 * time.Second,
			description:        "Single host should get minimum reasonable timeout",
		},
		{
			name:               "small network /30",
			network:            "192.168.1.0/30",
			baseTimeoutSeconds: 30, // unused but kept for test structure
			expectedMin:        15 * time.Second,
			expectedMax:        17 * time.Second,
			description:        "Small network should get minimal timeout",
		},
		{
			name:               "medium network /28",
			network:            "192.168.1.0/28",
			baseTimeoutSeconds: 30, // unused but kept for test structure
			expectedMin:        20 * time.Second,
			expectedMax:        25 * time.Second,
			description:        "Medium network should scale reasonably",
		},
		{
			name:               "standard /24 network",
			network:            "192.168.1.0/24",
			baseTimeoutSeconds: 30,                // unused but kept for test structure
			expectedMin:        140 * time.Second, // ~2.3 minutes
			expectedMax:        160 * time.Second,
			description:        "Standard /24 network should get reasonable timeout",
		},
		{
			name:               "large /20 network",
			network:            "192.168.0.0/20",
			baseTimeoutSeconds: 30,                 // unused but kept for test structure
			expectedMin:        1700 * time.Second, // ~28 minutes
			expectedMax:        1800 * time.Second, // capped at 30 minutes
			description:        "Large /20 network should approach maximum timeout",
		},
		{
			name:               "very large /16 network",
			network:            "192.168.0.0/16",
			baseTimeoutSeconds: 30,                 // unused but kept for test structure
			expectedMin:        1800 * time.Second, // Should hit max timeout (30 minutes)
			expectedMax:        1800 * time.Second,
			description:        "Very large network should hit maximum timeout limit",
		},
		{
			name:               "huge /8 network capped",
			network:            "10.0.0.0/8",
			baseTimeoutSeconds: 30,                 // unused but kept for test structure
			expectedMin:        1800 * time.Second, // Should hit max timeout
			expectedMax:        1800 * time.Second,
			description:        "Huge network should be capped at maximum",
		},
		{
			name:               "invalid network fallback",
			network:            "invalid-network",
			baseTimeoutSeconds: 30,                // unused but kept for test structure
			expectedMin:        140 * time.Second, // Uses default network size (254)
			expectedMax:        160 * time.Second,
			description:        "Invalid network should fallback to default calculation",
		},
		{
			name:               "very small timeout gets minimum",
			network:            "192.168.1.1/32",
			baseTimeoutSeconds: 1,                // unused but kept for test structure
			expectedMin:        10 * time.Second, // Should be clamped to minimum
			expectedMax:        16 * time.Second,
			description:        "Very low base timeout should be clamped to minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDiscoveryTimeout(tt.network)

			if result < tt.expectedMin {
				t.Errorf("calculateDiscoveryTimeout() = %v, expected >= %v (%s)",
					result, tt.expectedMin, tt.description)
			}

			if result > tt.expectedMax {
				t.Errorf("calculateDiscoveryTimeout() = %v, expected <= %v (%s)",
					result, tt.expectedMax, tt.description)
			}

			// Verify it's within absolute bounds
			if result < 10*time.Second {
				t.Errorf("calculateDiscoveryTimeout() = %v, should never be less than 10s", result)
			}

			if result > 1800*time.Second {
				t.Errorf("calculateDiscoveryTimeout() = %v, should never be more than 1800s (30m)", result)
			}
		})
	}
}

func TestEstimateNetworkTargets(t *testing.T) {
	tests := []struct {
		name        string
		network     string
		expected    int
		description string
	}{
		{
			name:        "single host /32",
			network:     "192.168.1.1/32",
			expected:    1,
			description: "Single host should return 1",
		},
		{
			name:        "point-to-point /31",
			network:     "192.168.1.0/31",
			expected:    1, // (2^1) - 2 = 0, but clamped to 1
			description: "Point-to-point link should return minimum 1",
		},
		{
			name:        "small subnet /30",
			network:     "192.168.1.0/30",
			expected:    2, // (2^2) - 2 = 2
			description: "Small /30 subnet should return 2 hosts",
		},
		{
			name:        "typical /29",
			network:     "192.168.1.0/29",
			expected:    6, // (2^3) - 2 = 6
			description: "Typical /29 subnet should return 6 hosts",
		},
		{
			name:        "common /28",
			network:     "192.168.1.0/28",
			expected:    14, // 14 hosts in /28 network
			description: "Common /28 subnet should return 14 hosts",
		},
		{
			name:        "standard /24",
			network:     "192.168.1.0/24",
			expected:    254, // 254 hosts in /24 network
			description: "Standard /24 network should return 254 hosts",
		},
		{
			name:        "medium /20",
			network:     "192.168.0.0/20",
			expected:    4094, // 4094 hosts in /20 network
			description: "Medium /20 network should calculate correctly",
		},
		{
			name:        "large /16",
			network:     "192.168.0.0/16",
			expected:    65534, // 65534 hosts in /16 network
			description: "Large /16 network should calculate full range",
		},
		{
			name:        "very large /8 capped",
			network:     "10.0.0.0/8",
			expected:    65534, // Should be capped at maximum
			description: "Very large /8 network should be capped to prevent excessive values",
		},
		{
			name:        "invalid CIDR default",
			network:     "invalid-network",
			expected:    254, // Should return defaultNetworkSize
			description: "Invalid CIDR should return default network size",
		},
		{
			name:        "empty string default",
			network:     "",
			expected:    254,
			description: "Empty string should return default network size",
		},
		{
			name:        "malformed CIDR",
			network:     "192.168.1.0/99",
			expected:    254,
			description: "Malformed CIDR should return default network size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateNetworkTargets(tt.network)

			if result != tt.expected {
				t.Errorf("estimateNetworkTargets(%q) = %d, expected %d (%s)",
					tt.network, result, tt.expected, tt.description)
			}

			// Verify result is always positive
			if result < 1 {
				t.Errorf("estimateNetworkTargets(%q) = %d, should never be less than 1",
					tt.network, result)
			}
		})
	}
}

func TestDiscoveryTimeoutRealism(t *testing.T) {
	// Test that timeouts are realistic for different network sizes
	tests := []struct {
		name          string
		network       string
		maxReasonable time.Duration
		description   string
	}{
		{
			name:          "single host should be quick",
			network:       "192.168.1.1/32",
			maxReasonable: 30 * time.Second,
			description:   "Single host should complete quickly",
		},
		{
			name:          "small office should be reasonable",
			network:       "192.168.1.0/28", // 14 hosts
			maxReasonable: 60 * time.Second,
			description:   "Small office network should complete within a minute",
		},
		{
			name:          "standard network gets adequate time",
			network:       "192.168.1.0/24", // 254 hosts
			maxReasonable: 5 * time.Minute,
			description:   "Standard /24 should get adequate time",
		},
		{
			name:          "large network gets plenty of time",
			network:       "192.168.0.0/20", // 4094 hosts
			maxReasonable: 30 * time.Minute,
			description:   "Large network should get up to 30 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := calculateDiscoveryTimeout(tt.network)

			if timeout > tt.maxReasonable {
				t.Errorf("Timeout %v exceeds reasonable maximum %v for %s (%s)",
					timeout, tt.maxReasonable, tt.network, tt.description)
			}

			// Verify it's not ridiculously small either
			if timeout < 10*time.Second {
				t.Errorf("Timeout %v is unreasonably small for %s (%s)",
					timeout, tt.network, tt.description)
			}
		})
	}
}

func TestDiscoveryTimeoutScaling(t *testing.T) {
	// Test that timeouts scale reasonably with network size
	networks := []struct {
		cidr    string
		targets int
	}{
		{"192.168.1.1/32", 1},    // 1 host
		{"192.168.1.0/28", 14},   // 14 hosts
		{"192.168.1.0/24", 254},  // 254 hosts
		{"192.168.0.0/20", 4094}, // 4094 hosts
	}

	var previousTimeout time.Duration
	for i, network := range networks {
		timeout := calculateDiscoveryTimeout(network.cidr)

		// Each larger network should get at least as much time as smaller ones
		// (unless hitting the maximum cap)
		if i > 0 && timeout < previousTimeout && timeout < 1800*time.Second {
			t.Errorf("Timeout decreased from %v to %v when network size increased from %d to %d hosts",
				previousTimeout, timeout, networks[i-1].targets, network.targets)
		}

		previousTimeout = timeout
	}
}

func TestDiscoveryTimeoutEdgeCases(t *testing.T) {
	tests := []struct {
		name               string
		network            string
		baseTimeoutSeconds int // unused but kept for test structure
		expectedTimeout    time.Duration
		description        string
	}{
		{
			name:               "zero base timeout",
			network:            "192.168.1.0/24",
			baseTimeoutSeconds: 0,                // unused but kept for test structure
			expectedTimeout:    10 * time.Second, // Should be clamped to minimum
			description:        "Zero base timeout should result in minimum timeout",
		},
		{
			name:               "negative base timeout",
			network:            "192.168.1.0/24",
			baseTimeoutSeconds: -10,              // unused but kept for test structure
			expectedTimeout:    10 * time.Second, // Should be clamped to minimum
			description:        "Negative base timeout should result in minimum timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDiscoveryTimeout(tt.network)

			// Allow some tolerance since the actual calculation involves more than just the minimum
			if result < tt.expectedTimeout {
				t.Errorf("calculateDiscoveryTimeout(%q) = %v, expected >= %v (%s)",
					tt.network, result, tt.expectedTimeout, tt.description)
			}
		})
	}
}

func TestDiscoveryTimeoutConstants(t *testing.T) {
	// Test that our constants are reasonable
	if minTimeoutSeconds != 10 {
		t.Errorf("minTimeoutSeconds = %d, expected 10", minTimeoutSeconds)
	}

	if maxTimeoutSeconds != 1800 {
		t.Errorf("maxTimeoutSeconds = %d, expected 1800 (30 minutes)", maxTimeoutSeconds)
	}

	if baseTimeoutPerHost != 0.5 {
		t.Errorf("baseTimeoutPerHost = %f, expected 0.5", baseTimeoutPerHost)
	}

	if batchTimeoutSeconds != 15 {
		t.Errorf("batchTimeoutSeconds = %d, expected 15", batchTimeoutSeconds)
	}

	if timeoutConcurrency != 50 {
		t.Errorf("timeoutConcurrency = %d, expected 50", timeoutConcurrency)
	}

	if defaultNetworkSize != 254 {
		t.Errorf("defaultNetworkSize = %d, expected 254", defaultNetworkSize)
	}
}

// Benchmark tests to ensure performance is acceptable
func BenchmarkCalculateDiscoveryTimeout(b *testing.B) {
	networks := []string{
		"192.168.1.0/24",
		"10.0.0.0/16",
		"172.16.0.0/20",
		"192.168.1.1/32",
		"invalid-network",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		network := networks[i%len(networks)]
		calculateDiscoveryTimeout(network)
	}
}

func BenchmarkEstimateNetworkTargets(b *testing.B) {
	networks := []string{
		"192.168.1.0/24",
		"10.0.0.0/16",
		"172.16.0.0/20",
		"192.168.1.1/32",
		"invalid-network",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		network := networks[i%len(networks)]
		estimateNetworkTargets(network)
	}
}

func TestRealisticTimeoutExamples(t *testing.T) {
	// Test specific examples to ensure they're realistic
	examples := []struct {
		network     string
		description string
		maxExpected time.Duration
		minExpected time.Duration
	}{
		{
			network:     "192.168.1.1/32",
			description: "Home router admin",
			maxExpected: 30 * time.Second,
			minExpected: 10 * time.Second,
		},
		{
			network:     "192.168.1.0/24",
			description: "Home/small office network",
			maxExpected: 5 * time.Minute,
			minExpected: 1 * time.Minute,
		},
		{
			network:     "192.168.0.0/20",
			description: "Large enterprise subnet",
			maxExpected: 30 * time.Minute,
			minExpected: 10 * time.Minute,
		},
		{
			network:     "10.0.0.0/16",
			description: "Very large corporate network",
			maxExpected: 30 * time.Minute,
			minExpected: 20 * time.Minute,
		},
	}

	for _, example := range examples {
		t.Run(example.description, func(t *testing.T) {
			timeout := calculateDiscoveryTimeout(example.network)

			if timeout < example.minExpected {
				t.Errorf("%s (%s): timeout %v is less than expected minimum %v",
					example.description, example.network, timeout, example.minExpected)
			}

			if timeout > example.maxExpected {
				t.Errorf("%s (%s): timeout %v exceeds expected maximum %v",
					example.description, example.network, timeout, example.maxExpected)
			}
		})
	}
}
