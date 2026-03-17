# Scanorama

[![CI](https://github.com/anstrom/scanorama/actions/workflows/main.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/main.yml)
[![Security](https://github.com/anstrom/scanorama/actions/workflows/security.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/security.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anstrom/scanorama)](https://goreportcard.com/report/github.com/anstrom/scanorama)
[![codecov](https://codecov.io/gh/anstrom/scanorama/branch/main/graph/badge.svg)](https://codecov.io/gh/anstrom/scanorama)

A network scanning and discovery daemon built on nmap with PostgreSQL persistence, a REST API, and automated scheduling. Scanorama runs continuously in the background, discovers hosts on your networks, tracks open ports and OS fingerprints, and exposes everything over a REST API with Swagger documentation.

## Requirements

- **nmap 7.0+** — required at runtime
- **PostgreSQL 14+** — required for persistence

## Quick Start

```bash
git clone https://github.com/anstrom/scanorama.git
cd scanorama
make build

cp config/environments/config.example.yaml config.yaml
# edit config.yaml — set database credentials and API key

./build/scanorama api
```

The API is available at `http://localhost:8080` and Swagger UI at `http://localhost:8080/swagger/`.

Create your first API key:

```bash
./build/scanorama apikeys create --name "My Key"
# → sk_live_abc123...

export SCANORAMA_API_KEY=sk_live_abc123...
```

## CLI Reference

### Discovery

```bash
# Discover a CIDR
scanorama discover 192.168.1.0/24

# Discover and save the network to the database
scanorama discover 192.168.1.0/24 --add --name "home-lan"

# Discover all configured networks
scanorama discover --configured-networks

# Auto-detect and scan all local interfaces
scanorama discover --all-networks

# Discovery methods: ping (default), tcp, arp
scanorama discover 10.0.0.0/24 --method tcp

# Enable OS fingerprinting during discovery
scanorama discover 192.168.1.0/24 --detect-os
```

### Scanning

```bash
# Scan specific targets
scanorama scan --targets 192.168.1.1
scanorama scan --targets "192.168.1.1,192.168.1.10" --ports "22,80,443"

# Scan all previously discovered live hosts
scanorama scan --live-hosts

# Filter live hosts by OS family
scanorama scan --live-hosts --os-family windows

# Use a built-in scan profile
scanorama scan --targets 192.168.1.1 --profile linux-server

# Scan types
scanorama scan --targets 192.168.1.1 --type connect       # TCP connect (default)
scanorama scan --targets 192.168.1.1 --type syn           # SYN stealth (requires root)
scanorama scan --targets 192.168.1.1 --type version       # Service version detection
scanorama scan --targets 192.168.1.1 --type aggressive    # OS + version + scripts
scanorama scan --targets 192.168.1.1 --type comprehensive # Full port range
```

### Scheduling

```bash
# List scheduled jobs
scanorama schedule list

# Schedule a weekly discovery (every Sunday at 02:00)
scanorama schedule add-discovery "weekly-sweep" "0 2 * * 0" "10.0.0.0/8"

# Schedule a scan every 6 hours against live hosts
scanorama schedule add-scan "frequent-scan" "0 */6 * * *" --live-hosts

# Remove a scheduled job
scanorama schedule remove weekly-sweep
```

### Running as a Daemon

```bash
scanorama daemon start   # Start background daemon with API server
scanorama daemon status  # Check status
scanorama daemon stop    # Stop daemon

scanorama api            # Run API server in the foreground
```

## Configuration

```bash
cp config/environments/config.example.yaml config.yaml
```

```yaml
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: your_secure_password
  ssl_mode: prefer          # disable | require | verify-full

api:
  enabled: true
  listen_addr: "127.0.0.1"
  port: 8080
  auth_enabled: true

scanning:
  worker_pool_size: 10
  default_ports: "22,80,443,8080,8443"
  default_scan_type: connect
  enable_os_detection: false  # requires root

logging:
  level: info          # debug | info | warn | error
  format: text         # text | json
  output: stdout
```

### Environment Variables

| Variable | Description |
|---|---|
| `SCANORAMA_DB_HOST` | Database host |
| `SCANORAMA_DB_PORT` | Database port |
| `SCANORAMA_DB_NAME` | Database name |
| `SCANORAMA_DB_USER` | Database username |
| `SCANORAMA_DB_PASSWORD` | Database password |
| `SCANORAMA_DB_SSLMODE` | SSL mode |
| `SCANORAMA_API_KEY` | API key for CLI and client authentication |
| `SCANORAMA_LOG_LEVEL` | Log level |
| `SCANORAMA_LOG_FORMAT` | Log format (`text` or `json`) |

## API

All endpoints require an `X-API-Key` header except `/health`.

```
GET    /health

GET    /api/v1/hosts
GET    /api/v1/hosts/:id
GET    /api/v1/hosts/:id/scans

GET    /api/v1/scans
POST   /api/v1/scans
GET    /api/v1/scans/:id
PUT    /api/v1/scans/:id
DELETE /api/v1/scans/:id
GET    /api/v1/scans/:id/results

GET    /api/v1/profiles
POST   /api/v1/profiles
GET    /api/v1/profiles/:id
PUT    /api/v1/profiles/:id
DELETE /api/v1/profiles/:id

GET    /api/v1/networks
POST   /api/v1/networks
DELETE /api/v1/networks/:id

GET    /api/v1/schedules
POST   /api/v1/schedules
GET    /api/v1/schedules/:id
PUT    /api/v1/schedules/:id
DELETE /api/v1/schedules/:id

GET    /api/v1/apikeys
POST   /api/v1/apikeys
DELETE /api/v1/apikeys/:id
```

Full interactive documentation at `http://localhost:8080/swagger/`.

### Example: submit a scan

```bash
curl -X POST http://localhost:8080/api/v1/scans \
  -H "X-API-Key: $SCANORAMA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "quick-check",
    "targets": ["192.168.1.0/24"],
    "scan_type": "connect",
    "ports": "22,80,443"
  }'

# Poll for completion
curl -s http://localhost:8080/api/v1/scans/<id> \
  -H "X-API-Key: $SCANORAMA_API_KEY" | jq .status

# Fetch results
curl -s "http://localhost:8080/api/v1/scans/<id>/results?limit=50" \
  -H "X-API-Key: $SCANORAMA_API_KEY" | jq .
```

## Docker

The production compose stack includes Scanorama, PostgreSQL, and Nginx, with optional Redis, Prometheus, and Grafana profiles.

```bash
cd docker

# Create secrets
echo "strong_db_password" > secrets/postgres_password.txt
echo "your-api-key"       > secrets/api_key.txt

# Start core services
docker compose up -d

# With monitoring (Prometheus + Grafana)
docker compose --profile monitoring up -d

# With Redis
docker compose --profile cache up -d
```

| Service | Port | Notes |
|---|---|---|
| Nginx (HTTP) | 80 | Reverse proxy |
| Nginx (HTTPS) | 443 | Reverse proxy (TLS) |
| Scanorama API | 8080 | Direct access |
| PostgreSQL | 5432 | Internal only |
| Prometheus | 9090 | `--profile monitoring` |
| Grafana | 3000 | `--profile monitoring` |

## License

MIT — see [LICENSE](LICENSE) for details.