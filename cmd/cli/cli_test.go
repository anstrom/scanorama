package cli

import (
	"testing"
	"time"
)

func TestValidatePorts(t *testing.T) {
	tests := []struct {
		name    string
		ports   string
		wantErr bool
	}{
		{
			name:    "valid single port",
			ports:   "80",
			wantErr: false,
		},
		{
			name:    "valid port list",
			ports:   "80,443,8080",
			wantErr: false,
		},
		{
			name:    "valid port range",
			ports:   "80-443",
			wantErr: false,
		},
		{
			name:    "valid top ports",
			ports:   "T:100",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			ports:   "22,80-443,8080",
			wantErr: false,
		},
		{
			name:    "empty string",
			ports:   "",
			wantErr: true,
		},
		{
			name:    "invalid port - too high",
			ports:   "65536",
			wantErr: true,
		},
		{
			name:    "invalid port - zero",
			ports:   "0",
			wantErr: true,
		},
		{
			name:    "invalid port - negative",
			ports:   "-1",
			wantErr: true,
		},
		{
			name:    "invalid range - reversed",
			ports:   "443-80",
			wantErr: true,
		},
		{
			name:    "invalid range - too many parts",
			ports:   "80-443-8080",
			wantErr: true,
		},
		{
			name:    "invalid characters",
			ports:   "80,abc,443",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePorts(tt.ports)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePorts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name     string
		targets  string
		expected []string
	}{
		{
			name:     "empty string",
			targets:  "",
			expected: nil,
		},
		{
			name:     "single IP",
			targets:  "192.168.1.1",
			expected: []string{"192.168.1.1"},
		},
		{
			name:     "multiple IPs",
			targets:  "192.168.1.1,192.168.1.2",
			expected: []string{"192.168.1.1", "192.168.1.2"},
		},
		{
			name:     "IPs with spaces",
			targets:  "192.168.1.1, 192.168.1.2 , 192.168.1.3",
			expected: []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name:     "localhost",
			targets:  "localhost",
			expected: []string{"localhost"},
		},
		{
			name:     "IP range",
			targets:  "192.168.1.1-10",
			expected: []string{"192.168.1.1-10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTargets(tt.targets)
			if len(result) != len(tt.expected) {
				t.Errorf("parseTargets() length = %v, want %v", len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseTargets()[%d] = %v, want %v", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{
			name:    "valid IP",
			ip:      "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "valid IP - localhost",
			ip:      "127.0.0.1",
			wantErr: false,
		},
		{
			name:    "valid IP - edge case",
			ip:      "255.255.255.255",
			wantErr: false,
		},
		{
			name:    "valid IP - zero",
			ip:      "0.0.0.0",
			wantErr: false,
		},
		{
			name:    "invalid IP - too many octets",
			ip:      "192.168.1.1.1",
			wantErr: true,
		},
		{
			name:    "invalid IP - too few octets",
			ip:      "192.168.1",
			wantErr: true,
		},
		{
			name:    "invalid IP - octet too high",
			ip:      "192.168.1.256",
			wantErr: true,
		},
		{
			name:    "invalid IP - negative octet",
			ip:      "192.168.1.-1",
			wantErr: true,
		},
		{
			name:    "invalid IP - non-numeric",
			ip:      "192.168.1.abc",
			wantErr: true,
		},
		{
			name:    "empty string",
			ip:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "hours",
			duration: "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "minutes",
			duration: "30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "days",
			duration: "7d",
			expected: 7 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "single day",
			duration: "1d",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "seconds",
			duration: "30s",
			expected: 30 * time.Second,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			duration: "abc",
			wantErr:  true,
		},
		{
			name:     "invalid day format",
			duration: "abcd",
			wantErr:  true,
		},
		{
			name:     "empty string",
			duration: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.duration)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("parseDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidatePortPart(t *testing.T) {
	tests := []struct {
		name    string
		part    string
		wantErr bool
	}{
		{
			name:    "valid single port",
			part:    "80",
			wantErr: false,
		},
		{
			name:    "valid port range",
			part:    "80-443",
			wantErr: false,
		},
		{
			name:    "invalid port - too high",
			part:    "65536",
			wantErr: true,
		},
		{
			name:    "invalid range - reversed",
			part:    "443-80",
			wantErr: true,
		},
		{
			name:    "invalid format",
			part:    "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePortPart(tt.part)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePortPart() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		name     string
		portStr  string
		expected int
		wantErr  bool
	}{
		{
			name:     "valid port",
			portStr:  "80",
			expected: 80,
			wantErr:  false,
		},
		{
			name:     "valid port with spaces",
			portStr:  " 443 ",
			expected: 443,
			wantErr:  false,
		},
		{
			name:     "max valid port",
			portStr:  "65535",
			expected: 65535,
			wantErr:  false,
		},
		{
			name:     "min valid port",
			portStr:  "1",
			expected: 1,
			wantErr:  false,
		},
		{
			name:    "port too high",
			portStr: "65536",
			wantErr: true,
		},
		{
			name:    "port zero",
			portStr: "0",
			wantErr: true,
		},
		{
			name:    "negative port",
			portStr: "-1",
			wantErr: true,
		},
		{
			name:    "non-numeric",
			portStr: "abc",
			wantErr: true,
		},
		{
			name:    "empty string",
			portStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePort(tt.portStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("parsePort() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "minutes",
			duration: 30 * time.Minute,
			expected: "30m",
		},
		{
			name:     "hours",
			duration: 2 * time.Hour,
			expected: "2.0h",
		},
		{
			name:     "days",
			duration: 3 * 24 * time.Hour,
			expected: "3d",
		},
		{
			name:     "less than minute",
			duration: 30 * time.Second,
			expected: "0m",
		},
		{
			name:     "exactly one day",
			duration: 24 * time.Hour,
			expected: "1d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}
