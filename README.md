# Scanorama

A flexible network scanner built in Go, powered by nmap. Scanorama provides a command-line interface for network
discovery and security auditing.

## Features

- Multiple scan types:
  - SYN scan (default)
  - TCP connect scan
  - Version detection scan
  - Script scan (using nmap NSE)
- Test coverage for core components
- Flexible target specification:
  - Single IP addresses
  - Multiple IP addresses
  - CIDR ranges
  - Hostnames
- Customizable port selection:
  - Single ports
  - Port ranges
  - Multiple port specifications
- Output options:
  - Human-readable console output
  - XML file output
- Built-in error handling and logging
- Progress reporting

## Requirements

- Go 1.24.6 or later
- nmap 7.0 or later
- Root/sudo privileges for SYN scans (optional)

## Installation

1. Clone the repository:

```/dev/null/install.sh#L1-3
git clone https://github.com/anstrom/scanorama.git
cd scanorama
```


1. Build and test the project using Make:

```/dev/null/build.sh#L1-6
# Set up development environment
make setup-hooks
make setup-dev-db

# Build and test the project
make ci
```

## Usage

Basic syntax:

```bash
./scanorama -targets <target-spec> [options]
```


### Examples

1. Basic scan of common ports:

```/dev/null/examples.sh#L1-2
./scanorama -targets localhost
```


1. Scan specific ports on multiple targets:

```/dev/null/examples.sh#L4-5
./scanorama -targets "192.168.1.1,192.168.1.2" -ports "80,443,8080"
```


1. Network range scan with version detection:

```/dev/null/examples.sh#L7-8
./scanorama -targets 192.168.1.0/24 -type version
```


1. Aggressive scan with script execution:

```/dev/null/examples.sh#L10-11
sudo ./scanorama -targets example.com -type script -aggressive
```


### Command Line Options

- `-targets`: Target specification (required)
  - Example: "192.168.1.1" or "192.168.1.0/24" or "example.com"
- `-ports`: Port specification (default: "1-1000")
  - Example: "80,443" or "1-1000"
- `-type`: Scan type (default: "syn")
  - Options: "syn", "connect", "version", "script"
- `-aggressive`: Enable aggressive scanning (version detection, script scanning)
- `-output`: Save results to XML file
  - Example: "-output results.xml"

## Project Structure


```/dev/null/structure.txt#L1-20
scanorama/
├── cmd/
│   └── scanorama/
│       └── main.go        # Entry point
├── internal/
│   ├── config/            # Configuration handling
│   ├── daemon/            # Background service logic
│   ├── db/                # Database interactions
│   ├── scan.go            # Core scanning logic
│   ├── scan_test.go       # Scan tests
│   ├── xml.go             # XML handling
│   └── xml_test.go        # XML tests
├── test/
│   ├── docker/            # Docker test environment
│   ├── fixtures/          # Test fixtures
│   └── helpers/           # Test helper utilities
├── scripts/               # Utility scripts
├── go.mod                 # Go module file
├── Makefile               # Build automation
└── README.md              # This file
```


## Build and Test

### Make Commands

```/dev/null/make-help.sh#L1-10
make help            # Show available commands
make setup-hooks     # Set up Git hooks for code quality
make setup-dev-db    # Set up development database
make ci              # Run full CI pipeline locally
make test            # Run tests (DEBUG=true for verbose output)
make build           # Build the binary
make quality         # Run comprehensive code quality checks
make lint            # Run linters
make format          # Fix formatting and common issues
make clean           # Remove build artifacts
```


The binary will be built in the `build/` directory by default.

### Testing

The project includes test suites for scanning and XML handling. Tests are organized alongside their implementation
files in the `internal/` directory.

#### Test Organization

- **Unit Tests**: Located in `*_test.go` files alongside their implementation files
- **Test Fixtures**: Located in `test/fixtures/` directory 
- **Docker Test Environment**: Located in `test/docker/` for integration testing
- **Test Helpers**: Reusable test utilities in `test/helpers/`

#### Docker Test Environment

The project uses Docker to provide a consistent test environment with the following services:
- PostgreSQL database
- Nginx web server
- SSH server
- Redis instance
- Flask application

> **Note:** If you already have PostgreSQL running locally on port 5432, you might experience port conflicts when running tests. You can either stop your local PostgreSQL instance before running tests or configure a different port as shown in the configuration section below.

To manage the test environment:

```/dev/null/test-env.sh#L1-10
# Set up development database (one-time setup)
make setup-dev-db

# Run tests (automatically manages test environment)
make test

# Run tests with debug output
DEBUG=true make test

# Check database status
./scripts/check-db.sh
```

The test environment automatically starts before tests run and stops after tests complete when using `make test`.

#### Running Tests

To run specific test suites:

```/dev/null/test.sh#L1-12
# Run all tests
make test

# Run tests with debug output
DEBUG=true make test

# Run scan tests only
go test ./internal -run "Scan"

# Run XML tests only
go test ./internal -run "XML"

# Run database tests only
go test ./internal/db -v
```

#### Test Environment Configuration

The test environment uses Docker containers to provide services for testing:

- PostgreSQL database runs on port 5432 (default PostgreSQL port)
- The test database name is `scanorama_test` with user `test_user` and password `test_password`
- Other test services run on non-standard ports to avoid conflicts:
  - Nginx: port 8080
  - SSH: port 8022
  - Redis: port 8379
  - Flask test app: port 8888

If you have conflicts with the default PostgreSQL port (e.g., you already have PostgreSQL running locally), you can change it:

```/dev/null/custom-port.sh#L1-2
# Run tests with a custom PostgreSQL port
POSTGRES_PORT=5433 make test
```

> **Important:** When working with CI environments like GitHub Actions, be aware that the CI may already have services running on standard ports. The test environment is configured to handle both local development and CI contexts automatically.


## Security Considerations

- Some scan types require root/sudo privileges
- Network scanning should only be performed on networks you own or have permission to scan
- Aggressive scanning can be detected by IDS/IPS systems
- Version detection and script scanning may trigger security alerts

## Contributing

### Development Setup

1. Fork the repository
2. Clone your fork and navigate to the project directory
3. Set up the complete development environment:
   ```bash
   make setup-hooks    # Configure Git hooks for code quality
   make setup-dev-db   # Set up PostgreSQL development database
   ```

4. Verify everything works:
   ```bash
   make ci             # Run full CI pipeline locally
   ```

This single command runs quality checks, tests, and builds the project to ensure everything is working correctly.

### Workflow

1. Create your feature branch (`git checkout -b feature/amazing-feature`)
2. Make your changes
3. Run the full CI pipeline: `make ci`
   - This runs quality checks, tests, and builds
   - The pre-commit hook will also run linting checks automatically
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

### Code Quality

- All code must pass linting checks (enforced by pre-commit hooks)
- All tests must pass
- Follow conventional commit message format
- Use `make ci` to run the complete quality pipeline locally
- Use `DEBUG=true make test` for verbose test output when debugging


## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built using the [nmap](https://nmap.org/) security scanner
- Go nmap library: [github.com/Ullaakut/nmap](https://github.com/Ullaakut/nmap)

