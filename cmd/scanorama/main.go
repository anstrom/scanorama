// Package main provides the entry point for the Scanorama network scanning application.
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

// setVersionInfo sets the version information in the CLI package.
// This function is separated to make it testable.
func setVersionInfo() {
	cli.SetVersion(version, commit, buildTime)
}

// executeApplication runs the CLI application.
// This function is separated to make testing easier.
func executeApplication() {
	cli.Execute()
}

// run contains the main application logic that can be tested.
func run() {
	// Set version information in the CLI package
	setVersionInfo()

	// Execute the root command
	executeApplication()
}

func main() {
	run()
}
