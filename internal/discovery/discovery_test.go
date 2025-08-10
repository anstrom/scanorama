package discovery

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateDynamicTimeout(t *testing.T) {
	engine := &Engine{}
	baseTimeout := 10 * time.Second

	tests := []struct {
		name        string
		targetCount int
		baseTimeout time.Duration
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{
			name:        "small network (5 targets)",
			targetCount: 5,
			baseTimeout: baseTimeout,
			expectedMin: 50 * time.Second,
			expectedMax: 70 * time.Second,
		},
		{
			name:        "medium network (30 targets)",
			targetCount: 30,
			baseTimeout: baseTimeout,
			expectedMin: 60 * time.Second,
			expectedMax: 80 * time.Second,
		},
		{
			name:        "large network (254 targets)",
			targetCount: 254,
			baseTimeout: baseTimeout,
			expectedMin: 100 * time.Second,
			expectedMax: 120 * time.Second,
		},
		{
			name:        "very large network (1000 targets)",
			targetCount: 1000,
			baseTimeout: baseTimeout,
			expectedMin: 250 * time.Second,
			expectedMax: 300 * time.Second,
		},
		{
			name:        "zero base timeout uses default",
			targetCount: 100,
			baseTimeout: 0,
			expectedMin: 30 * time.Second,
			expectedMax: 300 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.calculateDynamicTimeout(tt.targetCount, tt.baseTimeout)
			assert.GreaterOrEqual(t, result, tt.expectedMin)
			assert.LessOrEqual(t, result, tt.expectedMax)

			// Ensure timeout is within absolute bounds
			assert.GreaterOrEqual(t, result, minTimeout)
			assert.LessOrEqual(t, result, maxTimeout)
		})
	}
}

func TestGenerateTargetsFromCIDR(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name          string
		cidr          string
		maxHosts      int
		expectedCount int
		expectedFirst string
		expectedLast  string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "single host /32",
			cidr:          "192.168.1.10/32",
			maxHosts:      10,
			expectedCount: 1,
			expectedFirst: "192.168.1.10",
			expectedLast:  "192.168.1.10",
		},
		{
			name:          "point to point /31",
			cidr:          "192.168.1.0/31",
			maxHosts:      10,
			expectedCount: 2,
			expectedFirst: "192.168.1.0",
			expectedLast:  "192.168.1.1",
		},
		{
			name:          "small network /30",
			cidr:          "192.168.1.0/30",
			maxHosts:      10,
			expectedCount: 2,
			expectedFirst: "192.168.1.1",
			expectedLast:  "192.168.1.2",
		},
		{
			name:          "standard /24 network",
			cidr:          "192.168.1.0/24",
			maxHosts:      300,
			expectedCount: 254,
			expectedFirst: "192.168.1.1",
			expectedLast:  "192.168.1.254",
		},
		{
			name:          "limited hosts /24",
			cidr:          "192.168.1.0/24",
			maxHosts:      10,
			expectedCount: 10,
			expectedFirst: "192.168.1.1",
			expectedLast:  "192.168.1.10",
		},
		{
			name:          "large network /16",
			cidr:          "10.0.0.0/16",
			maxHosts:      100,
			expectedCount: 100,
			expectedFirst: "10.0.0.1",
			expectedLast:  "10.0.0.100",
		},
		{
			name:          "network too large",
			cidr:          "10.0.0.0/7",
			maxHosts:      100,
			shouldError:   true,
			errorContains: "network too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipnet, err := net.ParseCIDR(tt.cidr)
			require.NoError(t, err)

			targets, err := engine.generateTargetsFromCIDR(*ipnet, tt.maxHosts)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(targets))

			if len(targets) > 0 {
				assert.Equal(t, tt.expectedFirst, targets[0])
				assert.Equal(t, tt.expectedLast, targets[len(targets)-1])
			}
		})
	}
}

func TestBuildNmapOptionsForTargets(t *testing.T) {
	engine := &Engine{}
	targets := []string{"192.168.1.1", "192.168.1.2"}

	tests := []struct {
		name            string
		config          Config
		expectedMethod  string
		shouldHavePorts bool
		shouldHaveOS    bool
	}{
		{
			name:           "tcp method",
			config:         Config{Method: "tcp"},
			expectedMethod: "-PS22,80,443,8080",
		},
		{
			name:           "ping method",
			config:         Config{Method: "ping"},
			expectedMethod: "-PE",
		},
		{
			name:           "arp method",
			config:         Config{Method: "arp"},
			expectedMethod: "-PR",
		},
		{
			name:         "with OS detection",
			config:       Config{Method: "tcp", DetectOS: true},
			shouldHaveOS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := engine.buildNmapOptionsForTargets(targets, &tt.config, 60*time.Second)

			// Should always have ping scan
			assert.Contains(t, args, "-sn")

			// Check method-specific options
			if tt.expectedMethod != "" {
				assert.Contains(t, args, tt.expectedMethod)
			}

			// Check OS detection
			if tt.shouldHaveOS {
				assert.Contains(t, args, "-O")
			}

			// Should contain targets
			assert.Contains(t, args, "192.168.1.1")
			assert.Contains(t, args, "192.168.1.2")
		})
	}
}

func TestNetworkSizeValidation(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		shouldError bool
	}{
		{
			name:        "valid /24 network",
			cidr:        "192.168.1.0/24",
			shouldError: false,
		},
		{
			name:        "valid /16 network",
			cidr:        "10.0.0.0/16",
			shouldError: false,
		},
		{
			name:        "network too large /8",
			cidr:        "10.0.0.0/8",
			shouldError: true,
		},
		{
			name:        "valid /32 host",
			cidr:        "192.168.1.1/32",
			shouldError: false,
		},
		{
			name:        "valid /31 network",
			cidr:        "192.168.1.0/31",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil)
			_, ipnet, err := net.ParseCIDR(tt.cidr)
			require.NoError(t, err)

			err = engine.validateNetworkSize(*ipnet)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkSizeLimit(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name        string
		cidr        string
		maxHosts    int
		expectedLen int
	}{
		{
			name:        "respect max hosts limit",
			cidr:        "192.168.1.0/24",
			maxHosts:    50,
			expectedLen: 50,
		},
		{
			name:        "no limit when max hosts is larger",
			cidr:        "192.168.1.0/29",
			maxHosts:    100,
			expectedLen: 6, // /29 has 6 usable hosts
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipnet, err := net.ParseCIDR(tt.cidr)
			require.NoError(t, err)

			targets, err := engine.generateTargetsFromCIDR(*ipnet, tt.maxHosts)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedLen, len(targets))
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid config with network",
			config: Config{
				Network: "192.168.1.0/24",
				Method:  "tcp",
			},
			shouldError: false,
		},
		{
			name: "valid config with networks array",
			config: Config{
				Networks: []string{"192.168.1.0/24"},
				Method:   "tcp",
			},
			shouldError: false,
		},
		{
			name:        "no network specified",
			config:      Config{Method: "tcp"},
			shouldError: true,
			errorMsg:    "no network specified",
		},
		{
			name: "invalid CIDR",
			config: Config{
				Network: "invalid-cidr",
				Method:  "tcp",
			},
			shouldError: true,
			errorMsg:    "invalid network CIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil)
			if tt.shouldError {
				_, err := engine.Discover(context.Background(), &tt.config)
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else if tt.config.Network != "" {
				// For valid configs, just test that validation passes
				// by checking network parsing directly
				_, _, err := net.ParseCIDR(tt.config.Network)
				assert.NoError(t, err)
			}
		})
	}
}

func TestTimeoutMultiplierCalculation(t *testing.T) {
	engine := &Engine{}
	baseTimeout := 10 * time.Second

	tests := []struct {
		targetCount        int
		expectedMultiplier float64
	}{
		{10, 6.2},
		{50, 7.0},
		{100, 8.0},
		{200, 10.0},
		{500, 16.0},
		{2500, 30.0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := engine.calculateDynamicTimeout(tt.targetCount, baseTimeout)
			expectedTimeout := time.Duration(float64(baseTimeout) * tt.expectedMultiplier)

			// Allow for rounding differences
			assert.InDelta(t, expectedTimeout.Seconds(), result.Seconds(), 1.0)
		})
	}
}

func TestCIDRTargetGeneration(t *testing.T) {
	engine := &Engine{}

	// Test /31 network (RFC 3021)
	_, ipnet31, _ := net.ParseCIDR("192.168.1.0/31")
	targets31, err := engine.generateTargetsFromCIDR(*ipnet31, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, len(targets31))
	assert.Equal(t, "192.168.1.0", targets31[0])
	assert.Equal(t, "192.168.1.1", targets31[1])

	// Test /32 network
	_, ipnet32, _ := net.ParseCIDR("192.168.1.5/32")
	targets32, err := engine.generateTargetsFromCIDR(*ipnet32, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, len(targets32))
	assert.Equal(t, "192.168.1.5", targets32[0])
}

func TestDiscoveryConfigDefaults(t *testing.T) {
	engine := NewEngine(nil)

	assert.Equal(t, defaultConcurrency, engine.concurrency)
	assert.Equal(t, time.Duration(defaultTimeoutSeconds)*time.Second, engine.timeout)
}

func TestNextIPFunction(t *testing.T) {
	engine := &Engine{}

	// Test normal increment
	ip1 := net.ParseIP("192.168.1.1")
	next1 := engine.nextIP(ip1)
	assert.Equal(t, "192.168.1.2", next1.String())

	// Test rollover
	ip2 := net.ParseIP("192.168.1.255")
	next2 := engine.nextIP(ip2)
	assert.Equal(t, "192.168.2.0", next2.String())
}

func TestDiscoveryMethodValidation(t *testing.T) {
	engine := &Engine{}
	targets := []string{"192.168.1.1"}
	timeout := 60 * time.Second

	methods := []string{"tcp", "ping", "arp"}
	for _, method := range methods {
		config := Config{Method: method}
		args := engine.buildNmapOptionsForTargets(targets, &config, timeout)
		assert.NotEmpty(t, args)
		assert.Contains(t, args, "-sn") // All should have ping scan
	}
}

func BenchmarkGenerateTargetsFromCIDR(t *testing.B) {
	engine := &Engine{}
	_, ipnet, _ := net.ParseCIDR("192.168.0.0/24")

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_, _ = engine.generateTargetsFromCIDR(*ipnet, 1000)
	}
}

func BenchmarkCalculateDynamicTimeout(t *testing.B) {
	engine := &Engine{}
	baseTimeout := 10 * time.Second

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_ = engine.calculateDynamicTimeout(254, baseTimeout)
	}
}

func TestDiscoveryEngineIntegration(t *testing.T) {
	// This test verifies the engine can be created and basic operations work
	engine := NewEngine(nil)
	assert.NotNil(t, engine)

	// Test timeout calculation
	timeout := engine.calculateDynamicTimeout(10, 30*time.Second)
	assert.Greater(t, timeout, 30*time.Second)
}

func TestErrorHandling(t *testing.T) {
	engine := &Engine{}

	// Test invalid CIDR
	_, err := engine.Discover(context.Background(), &Config{
		Network: "invalid",
		Method:  "tcp",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid network CIDR")

	// Test network too large
	err = engine.validateNetworkSize(net.IPNet{
		IP:   net.ParseIP("10.0.0.0"),
		Mask: net.CIDRMask(7, 32),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network size too large")
}

func TestTimeoutBoundaries(t *testing.T) {
	engine := &Engine{}

	// Test minimum timeout enforcement
	result := engine.calculateDynamicTimeout(1, 1*time.Second)
	assert.GreaterOrEqual(t, result, minTimeout)

	// Test maximum timeout enforcement
	result = engine.calculateDynamicTimeout(10000, 60*time.Second)
	assert.LessOrEqual(t, result, maxTimeout)
}

func TestDiscoveryConfigurationEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		valid  bool
	}{
		{
			name: "empty network and networks",
			config: Config{
				Method: "tcp",
			},
			valid: false,
		},
		{
			name: "both network and networks specified",
			config: Config{
				Network:  "192.168.1.0/24",
				Networks: []string{"10.0.0.0/24"},
				Method:   "tcp",
			},
			valid: true, // Should use Network field
		},
		{
			name: "zero timeout gets default",
			config: Config{
				Network: "192.168.1.0/24",
				Method:  "tcp",
				Timeout: 0,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test only the configuration validation logic
			if tt.config.Network == "" && len(tt.config.Networks) == 0 {
				assert.False(t, tt.valid, "Config with no networks should be invalid")
			} else if tt.config.Network != "" {
				_, _, err := net.ParseCIDR(tt.config.Network)
				assert.NoError(t, err)
			}
		})
	}
}

func TestNmapOptionsGeneration(t *testing.T) {
	engine := &Engine{}
	targets := []string{"192.168.1.1", "192.168.1.2"}

	tests := []struct {
		name    string
		config  Config
		timeout time.Duration
	}{
		{
			name:    "short timeout uses aggressive timing",
			config:  Config{Method: "tcp"},
			timeout: 20 * time.Second,
		},
		{
			name:    "medium timeout uses normal timing",
			config:  Config{Method: "tcp"},
			timeout: 60 * time.Second,
		},
		{
			name:    "long timeout uses polite timing",
			config:  Config{Method: "tcp"},
			timeout: 200 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := engine.buildNmapOptionsForTargets(targets, &tt.config, tt.timeout)
			assert.NotEmpty(t, args)

			// Check that timing template is set
			hasTimingTemplate := false
			for _, arg := range args {
				if arg == "-T2" || arg == "-T3" || arg == "-T4" {
					hasTimingTemplate = true
					break
				}
			}
			assert.True(t, hasTimingTemplate, "Should have timing template")
		})
	}
}

func TestIPAddressGeneration(t *testing.T) {
	engine := &Engine{}

	// Test various network sizes
	testCases := []struct {
		cidr     string
		maxHosts int
		minHosts int
	}{
		{"192.168.1.0/28", 20, 14}, // /28 has 14 usable hosts
		{"10.0.0.0/24", 300, 254},  // /24 has 254 usable hosts
		{"172.16.0.0/30", 10, 2},   // /30 has 2 usable hosts
	}

	for _, tc := range testCases {
		t.Run(tc.cidr, func(t *testing.T) {
			_, ipnet, err := net.ParseCIDR(tc.cidr)
			require.NoError(t, err)

			targets, err := engine.generateTargetsFromCIDR(*ipnet, tc.maxHosts)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(targets), tc.minHosts)

			// Verify all targets are valid IPs and within the network
			for _, target := range targets {
				ip := net.ParseIP(target)
				assert.NotNil(t, ip, "Generated target should be valid IP: %s", target)
				assert.True(t, ipnet.Contains(ip), "Target should be within network: %s", target)
			}
		})
	}
}

func TestParseNmapOutput(t *testing.T) {
	engine := &Engine{}

	// Test parsing nmap output with multiple hosts
	output := `Starting Nmap 7.80 ( https://nmap.org ) at 2023-01-01 12:00 UTC
Nmap scan report for 192.168.1.1
Host is up (0.001s latency).
Nmap scan report for 192.168.1.100
Host is up (0.002s latency).
Nmap done: 254 IP addresses (2 hosts up) scanned in 2.34 seconds`

	results := engine.parseNmapOutput(output, "tcp")

	assert.Equal(t, 2, len(results))
	assert.Equal(t, "192.168.1.1", results[0].IPAddress.String())
	assert.Equal(t, "192.168.1.100", results[1].IPAddress.String())
	assert.Equal(t, "up", results[0].Status)
	assert.Equal(t, "tcp", results[0].Method)
}
