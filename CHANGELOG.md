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