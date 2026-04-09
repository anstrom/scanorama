# Scanorama Documentation

## Reference

| File | Description |
|------|-------------|
| [`CONFIGURATION.md`](CONFIGURATION.md) | Full configuration reference: every setting, environment variable, validation rule, hot-reload behaviour, and example configs for dev/staging/production |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | Building from source, systemd service, database setup, security hardening, health endpoints, Prometheus scraping, troubleshooting |

## Planning

| File | Description |
|------|-------------|
| [`planning/ROADMAP.md`](planning/ROADMAP.md) | Milestone roadmap: v0.23 (complete) through v1.0, feature scope, dependency graph, tool integrations reference |

## Technical

| File | Description |
|------|-------------|
| [`technical/architecture/system-overview.md`](technical/architecture/system-overview.md) | Component diagram, package reference, external dependencies |
| [`technical/architecture/data-flow.md`](technical/architecture/data-flow.md) | Request lifecycle, scan and discovery pipelines, WebSocket event flow, migration flow |
| [`technical/architecture/scheduling-flow.md`](technical/architecture/scheduling-flow.md) | Cron → job → scan → results state machine, concurrency model |
| [`technical/architecture/logging.md`](technical/architecture/logging.md) | Logging, metrics, and worker pool architecture |
| [`technical/testing.md`](technical/testing.md) | Test structure, running tests, writing DB and scanning tests, CI integration |

## API

The authoritative API reference is the live Swagger UI served by the backend:

```
http://localhost:8080/swagger/
```

The OpenAPI spec is generated from source annotations (`swag generate`) and lives in [`swagger/`](swagger/).