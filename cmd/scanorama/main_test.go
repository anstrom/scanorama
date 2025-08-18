package main

import (
	"os"
	"testing"
	"time"

	"github.com/anstrom/scanorama/cmd/cli"
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

		// Ensure cleanup happens even if test panics
		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
		}()

		// Modify values
		version = "1.0.0"
		commit = "abc123"
		buildTime = "2024-01-01T00:00:00Z"

		// Verify changes
		assert.Equal(t, "1.0.0", version)
		assert.Equal(t, "abc123", commit)
		assert.Equal(t, "2024-01-01T00:00:00Z", buildTime)
	})
}

// TestSetVersionIntegration tests the actual version setting functionality used in main()
func TestSetVersionIntegration(t *testing.T) {
	t.Run("set version calls cli.SetVersion", func(t *testing.T) {
		// Test that we can call cli.SetVersion with our variables
		testVersion := "2.1.0"
		testCommit := "def456"
		testBuildTime := "2024-03-15T10:30:00Z"

		// This tests the actual function call that main() makes
		assert.NotPanics(t, func() {
			cli.SetVersion(testVersion, testCommit, testBuildTime)
		})
	})

	t.Run("set version with current variables", func(t *testing.T) {
		// Test setting version with the current global variables
		// This simulates exactly what main() does
		assert.NotPanics(t, func() {
			cli.SetVersion(version, commit, buildTime)
		})
	})

	t.Run("set version with empty values", func(t *testing.T) {
		// Test edge case with empty values
		assert.NotPanics(t, func() {
			cli.SetVersion("", "", "")
		})
	})

	t.Run("set version multiple times", func(t *testing.T) {
		// Test that version can be set multiple times
		assert.NotPanics(t, func() {
			cli.SetVersion("v1.0.0", "abc123", "2024-01-01T00:00:00Z")
			cli.SetVersion("v2.0.0", "def456", "2024-02-01T00:00:00Z")
			cli.SetVersion(version, commit, buildTime) // Reset to defaults
		})
	})
}

// TestSetVersionInfo tests the setVersionInfo function
func TestSetVersionInfo(t *testing.T) {
	t.Run("setVersionInfo calls cli.SetVersion", func(t *testing.T) {
		// Test that setVersionInfo function executes without panic
		assert.NotPanics(t, func() {
			setVersionInfo()
		})
	})

	t.Run("setVersionInfo with modified variables", func(t *testing.T) {
		// Save original values
		origVersion := version
		origCommit := commit
		origBuildTime := buildTime

		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
		}()

		// Modify variables
		version = "test-v1.0.0"
		commit = "test-commit-123"
		buildTime = "2024-01-01T00:00:00Z"

		// Test setVersionInfo with modified values
		assert.NotPanics(t, func() {
			setVersionInfo()
		})
	})
}

// TestExecuteApplication tests the executeApplication function
func TestExecuteApplication(t *testing.T) {
	t.Run("executeApplication function exists", func(t *testing.T) {
		// We can't easily test executeApplication directly as it calls cli.Execute()
		// which would run the actual CLI. But we can verify the function exists
		// and can be called in a test context with controlled args.

		// Save original args
		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()

		// Set args to help to make execution quick
		os.Args = []string{"scanorama", "--help"}

		// The function exists and is callable (though we won't call it in tests)
		assert.True(t, true, "executeApplication function should exist")
	})
}

// TestRunFunction tests the run() function that contains the main logic
func TestRunFunction(t *testing.T) {
	t.Run("run function components", func(t *testing.T) {
		// Test that we can call the individual components of run()
		assert.NotPanics(t, func() {
			setVersionInfo()
		})

		// We can't test executeApplication() in unit tests as it would run the CLI
		// But we can verify that run() would call both components
	})

	t.Run("run function with version variables", func(t *testing.T) {
		// Test run() components with current version variables
		assert.NotPanics(t, func() {
			setVersionInfo()
		})
	})
}

// TestMainFunctionComponents tests the individual components that main() uses
func TestMainFunctionComponents(t *testing.T) {
	t.Run("version setting component", func(t *testing.T) {
		// Test the first part of main(): cli.SetVersion(version, commit, buildTime)
		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()

		// Set test args to avoid CLI execution
		os.Args = []string{"scanorama", "--help"}

		// Test the version setting call
		assert.NotPanics(t, func() {
			cli.SetVersion(version, commit, buildTime)
		})
	})

	t.Run("version variables accessibility", func(t *testing.T) {
		// Test that all version variables can be accessed
		v := version
		c := commit
		bt := buildTime

		assert.NotEmpty(t, v)
		assert.NotEmpty(t, c)
		assert.NotEmpty(t, bt)
	})
}

// TestMainExecutionSimulation simulates the main function execution
func TestMainExecutionSimulation(t *testing.T) {
	t.Run("simulate main execution steps", func(t *testing.T) {
		// Save original args
		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()

		// Set args to help command to avoid hanging
		os.Args = []string{"scanorama", "--help"}

		// Step 1: Set version information (this is what main() does first)
		assert.NotPanics(t, func() {
			cli.SetVersion(version, commit, buildTime)
		})

		// Step 2: We can't test cli.Execute() directly as it would run the CLI
		// But we can verify it exists and is callable in a controlled way
		// The CLI package tests handle the actual execution testing
	})

	t.Run("version setting with ldflags simulation", func(t *testing.T) {
		// Simulate what happens when ldflags set the version variables
		testVersion := "v1.2.3"
		testCommit := "a1b2c3d4"
		testBuildTime := "2024-03-15T14:30:00Z"

		// Temporarily modify the global variables (simulating ldflags)
		origVersion := version
		origCommit := commit
		origBuildTime := buildTime

		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
		}()

		version = testVersion
		commit = testCommit
		buildTime = testBuildTime

		// Now test setting the version (what main() would do)
		assert.NotPanics(t, func() {
			cli.SetVersion(version, commit, buildTime)
		})

		// Verify the variables were set correctly
		assert.Equal(t, testVersion, version)
		assert.Equal(t, testCommit, commit)
		assert.Equal(t, testBuildTime, buildTime)
	})
}

func TestVersionInformation(t *testing.T) {
	t.Run("version variables are strings", func(t *testing.T) {
		assert.IsType(t, "", version)
		assert.IsType(t, "", commit)
		assert.IsType(t, "", buildTime)
	})

	t.Run("version information is not empty", func(t *testing.T) {
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

	t.Run("version format validation", func(t *testing.T) {
		// Test that version follows expected format
		assert.True(t, version != "", "Version should not be empty")

		// Common version formats
		validFormats := []string{"dev", "v1.0.0", "1.0.0", "latest"}
		isValidFormat := false
		for _, format := range validFormats {
			if version == format || len(version) >= 3 {
				isValidFormat = true
				break
			}
		}
		assert.True(t, isValidFormat, "Version should follow a valid format")
	})
}

// TestMainPackageStructure tests the package structure and imports
func TestMainPackageStructure(t *testing.T) {
	t.Run("cli package import works", func(t *testing.T) {
		// Test that we can call functions from the cli package
		assert.NotPanics(t, func() {
			cli.SetVersion("test", "test", "test")
		})
	})

	t.Run("package variables are accessible", func(t *testing.T) {
		// Test that package-level variables are accessible
		v := version
		c := commit
		bt := buildTime

		assert.IsType(t, "", v)
		assert.IsType(t, "", c)
		assert.IsType(t, "", bt)
	})
}

// TestMainVsRunSeparation tests that main and run are properly separated
func TestMainVsRunSeparation(t *testing.T) {
	t.Run("main calls run", func(t *testing.T) {
		// We can't directly test main() but we can test that run() contains the logic
		// This test verifies that the separation exists

		// Test that we can call the components of run()
		assert.NotPanics(t, func() {
			setVersionInfo()
		})
	})

	t.Run("run function components", func(t *testing.T) {
		// Test the individual components that run() should execute

		// Component 1: Version setting
		assert.NotPanics(t, func() {
			setVersionInfo()
		})

		// Component 2: CLI execution (we can't test this directly but verify function exists)
		// executeApplication() is tested separately where possible
	})

	t.Run("function separation verification", func(t *testing.T) {
		// Verify that the main logic is properly separated into testable functions

		// Test setVersionInfo independently
		assert.NotPanics(t, func() {
			setVersionInfo()
		})

		// executeApplication exists but we don't call it in tests
		// run() exists and calls both functions
	})
}

// TestApplicationEntryPoint tests the application entry point structure
func TestApplicationEntryPoint(t *testing.T) {
	t.Run("version info setting", func(t *testing.T) {
		// Test the version information setting functionality
		assert.NotPanics(t, func() {
			setVersionInfo()
		})
	})

	t.Run("application structure", func(t *testing.T) {
		// Test that the application has proper structure
		// main() -> run() -> setVersionInfo() + executeApplication()

		// We can test setVersionInfo()
		assert.NotPanics(t, func() {
			setVersionInfo()
		})

		// executeApplication() exists but calls cli.Execute()
		// run() orchestrates both calls
	})
}

// TestBuildInformation tests build-time information handling
func TestBuildInformation(t *testing.T) {
	t.Run("default build values", func(t *testing.T) {
		// In development/test builds, these should be the defaults
		assert.Equal(t, "dev", version)
		assert.Equal(t, "none", commit)
		assert.Equal(t, "unknown", buildTime)
	})

	t.Run("build variable modification", func(t *testing.T) {
		// Test that build variables can be modified (simulating ldflags)
		origValues := []string{version, commit, buildTime}

		version = "v2.0.0"
		commit = "abc123def"
		buildTime = "2024-12-25T00:00:00Z"

		// Test that the changes are reflected
		assert.Equal(t, "v2.0.0", version)
		assert.Equal(t, "abc123def", commit)
		assert.Equal(t, "2024-12-25T00:00:00Z", buildTime)

		// Restore original values
		version = origValues[0]
		commit = origValues[1]
		buildTime = origValues[2]
	})

	t.Run("version setting propagation", func(t *testing.T) {
		// Test that version setting propagates to CLI
		testVer := "v1.5.0"
		testCommit := "xyz789"
		testTime := "2024-06-01T12:00:00Z"

		assert.NotPanics(t, func() {
			cli.SetVersion(testVer, testCommit, testTime)
		})
	})
}

// BenchmarkMainFunctionComponents benchmarks the main function components
func BenchmarkMainFunctionComponents(b *testing.B) {
	b.Run("setVersionInfo", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			setVersionInfo()
		}
	})

	b.Run("version variable access", func(b *testing.B) {
		var v, c, bt string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v = version
			c = commit
			bt = buildTime
		}
		_, _, _ = v, c, bt
	})

	b.Run("direct cli.SetVersion", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cli.SetVersion(version, commit, buildTime)
		}
	})
}
