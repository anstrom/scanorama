package internal

// This file explicitly exports functions to ensure CI visibility
// This is a workaround for CI compilation issues where functions
// appear undefined despite being properly defined in other files.

import "context"

// Ensure RunScan is exported and visible
var (
	// RunScan wrapper to ensure export visibility
	_ = RunScan
	// RunScanWithContext wrapper to ensure export visibility
	_ = RunScanWithContext
	// PrintResults wrapper to ensure export visibility
	_ = PrintResults
)

// ExplicitRunScan provides an explicit export of RunScan
func ExplicitRunScan(config ScanConfig) (*ScanResult, error) {
	return RunScan(config)
}

// ExplicitRunScanWithContext provides an explicit export of RunScanWithContext
func ExplicitRunScanWithContext(ctx context.Context, config ScanConfig) (*ScanResult, error) {
	return RunScanWithContext(ctx, config)
}

// ExplicitPrintResults provides an explicit export of PrintResults
func ExplicitPrintResults(result *ScanResult) {
	PrintResults(result)
}

// ForceExports ensures all functions are referenced and exported
func ForceExports() {
	// This function references all exported functions to ensure they're compiled
	// and available for external packages
	var config ScanConfig
	var result *ScanResult
	var ctx context.Context

	// Reference RunScan
	_, _ = RunScan(config)

	// Reference RunScanWithContext
	_, _ = RunScanWithContext(ctx, config)

	// Reference PrintResults
	PrintResults(result)
}
