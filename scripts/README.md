# Scripts Directory

Utility scripts for Scanorama development workflow.

## Available Scripts

### `setup-hooks.sh`
Set up Git pre-commit hooks for automatic code quality checks.
```bash
make setup-hooks
# or directly: ./scripts/setup-hooks.sh
```

### `setup-dev-db.sh`
Set up PostgreSQL development database with schema and test data.
```bash
make setup-dev-db
# or directly: ./scripts/setup-dev-db.sh
```

### `check-db.sh`
Check if a PostgreSQL database is available and accessible.
```bash
./scripts/check-db.sh        # Check with output
./scripts/check-db.sh -q     # Quiet mode
```

### `pre-commit-check.sh`
Quick verification script for build, linting, and tests.
```bash
./scripts/pre-commit-check.sh
```

## Typical Usage

```bash
# One-time setup
make setup-hooks
make setup-dev-db

# Daily development
make ci                      # Full pipeline
./scripts/pre-commit-check.sh  # Quick check before commit
```

All scripts are also integrated into the Makefile for convenience.