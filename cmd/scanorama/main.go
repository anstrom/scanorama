package main

import (
	"github.com/anstrom/scanorama/cmd/cli"
)

// Build information - these will be set by ldflags during build.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	// Set version information in the CLI package
	cli.SetVersion(version, commit, buildTime)

	// Execute the root command
	cli.Execute()
}
