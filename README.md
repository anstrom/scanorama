# Scanorama

A network scanner that finds hosts and open ports. Uses nmap to scan networks and stores results in a database.

## Requirements

- Go 1.24.6+
- nmap 7.0+
- PostgreSQL (for database storage)

## Quick Start

```bash
git clone https://github.com/anstrom/scanorama.git
cd scanorama
make setup-hooks
make setup-dev-db
make ci
```

## Usage

```bash
# Build the scanner
make build

# Discover hosts on a network
./scanorama discover 192.168.1.0/24

# Scan specific targets
./scanorama scan --targets localhost --ports 80,443,8080
./scanorama scan --targets 192.168.1.1,192.168.1.10 --ports 22,80,443

# View discovered hosts
./scanorama hosts
./scanorama hosts --status up
```

## Commands

- `discover <network>` - Find hosts on a network
- `scan --targets <hosts>` - Scan specific hosts for open ports
- `hosts` - List discovered hosts
- `version` - Show version info

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
2. Run `make setup-hooks` and `make setup-dev-db`
3. Make your changes
4. Run `make ci` to verify everything works
5. Commit and push
6. Create a pull request

## License

MIT License - see LICENSE file for details.