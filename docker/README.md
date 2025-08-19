# Scanorama Docker Configuration

This directory contains Docker Compose configurations for different environments and deployment scenarios.

## Directory Structure

```
docker/
├── README.md                    # This file
├── docker-compose.yml           # Production deployment
├── docker-compose.dev.yml       # Development environment
├── docker-compose.test.yml      # Testing environment
├── init-scripts/               # Database initialization scripts
├── nginx/                      # Nginx configuration files
├── secrets/                    # Secret files (not in git)
└── monitoring/                 # Monitoring configuration
```

## Compose Files Overview

### `docker-compose.yml` - Production
- **Database**: PostgreSQL 17 with persistent storage
- **Application**: Scanorama with full production configuration
- **Reverse Proxy**: Nginx with SSL/TLS support
- **Caching**: Redis (optional, use `--profile cache`)
- **Monitoring**: Prometheus + Grafana (optional, use `--profile monitoring`)
- **Security**: Uses Docker secrets for sensitive data
- **Networks**: Separate internal/external networks for security

### `docker-compose.dev.yml` - Development
- **Database**: PostgreSQL 17 with development data
- **Tools**: pgAdmin for database management (optional, use `--profile tools`)
- **Test Targets**: Nginx and SSH servers for testing scans (use `--profile targets`)
- **Cache**: Redis for development (optional, use `--profile cache`)
- **Networking**: Single development network
- **Volumes**: Named volumes for data persistence

### `docker-compose.test.yml` - Testing
- **Database**: PostgreSQL 17 with test configuration
- **Storage**: tmpfs for fast, ephemeral testing
- **Minimal**: Only essential services for running tests

## Quick Start

### Development Environment

```bash
# Start basic development services (PostgreSQL only)
docker compose -f docker/docker-compose.dev.yml up -d

# Start with all development tools
docker compose -f docker/docker-compose.dev.yml --profile tools --profile targets --profile cache up -d

# Stop development environment
docker compose -f docker/docker-compose.dev.yml down
```

### Testing Environment

```bash
# Start test services (used by make test)
docker compose -f docker/docker-compose.test.yml up -d --wait

# Stop test services
docker compose -f docker/docker-compose.test.yml down
```

### Production Deployment

```bash
# Set up secrets first (see Secrets section below)
mkdir -p docker/secrets/
echo "your_secure_password" > docker/secrets/postgres_password.txt
echo "your_api_key" > docker/secrets/api_key.txt
echo "redis_password" > docker/secrets/redis_password.txt
echo "grafana_admin_password" > docker/secrets/grafana_password.txt

# Start core services
docker compose -f docker/docker-compose.yml up -d

# Start with caching and monitoring
docker compose -f docker/docker-compose.yml --profile cache --profile monitoring up -d
```

## Environment Variables

### Production Environment Variables
The production compose file expects these environment variables to be set or will use defaults:

- `SCANORAMA_DB_HOST=postgres`
- `SCANORAMA_DB_PORT=5432`
- `SCANORAMA_DB_NAME=scanorama`
- `SCANORAMA_API_ENABLED=true`
- `SCANORAMA_API_HOST=0.0.0.0`
- `SCANORAMA_API_PORT=8080`

### Development Overrides
You can create a `.env` file in the docker directory to override defaults:

```bash
# docker/.env
POSTGRES_PASSWORD=my_dev_password
SCANORAMA_LOG_LEVEL=debug
```

## Secrets Management

### Production Secrets
Create these files in `docker/secrets/` (ensure they're not in git):

```bash
# Database password
echo "strong_postgres_password" > docker/secrets/postgres_password.txt

# API authentication key  
echo "your-secret-api-key-here" > docker/secrets/api_key.txt

# Redis password (if using cache profile)
echo "redis_secure_password" > docker/secrets/redis_password.txt

# Grafana admin password (if using monitoring profile)
echo "grafana_admin_password" > docker/secrets/grafana_password.txt
```

### Security Notes
- Never commit secrets to git
- Use strong, unique passwords
- Consider using external secret management in production
- Rotate secrets regularly

## Profiles

### Available Profiles

| Profile | Services | Use Case |
|---------|----------|----------|
| `tools` | pgAdmin | Database management UI |
| `targets` | nginx-test, ssh-test | Test targets for scanning |
| `cache` | Redis | Caching layer |
| `monitoring` | Prometheus, Grafana | Metrics and dashboards |

### Using Profiles

```bash
# Start specific profiles
docker compose -f docker/docker-compose.dev.yml --profile tools up -d

# Multiple profiles
docker compose -f docker/docker-compose.yml --profile cache --profile monitoring up -d

# List available profiles
docker compose -f docker/docker-compose.dev.yml config --profiles
```

## Networking

### Development Network
- **Network**: `scanorama-dev-network`
- **Type**: Bridge network
- **Access**: All services can communicate

### Production Networks
- **Internal**: `scanorama-internal-network` (backend services)
- **External**: `scanorama-external-network` (public-facing services)
- **Security**: Internal network is isolated from external access

## Volumes

### Development Volumes
- `postgres_dev_data`: PostgreSQL development data
- `pgadmin_dev_data`: pgAdmin configuration
- `redis_dev_data`: Redis development data

### Production Volumes
- `postgres_data`: PostgreSQL production data
- `scanorama_data`: Application data
- `scanorama_logs`: Application logs
- `prometheus_data`: Metrics storage
- `grafana_data`: Dashboard configuration

## Health Checks

All services include health checks with appropriate timeouts:

- **PostgreSQL**: `pg_isready` command
- **Scanorama**: HTTP health endpoint
- **Nginx**: Configuration validation
- **Redis**: `PING` command

## Resource Limits

Production services have resource limits configured:

- **PostgreSQL**: 1GB RAM, 1 CPU
- **Scanorama**: 2GB RAM, 2 CPUs  
- **Nginx**: 256MB RAM, 0.5 CPU
- **Redis**: 512MB RAM, 0.5 CPU

## Common Commands

### Development
```bash
# Start development environment
make test-up

# View logs
docker compose -f docker/docker-compose.dev.yml logs -f

# Access database
docker compose -f docker/docker-compose.dev.yml exec postgres psql -U scanorama_dev -d scanorama_dev

# Restart a service
docker compose -f docker/docker-compose.dev.yml restart postgres
```

### Production
```bash
# Deploy production stack
docker compose -f docker/docker-compose.yml up -d

# Scale the application
docker compose -f docker/docker-compose.yml up -d --scale scanorama=3

# Update services
docker compose -f docker/docker-compose.yml pull
docker compose -f docker/docker-compose.yml up -d

# Backup database
docker compose -f docker/docker-compose.yml exec postgres pg_dump -U scanorama scanorama > backup.sql
```

### Cleanup
```bash
# Remove all containers and networks
docker compose -f docker/docker-compose.dev.yml down

# Remove containers, networks, and volumes
docker compose -f docker/docker-compose.dev.yml down -v

# Remove everything including images
docker compose -f docker/docker-compose.dev.yml down -v --rmi all
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Change port mappings if 5432, 8080, etc. are in use
2. **Permission errors**: Ensure Docker daemon is running and user has permissions
3. **Out of disk space**: Clean up with `docker system prune`
4. **Service won't start**: Check logs with `docker compose logs <service>`

### Database Connection Issues
```bash
# Test database connectivity
docker compose -f docker/docker-compose.dev.yml exec postgres pg_isready -U scanorama_dev

# Access database directly
docker compose -f docker/docker-compose.dev.yml exec postgres psql -U scanorama_dev -d scanorama_dev
```

### Performance Issues
```bash
# Monitor resource usage
docker stats

# Check service health
docker compose -f docker/docker-compose.yml ps
```

## Integration with Makefile

The project Makefile integrates with these Docker Compose files:

- `make test`: Uses `docker-compose.test.yml`
- `make test-up`: Starts test services
- `make test-down`: Stops test services

## Security Considerations

1. **Secrets**: Use Docker secrets, never environment variables for sensitive data
2. **Networks**: Use separate networks for internal/external communication
3. **User permissions**: Run containers as non-root where possible
4. **Resource limits**: Set appropriate CPU/memory limits
5. **Health checks**: Implement proper health monitoring
6. **SSL/TLS**: Configure HTTPS for production deployments

## Monitoring and Observability

When using the monitoring profile:

- **Prometheus**: Available at http://localhost:9090
- **Grafana**: Available at http://localhost:3000
- **Metrics**: Scanorama exposes metrics at `/metrics` endpoint

## Backup and Recovery

### Database Backup
```bash
# Create backup
docker compose -f docker/docker-compose.yml exec postgres pg_dump -U scanorama -Fc scanorama > scanorama_backup.dump

# Restore backup
docker compose -f docker/docker-compose.yml exec -T postgres pg_restore -U scanorama -d scanorama < scanorama_backup.dump
```

### Volume Backup
```bash
# Backup volumes
docker run --rm -v postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/postgres_data.tar.gz /data
```
