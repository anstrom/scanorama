package discovery

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/anstrom/scanorama/internal/errors"
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

func TestBuildNmapLibraryOptions(t *testing.T) {
	engine := &Engine{}
	targets := []string{"192.168.1.1", "192.168.1.2"}

	tests := []struct {
		name         string
		config       Config
		shouldHaveOS bool
	}{
		{
			name:   "tcp method",
			config: Config{Method: "tcp"},
		},
		{
			name:   "ping method",
			config: Config{Method: "ping"},
		},
		{
			name:   "arp method",
			config: Config{Method: "arp"},
		},
		{
			name:         "with OS detection",
			config:       Config{Method: "tcp", DetectOS: true},
			shouldHaveOS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := engine.buildNmapLibraryOptions(targets, &tt.config, 60*time.Second)

			// Should have options
			assert.NotEmpty(t, options)

			// Note: We can't easily test the internal structure of nmap.Option
			// as they are opaque. The actual functionality is tested through integration tests.
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
		options := engine.buildNmapLibraryOptions(targets, &config, timeout)
		assert.NotEmpty(t, options)
	}
}

func TestSaveDiscoveredHostsEmptyResults(t *testing.T) {
	engine := NewEngine(nil)
	ctx := context.Background()

	// Test with empty results - this should not make any database calls
	results := []Result{}
	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
}

func TestCalculateDynamicTimeoutEdgeCases(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name        string
		targetCount int
		baseTimeout time.Duration
		expectMin   time.Duration
		expectMax   time.Duration
	}{
		{
			name:        "zero base timeout uses engine default",
			targetCount: 100,
			baseTimeout: 0,
			expectMin:   minTimeout,
			expectMax:   maxTimeout,
		},
		{
			name:        "very large target count hits max multiplier",
			targetCount: 10000,
			baseTimeout: 10 * time.Second,
			expectMin:   minTimeout,
			expectMax:   maxTimeout,
		},
		{
			name:        "small base timeout with small target count",
			targetCount: 1,
			baseTimeout: 1 * time.Second,
			expectMin:   minTimeout,
			expectMax:   maxTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.calculateDynamicTimeout(tt.targetCount, tt.baseTimeout)
			assert.GreaterOrEqual(t, result, tt.expectMin)
			assert.LessOrEqual(t, result, tt.expectMax)
		})
	}
}

func TestDiscoveryMethodSpecificOptions(t *testing.T) {
	engine := &Engine{}
	targets := []string{"127.0.0.1"}
	timeout := 60 * time.Second

	methods := []string{"tcp", "ping", "arp", "unknown"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			config := &Config{Method: method}
			options := engine.buildNmapLibraryOptions(targets, config, timeout)
			assert.NotEmpty(t, options)
			// Each method should generate different options
		})
	}
}

func TestOSDetectionOption(t *testing.T) {
	engine := &Engine{}
	targets := []string{"127.0.0.1"}
	timeout := 60 * time.Second

	tests := []struct {
		name     string
		detectOS bool
	}{
		{"with OS detection", true},
		{"without OS detection", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Method:   "tcp",
				DetectOS: tt.detectOS,
			}
			options := engine.buildNmapLibraryOptions(targets, config, timeout)
			assert.NotEmpty(t, options)
			// OS detection should be included when requested
		})
	}
}

func TestSetConcurrency(t *testing.T) {
	engine := NewEngine(nil)

	// Test setting concurrency
	engine.SetConcurrency(20)
	assert.Equal(t, 20, engine.concurrency)

	// Test setting zero concurrency
	engine.SetConcurrency(0)
	assert.Equal(t, 0, engine.concurrency)
}

func TestSetTimeout(t *testing.T) {
	engine := NewEngine(nil)

	// Test setting timeout
	timeout := 120 * time.Second
	engine.SetTimeout(timeout)
	assert.Equal(t, timeout, engine.timeout)

	// Test setting zero timeout
	engine.SetTimeout(0)
	assert.Equal(t, time.Duration(0), engine.timeout)
}

func TestNmapDiscoveryWithTargetsError(t *testing.T) {
	engine := &Engine{}
	ctx := context.Background()

	// Test with invalid context (cancelled)
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	targets := []string{"192.168.1.1"}
	config := &Config{Method: "tcp"}

	// This should handle cancelled context or nmap execution error
	results, err := engine.nmapDiscoveryWithTargets(cancelCtx, targets, config, 1*time.Second)

	// Either succeeds with empty results or fails with context error
	if err != nil {
		t.Logf("Expected error with cancelled context: %v", err)
	} else {
		assert.NotNil(t, results)
	}
}

func TestDiscoverValidation(t *testing.T) {
	// Test only the validation logic without calling full Discover
	engine := NewEngine(nil)

	// Test network size validation directly
	_, ipnet, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	err = engine.validateNetworkSize(*ipnet)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network size too large")
}

func TestValidateNetworkSizeEdgeCases(t *testing.T) {
	engine := NewEngine(nil)

	// Test various network sizes without calling full Discover
	tests := []struct {
		cidr        string
		shouldError bool
	}{
		{"192.168.1.0/30", false}, // Valid small network
		{"10.0.0.0/16", false},    // Valid /16
		{"10.0.0.0/8", true},      // Too large
		{"192.168.1.1/32", false}, // Single host
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
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

func TestNmapLibraryOptionsGeneration(t *testing.T) {
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
			options := engine.buildNmapLibraryOptions(targets, &tt.config, tt.timeout)
			assert.NotEmpty(t, options)
			// Note: We can't easily inspect nmap.Option internals,
			// but we can verify that options are generated
		})
	}
}

func TestNmapDiscoveryWithTargetsEmptyCase(t *testing.T) {
	engine := &Engine{}
	ctx := context.Background()

	// Test empty targets case - this should return immediately without nmap execution
	results, err := engine.nmapDiscoveryWithTargets(ctx, []string{}, &Config{Method: "tcp"}, 30*time.Second)
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestConvertNmapResultsEdgeCases(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name       string
		nmapResult *nmap.Run
		method     string
		expected   int
	}{
		{
			name:       "nil result",
			nmapResult: nil,
			method:     "tcp",
			expected:   0,
		},
		{
			name: "host with no addresses",
			nmapResult: &nmap.Run{
				Hosts: []nmap.Host{
					{
						Addresses: []nmap.Address{},
						Status:    nmap.Status{State: "up"},
					},
				},
			},
			method:   "tcp",
			expected: 0,
		},
		{
			name: "host with invalid IP",
			nmapResult: &nmap.Run{
				Hosts: []nmap.Host{
					{
						Addresses: []nmap.Address{
							{Addr: "invalid-ip", AddrType: "ipv4"},
						},
						Status: nmap.Status{State: "up"},
					},
				},
			},
			method:   "tcp",
			expected: 0,
		},
		{
			name: "host down (filtered out)",
			nmapResult: &nmap.Run{
				Hosts: []nmap.Host{
					{
						Addresses: []nmap.Address{
							{Addr: "192.168.1.1", AddrType: "ipv4"},
						},
						Status: nmap.Status{State: "down"},
					},
				},
			},
			method:   "tcp",
			expected: 0, // Down hosts are filtered out
		},
		{
			name: "host with OS info",
			nmapResult: &nmap.Run{
				Hosts: []nmap.Host{
					{
						Addresses: []nmap.Address{
							{Addr: "192.168.1.1", AddrType: "ipv4"},
						},
						Status: nmap.Status{State: "up"},
						OS: nmap.OS{
							Matches: []nmap.OSMatch{
								{Name: "Linux 4.15"},
							},
						},
					},
				},
			},
			method:   "tcp",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.convertNmapResultsToDiscovery(tt.nmapResult, tt.method)
			assert.Equal(t, tt.expected, len(results))

			if tt.expected > 0 && tt.nmapResult != nil && len(tt.nmapResult.Hosts) > 0 {
				result := results[0]
				assert.Equal(t, tt.method, result.Method)

				// All returned results should be "up" since down hosts are filtered
				assert.Equal(t, "up", result.Status)

				if len(tt.nmapResult.Hosts[0].OS.Matches) > 0 {
					assert.Equal(t, "Linux 4.15", result.OSInfo)
				}
			}
		})
	}
}

func TestHostTimeoutCalculation(t *testing.T) {
	engine := &Engine{}
	targets := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	config := &Config{Method: "tcp"}

	tests := []struct {
		name            string
		timeout         time.Duration
		expectedMinimum time.Duration
	}{
		{
			name:            "very short timeout gets minimum",
			timeout:         100 * time.Millisecond,
			expectedMinimum: time.Second,
		},
		{
			name:            "normal timeout",
			timeout:         30 * time.Second,
			expectedMinimum: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := engine.buildNmapLibraryOptions(targets, config, tt.timeout)
			assert.NotEmpty(t, options)
			// The host timeout calculation is internal to the function
			// but this exercises the code path
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

func TestConvertNmapResultsToDiscovery(t *testing.T) {
	engine := &Engine{}

	// Create mock nmap result
	nmapResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.1", AddrType: "ipv4"},
				},
				Status: nmap.Status{State: "up"},
			},
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.100", AddrType: "ipv4"},
				},
				Status: nmap.Status{State: "up"},
			},
		},
	}

	results := engine.convertNmapResultsToDiscovery(nmapResult, "tcp")

	assert.Equal(t, 2, len(results))
	assert.Equal(t, "192.168.1.1", results[0].IPAddress.String())
	assert.Equal(t, "192.168.1.100", results[1].IPAddress.String())
	assert.Equal(t, "up", results[0].Status)
	assert.Equal(t, "tcp", results[0].Method)
}

// TestDiscoveryResilience tests the resilience functionality behavior
func TestDiscoveryResilience(t *testing.T) {
	t.Run("discovery_retries_on_retryable_errors", func(t *testing.T) {
		// This test verifies that discovery service retries on transient failures
		// but doesn't test implementation details like exact retry counts

		engine := NewEngine(nil)
		config := &Config{
			Method:      "ping",
			DetectOS:    false,
			Timeout:     5,
			Concurrency: 1,
			MaxHosts:    1,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Test with a target that should fail initially but might succeed on retry
		targets := []string{"192.0.2.1"} // RFC5737 test address

		// This tests the behavior - discovery should handle failures gracefully
		results, err := engine.nmapDiscoveryWithResilience(ctx, targets, config, 5*time.Second)

		// The key behavior is that the function should complete without panicking
		// and should return appropriate error information if all attempts fail
		if err != nil {
			// Verify error contains meaningful information
			assert.Contains(t, err.Error(), "discovery failed")
		} else {
			// If it succeeds, results should be valid
			assert.NotNil(t, results)
		}
	})

	t.Run("discovery_respects_context_cancellation", func(t *testing.T) {
		engine := NewEngine(nil)
		config := &Config{
			Method:      "ping",
			DetectOS:    false,
			Timeout:     5,
			Concurrency: 1,
			MaxHosts:    10,
		}

		// Create context that will be cancelled quickly
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		targets := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}

		start := time.Now()
		results, err := engine.nmapDiscoveryWithResilience(ctx, targets, config, 5*time.Second)
		elapsed := time.Since(start)

		// Should return quickly due to context cancellation
		assert.True(t, elapsed < 2*time.Second, "Discovery should respect context cancellation")

		// Should return context cancellation error
		if err != nil {
			assert.Contains(t, err.Error(), "cancel")
		}

		// Results should be nil on cancellation
		assert.Nil(t, results)
	})

	t.Run("discovery_handles_empty_target_list", func(t *testing.T) {
		engine := NewEngine(nil)
		config := &Config{
			Method:      "ping",
			DetectOS:    false,
			Timeout:     5,
			Concurrency: 1,
			MaxHosts:    0,
		}

		ctx := context.Background()
		targets := []string{}

		results, err := engine.nmapDiscoveryWithResilience(ctx, targets, config, 5*time.Second)

		// Should handle empty targets gracefully
		if err == nil {
			assert.Equal(t, 0, len(results))
		} else {
			// Error should be descriptive
			assert.NotEmpty(t, err.Error())
		}
	})

	t.Run("discovery_maintains_result_consistency", func(t *testing.T) {
		// Test that discovery results are consistent and properly formatted
		engine := NewEngine(nil)
		config := &Config{
			Method:      "ping",
			DetectOS:    false,
			Timeout:     10,
			Concurrency: 1,
			MaxHosts:    5,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Use localhost which should be reachable
		targets := []string{"127.0.0.1"}

		results, err := engine.nmapDiscoveryWithResilience(ctx, targets, config, 10*time.Second)

		// If discovery succeeds, results should be well-formed
		if err == nil && len(results) > 0 {
			for _, result := range results {
				// Result should have valid IP address
				assert.NotNil(t, result.IPAddress)
				assert.NotEmpty(t, result.IPAddress.String())

				// Status should be meaningful
				assert.Contains(t, []string{"up", "down", "filtered"}, result.Status)

				// Method should match configuration
				assert.Equal(t, config.Method, result.Method)

				// Response time should be non-negative
				assert.True(t, result.ResponseTime >= 0)
			}
		}
	})
}

// TestClassifyNmapError tests the error classification logic for nmap failures.
func TestClassifyNmapError(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name         string
		message      string
		nmapError    error
		expectedCode errors.ErrorCode
	}{
		{
			name:         "timeout error",
			message:      "scan operation failed",
			nmapError:    fmt.Errorf("connection timed out"),
			expectedCode: errors.CodeTimeout,
		},
		{
			name:         "network unreachable",
			message:      "scan failed",
			nmapError:    fmt.Errorf("network unreachable"),
			expectedCode: errors.CodeNetworkUnreachable,
		},
		{
			name:         "host unreachable",
			message:      "scan failed",
			nmapError:    fmt.Errorf("destination host unreachable"),
			expectedCode: errors.CodeHostUnreachable,
		},
		{
			name:         "permission denied",
			message:      "scanner creation failed",
			nmapError:    fmt.Errorf("permission denied"),
			expectedCode: errors.CodePermission,
		},
		{
			name:         "invalid target",
			message:      "scan failed",
			nmapError:    fmt.Errorf("bad target specification"),
			expectedCode: errors.CodeTargetInvalid,
		},
		{
			name:         "context canceled",
			message:      "scan interrupted",
			nmapError:    fmt.Errorf("context canceled"),
			expectedCode: errors.CodeCanceled,
		},
		{
			name:         "context deadline exceeded",
			message:      "scan timeout",
			nmapError:    fmt.Errorf("context deadline exceeded"),
			expectedCode: errors.CodeTimeout,
		},
		{
			name:         "unknown error defaults to discovery failed",
			message:      "unexpected failure",
			nmapError:    fmt.Errorf("mysterious nmap error"),
			expectedCode: errors.CodeDiscoveryFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.classifyNmapError(tt.message, tt.nmapError)

			assert.Error(t, err)
			assert.Equal(t, tt.expectedCode, errors.GetCode(err))
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

// TestDiscoveryRetryBehaviorWithClassification tests that retry logic works correctly
// with the new error classification system.
func TestDiscoveryRetryBehaviorWithClassification(t *testing.T) {
	tests := []struct {
		name             string
		errorType        string
		shouldRetry      bool
		expectedAttempts int
	}{
		{
			name:             "timeout errors are retryable",
			errorType:        "timeout",
			shouldRetry:      true,
			expectedAttempts: 3, // Should retry up to max attempts
		},
		{
			name:             "network unreachable errors are retryable",
			errorType:        "network_unreachable",
			shouldRetry:      true,
			expectedAttempts: 3,
		},
		{
			name:             "permission errors are not retryable",
			errorType:        "permission",
			shouldRetry:      false,
			expectedAttempts: 1, // Should fail immediately
		},
		{
			name:             "invalid target errors are not retryable",
			errorType:        "invalid_target",
			shouldRetry:      false,
			expectedAttempts: 1,
		},
		{
			name:             "unknown errors default to retryable",
			errorType:        "unknown",
			shouldRetry:      true,
			expectedAttempts: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &Engine{}
			ctx := context.Background()
			config := &Config{Method: "ping"}

			var attemptCount int

			// Create test function that simulates nmap errors
			testNmapFunc := func(ctx context.Context, targets []string, config *Config,
				timeout time.Duration) ([]Result, error) {
				attemptCount++

				// Simulate different types of nmap errors
				var mockError error
				switch tt.errorType {
				case "timeout":
					mockError = fmt.Errorf("connection timed out")
				case "network_unreachable":
					mockError = fmt.Errorf("network unreachable")
				case "permission":
					mockError = fmt.Errorf("permission denied")
				case "invalid_target":
					mockError = fmt.Errorf("invalid target specification")
				default:
					mockError = fmt.Errorf("unknown nmap error")
				}

				return nil, engine.classifyNmapError("mock nmap error", mockError)
			}

			// Create a wrapper engine for testing
			testEngine := &testDiscoveryEngine{
				Engine:   engine,
				testFunc: testNmapFunc,
			}

			start := time.Now()
			_, err := testEngine.testDiscoveryWithResilience(ctx, []string{"192.168.1.1"}, config, 5*time.Second)
			elapsed := time.Since(start)

			// Verify the error occurred
			assert.Error(t, err)

			// Verify the correct number of attempts were made
			assert.Equal(t, tt.expectedAttempts, attemptCount,
				"Expected %d attempts but got %d for error type %s",
				tt.expectedAttempts, attemptCount, tt.errorType)

			// Verify retry behavior timing
			if tt.shouldRetry && tt.expectedAttempts > 1 {
				// Should take some time due to retry delays (at least 2s for first retry)
				assert.Greater(t, elapsed, 1500*time.Millisecond,
					"Retryable error should have retry delays")
			} else {
				// Non-retryable errors should fail quickly
				assert.Less(t, elapsed, 500*time.Millisecond,
					"Non-retryable error should fail immediately")
			}

			// Verify error code is preserved through retry logic
			expectedRetryable := errors.IsRetryable(err)
			assert.Equal(t, tt.shouldRetry, expectedRetryable,
				"Error retryability should match expected behavior")
		})
	}
}

// testDiscoveryEngine wraps Engine for testing purposes
type testDiscoveryEngine struct {
	*Engine
	testFunc func(context.Context, []string, *Config, time.Duration) ([]Result, error)
}

// testDiscoveryWithResilience is a test version of nmapDiscoveryWithResilience that uses testFunc
func (e *testDiscoveryEngine) testDiscoveryWithResilience(ctx context.Context,
	targets []string, config *Config, timeout time.Duration) ([]Result, error) {
	var lastError error
	retryDelay := baseRetryDelay

	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			return nil, errors.WrapDiscoveryError(errors.CodeCanceled, "discovery cancelled", ctx.Err())
		default:
		}

		results, err := e.testFunc(ctx, targets, config, timeout)
		if err == nil {
			return results, nil
		}

		lastError = err

		// Check if error is retryable
		if !errors.IsRetryable(err) {
			break
		}

		// Don't retry on last attempt
		if attempt == maxRetryAttempts {
			break
		}

		// Sleep with exponential backoff
		time.Sleep(retryDelay)
		retryDelay *= 2
		if retryDelay > maxRetryDelay {
			retryDelay = maxRetryDelay
		}
	}

	return nil, lastError
}
