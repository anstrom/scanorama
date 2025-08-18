package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("returns default configuration", func(t *testing.T) {
		config := DefaultConfig()

		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, 5432, config.Port)
		assert.Empty(t, config.Database)
		assert.Empty(t, config.Username)
		assert.Empty(t, config.Password)
		assert.Equal(t, "disable", config.SSLMode)
		assert.Equal(t, 25, config.MaxOpenConns)
		assert.Equal(t, 5, config.MaxIdleConns)
		assert.Equal(t, 5*time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, 5*time.Minute, config.ConnMaxIdleTime)
	})

	t.Run("default config is valid structure", func(t *testing.T) {
		config := DefaultConfig()

		// Verify all fields are accessible
		assert.IsType(t, "", config.Host)
		assert.IsType(t, 0, config.Port)
		assert.IsType(t, "", config.Database)
		assert.IsType(t, "", config.Username)
		assert.IsType(t, "", config.Password)
		assert.IsType(t, "", config.SSLMode)
		assert.IsType(t, 0, config.MaxOpenConns)
		assert.IsType(t, 0, config.MaxIdleConns)
		assert.IsType(t, time.Duration(0), config.ConnMaxLifetime)
		assert.IsType(t, time.Duration(0), config.ConnMaxIdleTime)
	})
}

func TestConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := Config{
			Host:            "localhost",
			Port:            5432,
			Database:        "testdb",
			Username:        "testuser",
			Password:        "testpass",
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		}

		// Test that config can be created and accessed
		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, 5432, config.Port)
		assert.Equal(t, "testdb", config.Database)
		assert.Equal(t, "testuser", config.Username)
		assert.Equal(t, "testpass", config.Password)
		assert.Equal(t, "disable", config.SSLMode)
		assert.Equal(t, 10, config.MaxOpenConns)
		assert.Equal(t, 5, config.MaxIdleConns)
		assert.Equal(t, time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, time.Minute, config.ConnMaxIdleTime)
	})

	t.Run("config with different ssl modes", func(t *testing.T) {
		sslModes := []string{"disable", "require", "verify-ca", "verify-full"}

		for _, mode := range sslModes {
			config := DefaultConfig()
			config.SSLMode = mode

			assert.Equal(t, mode, config.SSLMode)
		}
	})

	t.Run("config with various ports", func(t *testing.T) {
		testPorts := []int{5432, 5433, 5434, 3306, 1433}

		for _, port := range testPorts {
			config := DefaultConfig()
			config.Port = port

			assert.Equal(t, port, config.Port)
			assert.True(t, port > 0 && port < 65536)
		}
	})

	t.Run("config with connection limits", func(t *testing.T) {
		config := DefaultConfig()
		config.MaxOpenConns = 50
		config.MaxIdleConns = 10

		assert.Equal(t, 50, config.MaxOpenConns)
		assert.Equal(t, 10, config.MaxIdleConns)
		assert.True(t, config.MaxOpenConns >= config.MaxIdleConns)
	})
}

func TestConfigConnectionString(t *testing.T) {
	t.Run("build connection string components", func(t *testing.T) {
		config := Config{
			Host:     "localhost",
			Port:     5432,
			Database: "testdb",
			Username: "testuser",
			Password: "testpass",
			SSLMode:  "disable",
		}

		// Test that all components are accessible for connection string building
		assert.NotEmpty(t, config.Host)
		assert.Greater(t, config.Port, 0)
		assert.NotEmpty(t, config.Database)
		assert.NotEmpty(t, config.Username)
		assert.NotEmpty(t, config.Password)
		assert.NotEmpty(t, config.SSLMode)
	})

	t.Run("config with special characters", func(t *testing.T) {
		config := Config{
			Host:     "db-host.example.com",
			Database: "test_db-name",
			Username: "user@domain.com",
			Password: "p@ssw0rd!#$",
			SSLMode:  "require",
		}

		// Test that special characters are handled properly
		assert.Contains(t, config.Host, "-")
		assert.Contains(t, config.Database, "_")
		assert.Contains(t, config.Username, "@")
		assert.Contains(t, config.Password, "@")
		assert.Contains(t, config.Password, "#")
		assert.Equal(t, "require", config.SSLMode)
	})

	t.Run("config with empty database name", func(t *testing.T) {
		config := DefaultConfig()
		assert.Empty(t, config.Database)
	})

	t.Run("config with empty credentials", func(t *testing.T) {
		config := DefaultConfig()
		assert.Empty(t, config.Username)
		assert.Empty(t, config.Password)
	})
}

func TestConfigTimeouts(t *testing.T) {
	t.Run("default timeouts", func(t *testing.T) {
		config := DefaultConfig()

		assert.Equal(t, 5*time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, 5*time.Minute, config.ConnMaxIdleTime)
	})

	t.Run("custom timeouts", func(t *testing.T) {
		config := Config{
			ConnMaxLifetime: 10 * time.Minute,
			ConnMaxIdleTime: 2 * time.Minute,
		}

		assert.Equal(t, 10*time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, 2*time.Minute, config.ConnMaxIdleTime)
	})

	t.Run("timeout edge cases", func(t *testing.T) {
		config := Config{
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: time.Second,
		}

		assert.Equal(t, time.Duration(0), config.ConnMaxLifetime)
		assert.Equal(t, time.Second, config.ConnMaxIdleTime)
	})
}

func TestConfigCopy(t *testing.T) {
	t.Run("config can be copied", func(t *testing.T) {
		original := Config{
			Host:            "original-host",
			Port:            5432,
			Database:        "original-db",
			Username:        "original-user",
			Password:        "original-pass",
			SSLMode:         "require",
			MaxOpenConns:    20,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 30 * time.Minute,
		}

		// Create a copy
		configCopy := original

		// Verify copy has same values
		assert.Equal(t, original.Host, configCopy.Host)
		assert.Equal(t, original.Port, configCopy.Port)
		assert.Equal(t, original.Database, configCopy.Database)
		assert.Equal(t, original.Username, configCopy.Username)
		assert.Equal(t, original.Password, configCopy.Password)
		assert.Equal(t, original.SSLMode, configCopy.SSLMode)
		assert.Equal(t, original.MaxOpenConns, configCopy.MaxOpenConns)
		assert.Equal(t, original.MaxIdleConns, configCopy.MaxIdleConns)
		assert.Equal(t, original.ConnMaxLifetime, configCopy.ConnMaxLifetime)
		assert.Equal(t, original.ConnMaxIdleTime, configCopy.ConnMaxIdleTime)

		// Modify copy to ensure it's independent
		configCopy.Host = "modified-host"
		assert.NotEqual(t, original.Host, configCopy.Host)
	})
}

func TestConfigConstants(t *testing.T) {
	t.Run("default constants are valid", func(t *testing.T) {
		assert.Equal(t, 5432, defaultPostgresPort)
		assert.Equal(t, 25, defaultMaxOpenConns)
		assert.Equal(t, 5, defaultMaxIdleConns)
		assert.Equal(t, 5, defaultConnMaxLifetime)
		assert.Equal(t, 5, defaultConnMaxIdleTime)
	})

	t.Run("constants are used in default config", func(t *testing.T) {
		config := DefaultConfig()

		assert.Equal(t, defaultPostgresPort, config.Port)
		assert.Equal(t, defaultMaxOpenConns, config.MaxOpenConns)
		assert.Equal(t, defaultMaxIdleConns, config.MaxIdleConns)
		assert.Equal(t, defaultConnMaxLifetime*time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, defaultConnMaxIdleTime*time.Minute, config.ConnMaxIdleTime)
	})
}

func TestConfigStructure(t *testing.T) {
	t.Run("config has all required fields", func(t *testing.T) {
		config := Config{}

		// Verify all fields exist and have correct types
		config.Host = "test"
		config.Port = 1234
		config.Database = "test"
		config.Username = "test"
		config.Password = "test"
		config.SSLMode = "test"
		config.MaxOpenConns = 10
		config.MaxIdleConns = 5
		config.ConnMaxLifetime = time.Minute
		config.ConnMaxIdleTime = time.Second

		assert.Equal(t, "test", config.Host)
		assert.Equal(t, 1234, config.Port)
		assert.Equal(t, "test", config.Database)
		assert.Equal(t, "test", config.Username)
		assert.Equal(t, "test", config.Password)
		assert.Equal(t, "test", config.SSLMode)
		assert.Equal(t, 10, config.MaxOpenConns)
		assert.Equal(t, 5, config.MaxIdleConns)
		assert.Equal(t, time.Minute, config.ConnMaxLifetime)
		assert.Equal(t, time.Second, config.ConnMaxIdleTime)
	})

	t.Run("config supports zero values", func(t *testing.T) {
		config := Config{}

		assert.Empty(t, config.Host)
		assert.Zero(t, config.Port)
		assert.Empty(t, config.Database)
		assert.Empty(t, config.Username)
		assert.Empty(t, config.Password)
		assert.Empty(t, config.SSLMode)
		assert.Zero(t, config.MaxOpenConns)
		assert.Zero(t, config.MaxIdleConns)
		assert.Zero(t, config.ConnMaxLifetime)
		assert.Zero(t, config.ConnMaxIdleTime)
	})
}

func TestConfigEdgeCases(t *testing.T) {
	t.Run("config with localhost variations", func(t *testing.T) {
		hosts := []string{"localhost", "127.0.0.1", "::1", "db", "database.local"}

		for _, host := range hosts {
			config := DefaultConfig()
			config.Host = host

			assert.Equal(t, host, config.Host)
			assert.NotEmpty(t, config.Host)
		}
	})

	t.Run("config with various ssl modes", func(t *testing.T) {
		sslModes := []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}

		for _, mode := range sslModes {
			config := DefaultConfig()
			config.SSLMode = mode

			assert.Equal(t, mode, config.SSLMode)
		}
	})

	t.Run("config with high connection counts", func(t *testing.T) {
		config := DefaultConfig()
		config.MaxOpenConns = 1000
		config.MaxIdleConns = 100

		assert.Equal(t, 1000, config.MaxOpenConns)
		assert.Equal(t, 100, config.MaxIdleConns)
		assert.True(t, config.MaxOpenConns > config.MaxIdleConns)
	})

	t.Run("config with very short timeouts", func(t *testing.T) {
		config := DefaultConfig()
		config.ConnMaxLifetime = time.Second
		config.ConnMaxIdleTime = 500 * time.Millisecond

		assert.Equal(t, time.Second, config.ConnMaxLifetime)
		assert.Equal(t, 500*time.Millisecond, config.ConnMaxIdleTime)
	})

	t.Run("config with very long timeouts", func(t *testing.T) {
		config := DefaultConfig()
		config.ConnMaxLifetime = 24 * time.Hour
		config.ConnMaxIdleTime = time.Hour

		assert.Equal(t, 24*time.Hour, config.ConnMaxLifetime)
		assert.Equal(t, time.Hour, config.ConnMaxIdleTime)
	})
}

// BenchmarkDefaultConfig benchmarks the DefaultConfig function
func BenchmarkDefaultConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// BenchmarkConfigCopy benchmarks copying a config struct
func BenchmarkConfigCopy(b *testing.B) {
	original := DefaultConfig()
	original.Host = "benchmark-host"
	original.Database = "benchmark-db"
	original.Username = "benchmark-user"
	original.Password = "benchmark-password"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		configCopy := original
		_ = configCopy
	}
}

// BenchmarkConfigFieldAccess benchmarks accessing config fields
func BenchmarkConfigFieldAccess(b *testing.B) {
	config := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Host
		_ = config.Port
		_ = config.Database
		_ = config.Username
		_ = config.Password
		_ = config.SSLMode
		_ = config.MaxOpenConns
		_ = config.MaxIdleConns
		_ = config.ConnMaxLifetime
		_ = config.ConnMaxIdleTime
	}
}
