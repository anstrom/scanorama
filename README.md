# Scanorama

[![CI](https://github.com/anstrom/scanorama/actions/workflows/main.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/main.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anstrom/scanorama)](https://goreportcard.com/report/github.com/anstrom/scanorama)
[![codecov](https://codecov.io/gh/anstrom/scanorama/branch/main/graph/badge.svg)](https://codecov.io/gh/anstrom/scanorama)

Advanced network scanning and discovery tool with REST API, database persistence, and web frontend.

## Features

- **Network Discovery**: Automated network scanning with nmap integration
- **REST API**: Full HTTP API for managing scans, hosts, and networks
- **Web Frontend**: React-based UI for network management
- **Scheduling**: Automated scan jobs with cron-like scheduling
- **Database**: PostgreSQL persistence with automatic migrations
- **Docker**: Containerized deployment ready

## Requirements

- Go 1.25.1+
- **nmap 7.0+** (required for scanning)
- PostgreSQL
- Node.js 18+ (for frontend development)

## Quick Start

### Development

```bash
# Clone and build
git clone https://github.com/anstrom/scanorama.git
cd scanorama
make build

# Start development database
make dev-db

# Start API server
./build/scanorama api --config config/environments/config.dev.yaml

# Start frontend (in another terminal)
make dev-frontend
```

### Production

```bash
# Build and run daemon with scheduling
make build
./build/scanorama daemon start --config config/environments/config.yaml
```

## Available Commands

### Core Commands
- `scanorama api` - Start API server (development)
- `scanorama daemon start` - Start full daemon with scheduling (production)
- `scanorama scan <target>` - Manual port scanning
- `scanorama discover <network>` - Network discovery
- `scanorama hosts` - Manage discovered hosts

### Development
- `make build` - Build application with version info
- `make test` - Run tests
- `make lint` - Run linters (Go + Frontend + Docs)
- `make dev-db` - Start development database (Docker)
- `make dev-frontend` - Start frontend development server
- `make stop-dev` - Stop all development services

## Configuration

Configuration files in `config/environments/`:
- `config.dev.yaml` - Development settings
- `config.yaml` - Production settings

Example:
```yaml
database:
  host: "localhost"
  port: 5432
  database: "scanorama_dev"
  username: "scanorama_dev"
  password: "dev_password"

api:
  enabled: true
  listen_addr: "127.0.0.1"
  port: 8080
```

## API Documentation

Once running, API documentation is available at:
- **Swagger UI**: http://localhost:8080/swagger/
- **Health Check**: http://localhost:8080/api/v1/health

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Web Frontend  │    │    REST API      │    │   PostgreSQL    │
│   (React/Vite)  │◄──►│   (Go/Gorilla)   │◄──►│   (Database)    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌──────────────────┐
                       │   nmap Engine    │
                       │   (Scanning)     │
                       └──────────────────┘
```

## Development vs Production

**Development** (`scanorama api`):
- API server only
- No background scheduling
- Manual scan operations
- Hot reload friendly

**Production** (`scanorama daemon`):
- Full daemon service
- Automated scheduled scans
- Background job processing
- Production monitoring

## License

MIT License - see [LICENSE](LICENSE) file for details.