# Scanorama

Scanorama is an advanced network scanning and discovery tool built for continuous network monitoring. It provides OS-aware scanning capabilities, automated scheduling, robust database persistence, and enterprise-grade reliability with comprehensive API support for network management.

## Requirements

- Go 1.24.6+
- nmap 7.0+
- PostgreSQL (for database storage)

## Quick Start

```bash
git clone https://github.com/anstrom/scanorama.git
cd scanorama
make setup-hooks    # Set up development environment
make setup-dev-db   # Initialize PostgreSQL database
make ci             # Build and test
```

## Usage

```bash
# Build the scanner
make build

# Discover hosts on a network
./scanorama discover 192.168.1.0/24

# Scan specific targets with different scan types
./scanorama scan --targets localhost --ports 80,443,8080
./scanorama scan --targets 192.168.1.1,192.168.1.10 --ports 22,80,443 --type aggressive

# View discovered hosts
./scanorama hosts
./scanorama hosts --status up

# Use verbose mode for detailed structured logging
./scanorama -v scan --targets localhost --type version

# Configure logging via config file (config.yaml)
# logging:
#   level: debug
#   format: json
#   output: /var/log/scanorama.log
```

## Features

### Enterprise-Grade Reliability
- Race condition-free worker pool implementation
- Comprehensive test coverage with CI/CD integration
- Graceful shutdown and resource cleanup
- Production-ready error handling and recovery

### Advanced Scanning Engine
- Multiple scan types: connect, SYN, version detection, comprehensive, aggressive, stealth
- Concurrent scanning with configurable rate limiting
- Host discovery with OS detection capabilities
- Service version detection and enumeration

### Structured Logging & Monitoring
- Built-in structured logging with `slog` support
- Configurable output formats (text, JSON)
- Context-aware logging for scans, discovery, and operations
- Built-in metrics collection with counters, gauges, and histograms
- Automatic timing and performance tracking

### Database Integration
- PostgreSQL persistence with automatic migrations
- Transaction support with proper error handling
- Optimized queries with materialized views
- Database connection pooling and health checks

### REST API & Web Interface
- RESTful API for programmatic access
- WebSocket support for real-time updates
- Comprehensive API documentation with Swagger
- Health checks and metrics endpoints

### Error Handling & Observability
- Structured error types with error codes and context
- Retryable vs fatal error classification
- Detailed error information for troubleshooting
- Request tracing and performance monitoring

## Commands

- `discover <network>` - Discover active hosts on network ranges with OS detection
- `scan --targets <hosts>` - Perform port and service scanning with multiple scan types
  - **connect**: TCP connect scanning (default)
  - **syn**: SYN stealth scanning (requires privileges)
  - **version**: Service version detection
  - **comprehensive**: Full port range scanning
  - **aggressive**: OS detection + version scanning + scripts
  - **stealth**: Slow, evasive scanning
- `hosts` - Manage and view discovered hosts with filtering
- `daemon` - Run as background service with API server and scheduling
- `schedule` - Manage automated scan jobs with cron-like scheduling
- `profiles` - Use predefined scan configurations for consistent scanning

## Make Targets

```bash
make help            # Show all commands
make setup-hooks     # Set up Git hooks (one-time)
make setup-dev-db    # Set up database (one-time)
make ci              # Run full CI pipeline (quality + tests + build + security)
make test            # Run all tests (core + integration)
make test-core       # Run core package tests only
make coverage        # Generate test coverage reports
make build           # Build binary
make clean           # Clean build files and temporary artifacts
make lint            # Run code quality checks
make security        # Run security vulnerability scans
make docker-build    # Build Docker image
make docs-generate   # Generate API documentation
```

## Testing

```bash
# Run all tests (core + integration)
make test

# Run with debug output
DEBUG=true make test

# Run core package tests only (errors, logging, metrics)
make test-core

# Generate coverage reports
make coverage

# Run tests with race detection
go test -race ./...

# Run specific tests
go test ./internal/workers -run "TestJobExecution"
```

## Contributing

1. Fork and clone the repository
2. Run `make setup-hooks` to install Git hooks for automated quality checks
3. Run `make setup-dev-db` to set up development PostgreSQL database
4. Make your changes with comprehensive tests (aim for >90% coverage on core packages)
5. Run `make ci` to ensure all quality checks, tests, and security scans pass
6. Commit with clear, descriptive messages following conventional commit format
7. Create a pull request with detailed description and test results

### Code Quality Standards
- All code must pass `make lint` with zero issues
- Core packages (errors, logging, metrics) require >90% test coverage
- No race conditions allowed (`go test -race` must pass)
- All security vulnerabilities must be resolved
- API changes require updated Swagger documentation

See `docs/` for technical documentation and detailed contribution guidelines.

## Releases

To create a release:

1. Update CHANGELOG.md with release notes
2. Create and push a git tag:
   ```bash
   git tag v0.7.0
   git push origin v0.7.0
   ```

3. GitHub Actions will automatically:
   - Run full CI pipeline (tests, linting, security scans)
   - Build cross-platform binaries (Linux amd64, macOS arm64)
   - Create GitHub release with artifacts
   - Build and push Docker images

Release artifacts include statically-linked binaries for multiple platforms built with GoReleaser.

## License

MIT License - see LICENSE file for details.