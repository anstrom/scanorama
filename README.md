# Scanorama

Scanorama is an advanced network scanning and discovery tool built for continuous network monitoring. It provides OS-aware scanning capabilities, automated scheduling, and robust database persistence for enterprise network management.

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

### Structured Logging
- Built-in structured logging with `slog` support
- Configurable output formats (text, JSON)
- Context-aware logging for scans, discovery, and operations
- File output with automatic directory creation

### Monitoring & Metrics
- Built-in metrics collection for performance monitoring
- Counters, gauges, and histograms with label support
- Automatic timing of scan and discovery operations
- Database query performance tracking

### Error Handling
- Structured error types with error codes and context
- Retryable vs fatal error classification
- Detailed error information for troubleshooting

## Commands

- `discover <network>` - Discover active hosts on network ranges
- `scan --targets <hosts>` - Perform port and service scanning (connect, syn, version, comprehensive, aggressive, stealth)
- `hosts` - Manage and view discovered hosts
- `daemon` - Run as background service with scheduling
- `schedule` - Manage automated scan jobs
- `profiles` - Use predefined scan configurations

## Make Targets

```bash
make help            # Show all commands
make setup-hooks     # Set up Git hooks (one-time)
make setup-dev-db    # Set up database (one-time)
make ci              # Run tests and build
make test            # Run tests only
make build           # Build binary
make clean           # Clean build files
```

## Testing

```bash
# Run all tests
make test

# Run with debug output
DEBUG=true make test

# Run specific tests
go test ./internal -run "Scan"
```

## Contributing

1. Fork and clone the repository
2. Run `make setup-hooks` to install Git hooks
3. Run `make setup-dev-db` to set up development database
4. Make your changes with appropriate tests
5. Run `make ci` to ensure all checks pass
6. Commit with clear, descriptive messages
7. Create a pull request with detailed description

See `docs/` for technical documentation and contribution guidelines.

## License

MIT License - see LICENSE file for details.