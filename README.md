# Scanorama

[![CI](https://github.com/anstrom/scanorama/actions/workflows/main.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/main.yml)
[![Security](https://github.com/anstrom/scanorama/actions/workflows/security.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/security.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anstrom/scanorama)](https://goreportcard.com/report/github.com/anstrom/scanorama)
[![codecov](https://codecov.io/gh/anstrom/scanorama/branch/main/graph/badge.svg)](https://codecov.io/gh/anstrom/scanorama)

A network scanning and discovery tool built on nmap with database persistence, REST API, and automated scheduling capabilities.

## Quick Start

### Running Tests

```bash
# Start test database and run all tests
make test

# Run only unit tests (no database needed)
make test-unit

# Generate coverage report
make coverage

# Start database manually (for development)
make db-up

# Stop database
make db-down
```

### Database Management

The test database runs in Docker and is managed automatically:
- `make test` - Starts DB, runs tests, stops DB
- `make db-up` - Start test database
- `make db-down` - Stop test database
- `make db-reset` - Reset database (down + up)
- `make db-shell` - Connect with psql

### Development

```bash
# Format code
make fmt

# Run linter
make lint

# Build binary
make build

# See all targets
make help
```

## Features

- **Network Discovery**: Host discovery and port scanning using nmap
- **Multiple Scan Types**: Connect, SYN, version detection, aggressive, stealth
- **Database Integration**: PostgreSQL persistence with automatic migrations
- **REST API**: RESTful API with Swagger documentation
- **Scheduling**: Automated scan jobs with cron-like scheduling
- **Monitoring**: Structured logging, metrics, and health checks
- **Docker Support**: Containerized deployment ready

## Requirements

- Go 1.25.3+
- **nmap 7.0+** (required)
- PostgreSQL (for persistence)

## Quick Start

```bash
git clone https://github.com/anstrom/scanorama.git
cd scanorama
make setup-dev-db   # Initialize database
make build          # Build binary
```

## Usage

```bash
# Discover hosts on a network
./scanorama discover 192.168.1.0/24

# Scan specific targets
./scanorama scan --targets localhost --ports 80,443,8080
./scanorama scan --targets 192.168.1.1 --type aggressive

# View discovered hosts
./scanorama hosts

# Run as daemon with API server
./scanorama daemon
```

### Scan Types

- `connect` - TCP connect scanning (default)
- `syn` - SYN stealth scanning (requires privileges)
- `version` - Service version detection
- `comprehensive` - Full port range scanning
- `aggressive` - OS detection + version scanning + scripts
- `stealth` - Slow, evasive scanning

## API

Start the daemon and access the REST API:

```bash
./scanorama daemon
# API available at http://localhost:8080
# Swagger docs at http://localhost:8080/swagger/
```

## Configuration

Create `config.yaml`:

```yaml
database:
  host: localhost
  port: 5432
  name: scanorama
  user: scanorama
  password: your_password

api:
  host: 0.0.0.0
  port: 8080

logging:
  level: info
  format: json
```

## Development

```bash
make setup-hooks     # Set up Git hooks
make ci              # Run full CI pipeline
make test            # Run tests
make coverage        # Generate coverage reports
make lint            # Run linter
```

## Docker

```bash
docker run -p 8080:8080 ghcr.io/anstrom/scanorama:latest
```

## Contributing

1. Fork and clone the repository
2. Run `make setup-hooks` to install Git hooks
3. Run `make setup-dev-db` to set up development database
4. Make your changes with tests
5. Run `make ci` to ensure quality checks pass
6. Create a pull request

See [Contributing Guidelines](CONTRIBUTING.md) for more details.

## License

MIT License - see [LICENSE](LICENSE) file for details.