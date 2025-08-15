# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- TBD

### Changed
- TBD

### Fixed
- TBD

## [0.7.1] - 2025-08-15

### Added
- Test helper utilities for better testing experience and database setup
- Developer experience improvements with new Makefile targets (`dev`, `validate`, `test-unit`, `check`, `deps`, `quick`)
- Comprehensive testing utilities in `test/helpers/testing.go` for database, HTTP, and network testing

### Changed
- Upgraded `github.com/swaggo/http-swagger` from v1.3.4 to v2.0.2 for improved API documentation
- Updated `@redocly/cli` from v1.34.5 to v2.0.5 for better OpenAPI tooling
- Updated `@quobix/vacuum` from v0.6.3 to v0.17.8 for enhanced API linting
- Improved developer workflow with quick validation and testing commands

### Fixed
- Removed deprecated HTTP Swagger v1 dependency in favor of v2
- Enhanced development experience with streamlined setup and validation commands

## [0.2.0] - 2025-08-08

### Added
- Structured logging system using Go's slog package
- Support for text and JSON log output formats
- Configurable log levels (debug, info, warn, error) and outputs
- Context-aware logging for scans, discovery, database, and daemon operations
- Metrics collection system with counters, gauges, and histograms
- Worker pool for concurrent job execution with retry logic
- Rate limiting for network operations to avoid overwhelming targets
- Structured error handling with specific error codes and types
- ScanError, DatabaseError, DiscoveryError, and ConfigError implementations
- Technical documentation structure in docs/technical/
- Architecture documentation for logging and worker systems

### Changed
- Aligned scan types with nmap terminology (renamed 'intense' to 'aggressive')
- Updated CLI descriptions to remove redundant text
- Organized documentation into technical vs project documentation
- Migrated from basic CLI to Cobra framework with Viper configuration
- Updated all dependencies to latest stable versions
- Improved error handling patterns throughout application

### Fixed
- Proper version injection using git describe with commit hashes for untagged builds
- Build system now shows versions like 'v0.2.0-1-gcommit' for development builds
- Resolved all linting issues across the codebase
- Fixed Go module dependencies and build configuration

## [0.1.0] - 2024-01-15

### Added
- Initial release of Scanorama network scanner
- Basic port scanning functionality
- Host discovery capabilities
- Configuration file support
- Basic logging infrastructure

[Unreleased]: https://github.com/username/scanorama/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/username/scanorama/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/username/scanorama/releases/tag/v0.1.0