# Contributing to Scanorama

This document outlines the contribution process and standards for the Scanorama project.

## Code Contributions

### Prerequisites

- Go 1.21 or later
- PostgreSQL for database testing
- golangci-lint for code quality checks

### Development Process

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run tests and linting
5. Submit a pull request

## Commit Message Style Guide

Write commit messages that are direct and factual. Avoid subjective adjectives.

### Format

```
<type>[optional scope]: <description>

[optional body]
```

### Types

- `feat`: Add new feature
- `fix`: Fix bug or issue
- `docs`: Update documentation
- `test`: Add or update tests
- `refactor`: Refactor code without changing functionality
- `chore`: Update build process, dependencies, or tooling

### Scopes

Use scopes to indicate the component affected:

- `handlers`: API handlers
- `db`: Database layer
- `api`: API infrastructure
- `cli`: Command line interface
- `config`: Configuration

### Rules

**Do:**
- Be direct and factual
- State exactly what was done
- Use specific action verbs (add, remove, update, fix, implement)
- Keep the first line under 72 characters
- Use imperative mood ("add feature" not "added feature")

**Don't:**
- Use subjective adjectives (comprehensive, robust, enhanced, improved)
- Include opinion or evaluation words
- Make it sound like marketing copy

### Examples

**Good:**
```
feat(handlers): use UUID strings for host API
fix(db): resolve database syntax errors and add missing helpers
test: add API integration tests
docs: add API documentation and frontend guides
refactor(handlers): extract common pagination logic
```

**Avoid:**
```
feat(handlers): comprehensively improve host API with enhanced UUID support
fix(db): resolve critical database issues and add robust helper functions
test: add comprehensive API integration tests
docs: add extensive API documentation with detailed guides
refactor(handlers): significantly improve code organization
```

## Code Standards

### Go Style

- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and under 100 lines when possible

### Linting

All code must pass linting checks:

```bash
make lint
```

Common issues to avoid:
- Unchecked error returns
- Functions over 100 lines
- Unused variables or imports
- Missing documentation for exported functions

### Testing

- Add tests for new functionality
- Update existing tests when modifying behavior
- Ensure tests pass before submitting PR

Run tests:
```bash
go test ./...
```

Run tests with coverage:
```bash
make test-coverage
```

## Database Changes

### Migrations

- Create migration files in `internal/db/`
- Use sequential numbering: `001_initial_schema.sql`, `002_add_indexes.sql`
- Include both up and down migrations when possible
- Test migrations on sample data

### Schema Changes

- Ensure backward compatibility when possible
- Document breaking changes in PR description
- Update model structs in `internal/db/models.go`

## Pull Request Process

### Before Submitting

1. Ensure all tests pass
2. Run linting and fix any issues
3. Update documentation if needed
4. Write clear commit messages following the style guide

### PR Description

Include:
- Summary of changes
- Motivation for the change
- Any breaking changes
- Testing performed

### Review Process

- All PRs require at least one review
- Address review feedback promptly
- Maintain clean commit history
- Squash related commits when appropriate

## API Changes

### Backward Compatibility

- Avoid breaking existing API endpoints
- Use versioning for major changes
- Document deprecations clearly

### Documentation

- Update API documentation in `docs/api/`
- Include examples for new endpoints
- Update OpenAPI specifications if applicable

## Security

### Reporting Issues

Report security vulnerabilities privately to the maintainers.

### Code Security

- Validate all input data
- Use parameterized queries for database operations
- Implement proper authentication and authorization
- Follow OWASP guidelines for web applications

## Getting Help

- Check existing issues and documentation first
- Use GitHub discussions for questions
- Join community channels (if available)
- Be specific when reporting bugs or requesting features

## License

By contributing to Scanorama, you agree that your contributions will be licensed under the project's license terms.