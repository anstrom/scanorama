package db

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCIDetection verifies that CI environments are properly detected
func TestCIDetection(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectedCI  bool
		description string
	}{
		{
			name: "GitHub Actions CI",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectedCI:  true,
			description: "Should detect GitHub Actions CI environment",
		},
		{
			name: "Generic CI",
			envVars: map[string]string{
				"CI": "true",
			},
			expectedCI:  true,
			description: "Should detect generic CI environment",
		},
		{
			name: "Both CI indicators",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
				"CI":             "true",
			},
			expectedCI:  true,
			description: "Should detect CI when both indicators are present",
		},
		{
			name:        "No CI environment",
			envVars:     map[string]string{},
			expectedCI:  false,
			description: "Should not detect CI in local environment",
		},
		{
			name: "False CI values",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "false",
				"CI":             "false",
			},
			expectedCI:  false,
			description: "Should not detect CI when values are false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			originalEnv := make(map[string]string)
			for key := range tt.envVars {
				originalEnv[key] = os.Getenv(key)
			}

			// Clean up after test
			defer func() {
				for key, originalValue := range originalEnv {
					if originalValue == "" {
						os.Unsetenv(key)
					} else {
						os.Setenv(key, originalValue)
					}
				}
			}()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Test CI detection through configuration
			configs := getTestConfigs()

			if tt.expectedCI {
				// In CI, should have CI-specific config as first option
				require.NotEmpty(t, configs, "Should have configurations in CI")

				// First config should be the CI service container config
				ciConfig := configs[0]
				assert.Equal(t, "localhost", ciConfig.Host, "CI config should use localhost")
				assert.Equal(t, 5432, ciConfig.Port, "CI config should use standard PostgreSQL port")
				assert.Equal(t, "scanorama_test", ciConfig.Database, "CI config should use test database")
				assert.Equal(t, "scanorama_test_user", ciConfig.Username, "CI config should use test user")
				assert.Equal(t, "test_password_123", ciConfig.Password, "CI config should use test password")
				assert.Equal(t, "disable", ciConfig.SSLMode, "CI config should disable SSL")
			} else if len(configs) > 0 {
				// In non-CI, should not start with CI-specific config
				firstConfig := configs[0]
				// Should not be the hardcoded CI config
				isNotCIConfig := firstConfig.Port != 5432 ||
					firstConfig.Username != "scanorama_test_user" ||
					firstConfig.Password != "test_password_123"
				assert.True(t, isNotCIConfig, "Non-CI environment should not prioritize CI config")
			}
		})
	}
}

// TestCIConfigurationPriority verifies that CI configurations take priority
func TestCIConfigurationPriority(t *testing.T) {
	// Save original environment
	originalGitHubActions := os.Getenv("GITHUB_ACTIONS")
	originalCI := os.Getenv("CI")
	originalTestHost := os.Getenv("TEST_DB_HOST")
	originalTestPort := os.Getenv("TEST_DB_PORT")

	defer func() {
		// Restore original environment
		if originalGitHubActions == "" {
			os.Unsetenv("GITHUB_ACTIONS")
		} else {
			os.Setenv("GITHUB_ACTIONS", originalGitHubActions)
		}
		if originalCI == "" {
			os.Unsetenv("CI")
		} else {
			os.Setenv("CI", originalCI)
		}
		if originalTestHost == "" {
			os.Unsetenv("TEST_DB_HOST")
		} else {
			os.Setenv("TEST_DB_HOST", originalTestHost)
		}
		if originalTestPort == "" {
			os.Unsetenv("TEST_DB_PORT")
		} else {
			os.Setenv("TEST_DB_PORT", originalTestPort)
		}
	}()

	// Set CI environment with conflicting environment variables
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("CI", "true")
	os.Setenv("TEST_DB_HOST", "conflicting-host")
	os.Setenv("TEST_DB_PORT", "9999")

	configs := getTestConfigs()
	require.NotEmpty(t, configs, "Should have configurations")

	// First config should still be the CI service container config, not the env vars
	ciConfig := configs[0]
	assert.Equal(t, "localhost", ciConfig.Host, "CI config should override env vars for host")
	assert.Equal(t, 5432, ciConfig.Port, "CI config should override env vars for port")
	assert.Equal(t, "scanorama_test_user", ciConfig.Username, "CI config should use service container user")
}

// TestNonCIEnvironmentVariables verifies env vars work in non-CI environments
func TestNonCIEnvironmentVariables(t *testing.T) {
	// Save original environment
	originalGitHubActions := os.Getenv("GITHUB_ACTIONS")
	originalCI := os.Getenv("CI")
	originalTestHost := os.Getenv("TEST_DB_HOST")
	originalTestPort := os.Getenv("TEST_DB_PORT")

	defer func() {
		// Restore original environment
		if originalGitHubActions == "" {
			os.Unsetenv("GITHUB_ACTIONS")
		} else {
			os.Setenv("GITHUB_ACTIONS", originalGitHubActions)
		}
		if originalCI == "" {
			os.Unsetenv("CI")
		} else {
			os.Setenv("CI", originalCI)
		}
		if originalTestHost == "" {
			os.Unsetenv("TEST_DB_HOST")
		} else {
			os.Setenv("TEST_DB_HOST", originalTestHost)
		}
		if originalTestPort == "" {
			os.Unsetenv("TEST_DB_PORT")
		} else {
			os.Setenv("TEST_DB_PORT", originalTestPort)
		}
	}()

	// Ensure we're not in CI
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("CI")

	// Set environment variables
	os.Setenv("TEST_DB_HOST", "custom-host")
	os.Setenv("TEST_DB_PORT", "5555")

	configs := getTestConfigs()
	require.NotEmpty(t, configs, "Should have configurations")

	// Should find a config that uses the environment variables
	var foundCustomConfig bool
	for _, config := range configs {
		if config.Host == "custom-host" && config.Port == 5555 {
			foundCustomConfig = true
			break
		}
	}
	assert.True(t, foundCustomConfig, "Should use environment variables in non-CI environment")
}

// TestConfigurationDefaults verifies default values are applied correctly
func TestConfigurationDefaults(t *testing.T) {
	// Save original environment
	originalGitHubActions := os.Getenv("GITHUB_ACTIONS")
	originalCI := os.Getenv("CI")

	defer func() {
		// Restore original environment
		if originalGitHubActions == "" {
			os.Unsetenv("GITHUB_ACTIONS")
		} else {
			os.Setenv("GITHUB_ACTIONS", originalGitHubActions)
		}
		if originalCI == "" {
			os.Unsetenv("CI")
		} else {
			os.Setenv("CI", originalCI)
		}
	}()

	// Ensure we're not in CI
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("CI")

	configs := getTestConfigs()
	require.NotEmpty(t, configs, "Should have configurations")

	// All configs should have reasonable defaults
	for i, config := range configs {
		assert.NotEmpty(t, config.Host, "Config %d should have host", i)
		assert.Greater(t, config.Port, 0, "Config %d should have valid port", i)
		assert.NotEmpty(t, config.Database, "Config %d should have database name", i)
		assert.NotEmpty(t, config.Username, "Config %d should have username", i)
		assert.Equal(t, "disable", config.SSLMode, "Config %d should disable SSL for tests", i)
		assert.Greater(t, config.MaxOpenConns, 0, "Config %d should have max open connections", i)
		assert.Greater(t, config.MaxIdleConns, 0, "Config %d should have max idle connections", i)
		assert.Greater(t, config.ConnMaxLifetime, time.Duration(0), "Config %d should have connection lifetime", i)
	}
}

// TestDebugOutput verifies debug output works when enabled
func TestDebugOutput(t *testing.T) {
	// Save original environment
	originalDBDebug := os.Getenv("DB_DEBUG")
	originalGitHubActions := os.Getenv("GITHUB_ACTIONS")

	defer func() {
		// Restore original environment
		if originalDBDebug == "" {
			os.Unsetenv("DB_DEBUG")
		} else {
			os.Setenv("DB_DEBUG", originalDBDebug)
		}
		if originalGitHubActions == "" {
			os.Unsetenv("GITHUB_ACTIONS")
		} else {
			os.Setenv("GITHUB_ACTIONS", originalGitHubActions)
		}
	}()

	// Enable debug mode and CI
	os.Setenv("DB_DEBUG", "true")
	os.Setenv("GITHUB_ACTIONS", "true")

	// This should not panic and should produce debug output
	// We can't easily capture the output in this test, but we can ensure it doesn't crash
	configs := getTestConfigs()
	assert.NotEmpty(t, configs, "Should produce configurations even with debug enabled")
}
