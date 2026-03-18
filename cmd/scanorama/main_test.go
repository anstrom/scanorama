package main

import (
	"os"
	"strings"
	"testing"

	"github.com/anstrom/scanorama/cmd/cli"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestBuildVariables(t *testing.T) {
	t.Run("build variables have default values", func(t *testing.T) {
		assert.Equal(t, "dev", version)
		assert.Equal(t, "none", commit)
		assert.Equal(t, "unknown", buildTime)
	})
}

func TestVersionForwarding(t *testing.T) {
	// Save originals and restore after the test.
	origVersion, origCommit, origBuildTime := version, commit, buildTime
	defer func() {
		version = origVersion
		commit = origCommit
		buildTime = origBuildTime
		cli.SetVersion(origVersion, origCommit, origBuildTime)
	}()

	version = "1.2.3"
	commit = "abc123"
	buildTime = "2024-01-01T00:00:00Z"

	cli.SetVersion(version, commit, buildTime)

	got := cli.GetVersion()
	assert.True(t, strings.Contains(got, "1.2.3"), "GetVersion() should contain the version; got %q", got)
	assert.True(t, strings.Contains(got, "abc123"), "GetVersion() should contain the commit; got %q", got)
	assert.True(t, strings.Contains(got, "2024-01-01T00:00:00Z"),
		"GetVersion() should contain the build time; got %q", got)
}
