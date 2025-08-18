# Fix: Respect --config flag in CLI commands

## Problem

The `--config` flag was being ignored by Scanorama CLI commands because they were hardcoded to load `config.yaml` instead of using the config file specified by the user.

### Impact
- Users couldn't specify custom config files via `--config /path/to/custom.yaml`
- All commands would always try to load `config.yaml` regardless of the flag
- This made it impossible to use different configurations for different environments

### Affected Commands
- `scanorama scan`
- `scanorama daemon`
- All commands using database helpers (hosts, networks, etc.)

## Solution

Added a `getConfigFilePath()` helper function that:
1. Uses `viper.ConfigFileUsed()` to get the actual config file loaded by Cobra/Viper
2. Falls back to `config.yaml` if no config file was explicitly set
3. Respects the `--config` flag properly

## Changes Made

### Files Modified
- **`cmd/cli/root.go`**: Added `getConfigFilePath()` helper function
- **`cmd/cli/scan.go`**: Replace hardcoded `"config.yaml"` with `getConfigFilePath()`
- **`cmd/cli/daemon.go`**: Replace hardcoded `"config.yaml"` with `getConfigFilePath()`
- **`cmd/cli/database_helpers.go`**: Replace hardcoded `"config.yaml"` in both helper functions

### Tests Added
- **`cmd/cli/config_test.go`**: Comprehensive test suite covering:
  - Default behavior when no config specified
  - Custom config file path handling
  - Integration with viper configuration
  - Edge cases and benchmarks

## Testing

### Manual Testing
```bash
# Test with default config
scanorama scan --live-hosts

# Test with custom config
scanorama scan --config /path/to/custom.yaml --live-hosts

# Test with different commands
scanorama daemon start --config /etc/scanorama/prod.yaml
```

### Automated Testing
```bash
go test ./cmd/cli -v -run "TestConfig"
```

All tests pass with 100% coverage for the new functionality.

## Backward Compatibility

✅ **No breaking changes**
- Default behavior unchanged (still loads `config.yaml` by default)
- All existing command usage continues to work exactly as before
- Only adds new functionality for the `--config` flag

## Verification

Before this fix:
```bash
scanorama scan --config custom.yaml --live-hosts
# Would ignore custom.yaml and always try to load config.yaml
```

After this fix:
```bash
scanorama scan --config custom.yaml --live-hosts
# Correctly loads and uses custom.yaml
```

## Code Quality

- ✅ All pre-commit hooks pass
- ✅ Linting passes with 0 issues
- ✅ Full test coverage for new functionality
- ✅ No security vulnerabilities introduced
- ✅ Follows existing code patterns and conventions

## Related

This fix enables users to:
- Use different configs for different environments (dev/staging/prod)
- Store configs in custom locations
- Use organization-specific config file naming conventions
- Properly utilize the existing `--config` flag that was advertised but not working

Closes: #[issue-number] (if applicable)