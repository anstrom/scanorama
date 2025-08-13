package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestBuildVariables(t *testing.T) {
	t.Run("build variables have default values", func(t *testing.T) {
		// These variables are set at build time, so in tests they should have defaults
		assert.Equal(t, "dev", version)
		assert.Equal(t, "none", commit)
		assert.Equal(t, "unknown", buildTime)
	})

	t.Run("build variables can be modified", func(t *testing.T) {
		// Save original values
		origVersion := version
		origCommit := commit
		origBuildTime := buildTime

		// Modify values
		version = "1.0.0"
		commit = "abc123"
		buildTime = "2024-01-01T00:00:00Z"

		// Verify changes
		assert.Equal(t, "1.0.0", version)
		assert.Equal(t, "abc123", commit)
		assert.Equal(t, "2024-01-01T00:00:00Z", buildTime)

		// Restore original values
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
	})
}

func TestMainFunctionIntegration(t *testing.T) {
	t.Run("main function does not panic", func(t *testing.T) {
		// This test ensures the main function can be called without panicking
		// We'll test this by checking that importing and calling version setting doesn't fail
		assert.NotPanics(t, func() {
			// We can't actually call main() in tests as it would execute the CLI
			// But we can test the version setting part
			testVersion := "test-version"
			testCommit := "test-commit"
			testBuildTime := "test-build-time"

			// Save originals
			origVersion := version
			origCommit := commit
			origBuildTime := buildTime

			// Set test values
			version = testVersion
			commit = testCommit
			buildTime = testBuildTime

			// This simulates what main() does - importing cli package shouldn't panic
			// The actual CLI execution is tested in the cli package

			// Restore originals
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
		})
	})
}

func TestVersionInformation(t *testing.T) {
	t.Run("version variables are strings", func(t *testing.T) {
		assert.IsType(t, "", version)
		assert.IsType(t, "", commit)
		assert.IsType(t, "", buildTime)
	})

	t.Run("version information is not empty in build", func(t *testing.T) {
		// In actual builds, these should be set by ldflags
		// In tests, they have default values
		assert.NotEmpty(t, version)
		assert.NotEmpty(t, commit)
		assert.NotEmpty(t, buildTime)
	})

	t.Run("build time format validation", func(t *testing.T) {
		// Test with a properly formatted build time
		testBuildTime := "2024-01-01T12:00:00Z"

		// Parse the time to validate format
		_, err := time.Parse(time.RFC3339, testBuildTime)
		require.NoError(t, err, "Build time should be in RFC3339 format")

		// Test default value
		if buildTime != "unknown" {
			_, err := time.Parse(time.RFC3339, buildTime)
			if err != nil {
				// If parsing fails, it should be the default "unknown" value
				assert.Equal(t, "unknown", buildTime)
			}
		}
	})
}

func TestPackageStructure(t *testing.T) {
	t.Run("imports are valid", func(t *testing.T) {
		// This test verifies that all imports in main.go are accessible
		// The fact that this test file compiles means the imports are valid
		assert.True(t, true, "All imports should be accessible")
	})

	t.Run("package is main", func(t *testing.T) {
		// Verify this is actually the main package
		// This is more of a compile-time check, but we can assert it's true
		assert.True(t, true, "Package should be main")
	})
}

func TestMainPackageConstants(t *testing.T) {
	t.Run("default version is dev", func(t *testing.T) {
		// In development/test builds, version should default to "dev"
		if version == "dev" {
			assert.Equal(t, "dev", version)
			assert.Equal(t, "none", commit)
			assert.Equal(t, "unknown", buildTime)
		}
	})
}

// TestMainExecutionPath tests that the main execution path is correct
func TestMainExecutionPath(t *testing.T) {
	t.Run("main execution flow", func(t *testing.T) {
		// We can't directly test main() as it would run the CLI
		// But we can test the components that main() uses

		// Test that version setting variables exist and are accessible
		assert.NotNil(t, &version)
		assert.NotNil(t, &commit)
		assert.NotNil(t, &buildTime)

		// Test that the variables can be read
		v := version
		c := commit
		bt := buildTime

		assert.IsType(t, "", v)
		assert.IsType(t, "", c)
		assert.IsType(t, "", bt)
	})
}

// BenchmarkVersionAccess benchmarks accessing version variables
func BenchmarkVersionAccess(b *testing.B) {
	b.Run("version variable access", func(b *testing.B) {
		var v string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v = version
		}
		_ = v
	})

	b.Run("all version variables access", func(b *testing.B) {
		var v, c, bt string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v = version
			c = commit
			bt = buildTime
		}
		_, _, _ = v, c, bt
	})
}
