# Local GitHub Actions Testing with Act

This guide explains how to test GitHub Actions workflows locally using [act](https://github.com/nektos/act), eliminating the need to constantly push commits to test workflow changes.

## Why Test Locally?

Testing GitHub Actions locally provides several benefits:

- **Fast Feedback**: Get immediate results instead of waiting for remote runners
- **Save Resources**: Avoid consuming GitHub Actions minutes during development
- **Better Debugging**: Access to full container environment and logs
- **Offline Development**: Test workflows without internet connectivity
- **Faster Iteration**: Make changes and test immediately

## Prerequisites

### Required Tools

1. **Docker**: Act uses Docker to simulate GitHub Actions runners
   ```bash
   # Verify Docker is installed and running
   docker --version
   docker info
   ```

2. **act**: The GitHub Actions local testing tool

## Installation

### macOS/Linux (Homebrew)
```bash
brew install act
```

### Windows (Scoop)
```bash
scoop install act
```

### Linux (Shell Script)
```bash
curl --proto '=https' --tlsv1.2 -sSf https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
```

### Verify Installation
```bash
act --version
```

## Project Setup

Our project includes pre-configured files for local testing:

### Configuration Files

- **`.actrc`**: Act configuration with optimized settings
- **`.env.local.example`**: Template for environment variables
- **`.secrets.local.example`**: Template for secrets (never commit the actual `.secrets.local`)
- **`.github/events/`**: Sample GitHub event payloads

### Initial Setup

1. **Copy Environment Template**:
   ```bash
   cp .env.local.example .env.local
   # Edit .env.local with your specific values
   ```

2. **Copy Secrets Template** (if needed):
   ```bash
   cp .secrets.local.example .secrets.local
   # Edit .secrets.local with actual secrets for local testing
   ```

3. **Verify Configuration**:
   ```bash
   # Check act configuration
   cat .actrc
   
   # List available workflows
   act -l
   ```

## Basic Usage

### List All Workflows
```bash
# Show all workflows and their jobs
act -l

# Expected output:
# Stage Job ID             Job name                     Workflow name                 Workflow file              Events
# 0     docs-validation    API Documentation Validation Documentation Validation      docs-validation.yml        push,pull_request
# 0     docs-quality-...   Documentation Quality...     Documentation Validation      docs-validation.yml        push,pull_request
```

### Run Specific Workflows

```bash
# Run documentation validation workflow (triggered by push)
act push -W .github/workflows/docs-validation.yml

# Run documentation validation workflow (triggered by pull request)
act pull_request -W .github/workflows/docs-validation.yml

# Run specific job only
act -j docs-validation

# Run with custom event file
act --eventpath .github/events/pull_request.json
```

## Testing Documentation Workflows

### Quick Documentation Validation
```bash
# Test docs generation and validation
act push -j docs-validation

# Test with verbose output for debugging
act push -j docs-validation --verbose

# Test specific steps (dry run)
act push -j docs-validation --dryrun
```

### Full Documentation Pipeline
```bash
# Run complete documentation validation pipeline
act push -W .github/workflows/docs-validation.yml

# Run quality metrics job
act push -j docs-quality-metrics

# Run integration tests (requires database setup)
act pull_request -j docs-integration-test
```

### Testing Different Scenarios

1. **Test Push to Main**:
   ```bash
   act push --eventpath .github/events/push.json
   ```

2. **Test Pull Request**:
   ```bash
   act pull_request --eventpath .github/events/pull_request.json
   ```

3. **Test Manual Trigger**:
   ```bash
   act workflow_dispatch
   ```

## Advanced Usage

### Environment Customization

```bash
# Use custom environment file
act --env-file .env.custom push

# Override specific environment variables
act --env NODE_VERSION=20 --env GO_VERSION=1.21 push

# Use different platform
act --platform ubuntu-latest=catthehacker/ubuntu:act-22.04 push
```

### Debugging

```bash
# Enable verbose logging
act --verbose push

# Keep containers after run for inspection
act --reuse push

# Interactive debugging (shell into container)
act --shell push

# Bind mount workspace for file inspection
act --bind push
```

### Secrets and Security

```bash
# Use secrets file
act --secret-file .secrets.local push

# Override specific secrets
act --secret GITHUB_TOKEN=your_token push

# List secrets being used
act --list push
```

## Testing Specific Components

### Documentation Generation
```bash
# Test Swagger documentation generation
act push -j docs-validation --verbose | grep -A 20 "Generate API documentation"

# Test validation steps
act push -j docs-validation --verbose | grep -A 10 "Validate OpenAPI"
```

### Client Generation Testing
```bash
# Test client generation specifically
act push -j docs-validation --verbose | grep -A 15 "Test client generation"

# Test TypeScript client generation
act push -j docs-integration-test --verbose | grep -A 10 "Validate generated clients"
```

### Quality Metrics
```bash
# Test documentation quality assessment
act push -j docs-quality-metrics --verbose

# Check operation ID coverage
act push -j docs-validation --verbose | grep -A 5 "operation IDs"
```

## Common Workflows

### Before Pushing Changes
```bash
# Quick validation check
act push -j docs-validation

# Full pipeline test
act push -W .github/workflows/docs-validation.yml

# Check for any failures
echo $?  # Should be 0 for success
```

### Debugging Failed Workflows
```bash
# Run with maximum verbosity
act --verbose --debug push -j docs-validation

# Keep container for inspection
act --reuse push -j docs-validation

# Access container after failure
docker ps -a  # Find container ID
docker logs <container_id>
```

### Testing Changes Iteratively
```bash
# Make changes to workflow
vim .github/workflows/docs-validation.yml

# Test immediately
act push -j docs-validation --reuse

# Make more changes and test again
# (reuse flag speeds up subsequent runs)
```

## Troubleshooting

### Common Issues

1. **Docker Permission Errors**:
   ```bash
   # Add user to docker group (Linux)
   sudo usermod -aG docker $USER
   # Log out and back in
   ```

2. **Network Issues**:
   ```bash
   # Use host networking for service access
   act --network host push
   ```

3. **Resource Constraints**:
   ```bash
   # Limit container resources
   act --container-options "--memory=2g --cpus=1" push
   ```

4. **Missing Dependencies**:
   ```bash
   # Use larger runner image
   act --platform ubuntu-latest=catthehacker/ubuntu:full-latest push
   ```

### Platform-Specific Issues

**macOS with Apple Silicon**:
```bash
# Force x86_64 architecture
act --platform ubuntu-latest=catthehacker/ubuntu:act-latest-arm64 push
```

**Windows**:
```bash
# Use WSL2 backend for Docker
act --platform ubuntu-latest=catthehacker/ubuntu:act-latest push
```

### Debugging Checklist

1. Check Docker is running: `docker info`
2. Verify act configuration: `cat .actrc`
3. List available workflows: `act -l`
4. Test with dry run: `act --dryrun push`
5. Check logs: `act --verbose push`

## Performance Optimization

### Speed Up Testing

1. **Use Container Reuse**:
   ```bash
   # Add to .actrc
   --reuse
   ```

2. **Cache Dependencies**:
   ```bash
   # Pre-pull runner images
   docker pull catthehacker/ubuntu:act-latest
   ```

3. **Limit Resource Usage**:
   ```bash
   # Configure in .actrc
   --container-options "--memory=4g --cpus=2"
   ```

### Resource Management

```bash
# Clean up act containers
docker system prune -f

# Remove act images
docker images | grep act | awk '{print $3}' | xargs docker rmi

# Monitor resource usage
docker stats
```

## Integration with Development Workflow

### Pre-commit Testing
```bash
# Add to git hooks or Makefile
make test-workflows:
	act push -j docs-validation

# Test before committing
git add .
make test-workflows
git commit -m "Update documentation"
```

### CI/CD Validation
```bash
# Validate workflow changes
act push -W .github/workflows/docs-validation.yml

# Test multiple scenarios
for event in push pull_request workflow_dispatch; do
  echo "Testing $event event..."
  act $event -W .github/workflows/docs-validation.yml
done
```

### Documentation Development Cycle
```bash
# 1. Make documentation changes
vim docs/swagger_docs.go

# 2. Test generation
make docs-generate

# 3. Test validation locally
act push -j docs-validation

# 4. Test full pipeline
act push -W .github/workflows/docs-validation.yml

# 5. Commit if successful
git add . && git commit -m "Improve API documentation"
```

## Best Practices

### Security
- Never commit `.secrets.local` or `.env.local`
- Use dummy values in templates
- Rotate any secrets used in local testing
- Review `.actrc` for sensitive information

### Performance
- Use `--reuse` flag for iterative testing
- Pre-pull runner images
- Limit container resources appropriately
- Clean up unused containers regularly

### Debugging
- Start with `--dryrun` for workflow validation
- Use `--verbose` for detailed logging
- Test individual jobs before full workflows
- Keep containers with `--reuse` for inspection

### Workflow Development
- Test locally before pushing
- Use meaningful commit messages
- Document any act-specific configuration
- Keep event files updated with realistic data

## Additional Resources

- [act Documentation](https://nektosact.com/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Workflow Syntax](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)
- [Act GitHub Repository](https://github.com/nektos/act)

## Getting Help

If you encounter issues:

1. Check the troubleshooting section above
2. Review act logs with `--verbose`
3. Search [act issues](https://github.com/nektos/act/issues)
4. Check [act discussions](https://github.com/nektos/act/discussions)

## Project-Specific Notes

For the Scanorama project:

- Documentation workflows are optimized for npm-based validation
- Database integration tests require PostgreSQL setup
- Client generation tests validate TypeScript and JavaScript outputs
- Quality metrics track operation ID coverage and documentation completeness

Remember to keep your local testing environment updated and aligned with the remote GitHub Actions configuration.