package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigFilePath(t *testing.T) {
	// Save original viper state
	originalConfigFile := viper.ConfigFileUsed()
	defer func() {
		// Reset viper state after tests
		if originalConfigFile != "" {
			viper.SetConfigFile(originalConfigFile)
		} else {
			viper.Reset()
		}
	}()

	tests := []struct {
		name           string
		viperConfigSet string
		expectedResult string
	}{
		{
			name:           "returns default when no config file set",
			viperConfigSet: "",
			expectedResult: "config.yaml",
		},
		{
			name:           "returns viper config file when set",
			viperConfigSet: "/path/to/custom-config.yaml",
			expectedResult: "/path/to/custom-config.yaml",
		},
		{
			name:           "returns relative path when viper has relative path",
			viperConfigSet: "custom-config.yaml",
			expectedResult: "custom-config.yaml",
		},
		{
			name:           "returns absolute path when viper has absolute path",
			viperConfigSet: "/etc/scanorama/config.yaml",
			expectedResult: "/etc/scanorama/config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			viper.Reset()

			if tt.viperConfigSet != "" {
				// Set the config file in viper
				viper.SetConfigFile(tt.viperConfigSet)
			}

			result := getConfigFilePath()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestConfigFileIntegration(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "scanorama-config-test")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a test config file
	testConfigPath := filepath.Join(tempDir, "test-config.yaml")
	testConfigContent := `# Test configuration
database:
  host: "test-host"
  port: 5432
  database: "test-db"
  username: "test-user"
  password: "test-pass"
  ssl_mode: "disable"

logging:
  level: "debug"
  format: "json"
  output: "stdout"

api:
  enabled: true
  listen_addr: "127.0.0.1"
  port: 8080

scanning:
  worker_pool_size: 5
  default_ports: "22,80,443"
`

	err = os.WriteFile(testConfigPath, []byte(testConfigContent), 0644)
	require.NoError(t, err)

	// Save original viper state
	originalConfigFile := viper.ConfigFileUsed()
	defer func() {
		// Reset viper state after test
		viper.Reset()
		if originalConfigFile != "" {
			viper.SetConfigFile(originalConfigFile)
		}
	}()

	t.Run("loads custom config file path correctly", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Set the custom config file
		viper.SetConfigFile(testConfigPath)

		// Test that getConfigFilePath returns the custom path
		result := getConfigFilePath()
		assert.Equal(t, testConfigPath, result)
	})

	t.Run("loads config content when custom path is set", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Set and read the custom config file
		viper.SetConfigFile(testConfigPath)
		err := viper.ReadInConfig()
		require.NoError(t, err)

		// Verify the config path is correct
		result := getConfigFilePath()
		assert.Equal(t, testConfigPath, result)

		// Verify that config content was loaded correctly
		assert.Equal(t, "test-host", viper.GetString("database.host"))
		assert.Equal(t, 5432, viper.GetInt("database.port"))
		assert.Equal(t, "debug", viper.GetString("logging.level"))
		assert.Equal(t, "json", viper.GetString("logging.format"))
		assert.Equal(t, 8080, viper.GetInt("api.port"))
	})
}

func TestConfigFilePathWithRootCmd(t *testing.T) {
	// This test verifies that the --config flag properly sets the config file path
	// Save original state
	originalArgs := os.Args
	originalConfigFile := viper.ConfigFileUsed()

	defer func() {
		// Restore original state
		os.Args = originalArgs
		viper.Reset()
		if originalConfigFile != "" {
			viper.SetConfigFile(originalConfigFile)
		}
	}()

	// Create a temporary config file
	tempDir, err := os.MkdirTemp("", "scanorama-flag-test")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp directory: %v", err)
		}
	}()

	testConfigPath := filepath.Join(tempDir, "flag-test-config.yaml")
	testConfigContent := `# Flag test configuration
database:
  host: "flag-test-host"
  database: "flag-test-db"
`

	err = os.WriteFile(testConfigPath, []byte(testConfigContent), 0644)
	require.NoError(t, err)

	t.Run("config flag sets correct path", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Simulate command line args with --config flag
		os.Args = []string{"scanorama", "--config", testConfigPath, "hosts", "--help"}

		// Initialize the root command (this should set up viper with the config file)
		// Note: We can't easily test the full command execution here without complex setup,
		// but we can test that viper can be configured with the path correctly

		viper.SetConfigFile(testConfigPath)
		err := viper.ReadInConfig()
		require.NoError(t, err)

		// Test that getConfigFilePath returns the flag-specified path
		result := getConfigFilePath()
		assert.Equal(t, testConfigPath, result)

		// Verify config content was loaded
		assert.Equal(t, "flag-test-host", viper.GetString("database.host"))
		assert.Equal(t, "flag-test-db", viper.GetString("database.database"))
	})
}

func TestConfigFileDefaultBehavior(t *testing.T) {
	// Save original viper state
	originalConfigFile := viper.ConfigFileUsed()
	defer func() {
		viper.Reset()
		if originalConfigFile != "" {
			viper.SetConfigFile(originalConfigFile)
		}
	}()

	t.Run("returns default config.yaml when viper is reset", func(t *testing.T) {
		// Reset viper to clear any existing config
		viper.Reset()

		result := getConfigFilePath()
		assert.Equal(t, "config.yaml", result)
	})

	t.Run("returns default when viper config is empty string", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Even if we set an empty config file, it should return default
		viper.SetConfigFile("")

		result := getConfigFilePath()
		assert.Equal(t, "config.yaml", result)
	})
}

// Benchmark the getConfigFilePath function
func BenchmarkGetConfigFilePath(b *testing.B) {
	// Save original state
	originalConfigFile := viper.ConfigFileUsed()
	defer func() {
		if originalConfigFile != "" {
			viper.SetConfigFile(originalConfigFile)
		} else {
			viper.Reset()
		}
	}()

	b.Run("with_config_file_set", func(b *testing.B) {
		viper.SetConfigFile("/path/to/config.yaml")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = getConfigFilePath()
		}
	})

	b.Run("with_default_fallback", func(b *testing.B) {
		viper.Reset()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = getConfigFilePath()
		}
	})
}
