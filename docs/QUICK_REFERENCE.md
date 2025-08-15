# Quick Testing Reference

## 🚀 One-Time Setup

```bash
# Install act (if not already installed)
brew install act  # macOS/Linux
# or
scoop install act  # Windows

# Set up testing environment
make act-setup

# Verify everything works
make act-check-setup
```

## ⚡ Daily Commands

### Documentation Testing (Most Common)
```bash
# Test docs locally (recommended before every commit)
make act-local-docs

# Quick validation only
make docs-validate

# Advanced linting
make docs-spectral

# Generate fresh docs
make docs-generate
```

### GitHub Actions Testing
```bash
# List all workflows
make act-list

# Validate workflow syntax
make act-validate

# Test docs workflow (dry run)
act push --dryrun -j docs-validation

# Full workflow test (if you have tokens)
make act-docs
```

## 🔧 Troubleshooting

### Quick Fixes
```bash
# Check setup
make act-check-setup

# Clean up containers
make act-clean

# Restart Docker if needed
docker info
```

### Common Issues
| Problem | Solution |
|---------|----------|
| `act not found` | Run `brew install act` |
| `Docker not running` | Start Docker Desktop |
| `Permission denied` | Check Docker permissions |
| `.env.local missing` | Run `make act-setup` |
| Workflow syntax error | Run `make act-validate` |

## 📋 Workflow Checklist

### Before Committing
- [ ] `make act-local-docs` - Test docs pipeline
- [ ] `make act-validate` - Check workflow syntax
- [ ] Review any warnings from Vacuum linting

### When Changing Workflows
- [ ] `make act-validate` - Syntax check
- [ ] `act --dryrun push` - Test structure
- [ ] `make act-local-docs` - Verify docs still work

### When Debugging
- [ ] `make act-check-setup` - Verify configuration
- [ ] `make act-debug` - Run with full debugging
- [ ] `act --verbose push --dryrun` - Detailed dry run

## 📊 Quality Metrics

After running `make act-local-docs`, expect:
- ✅ **0 validation errors** (Redocly)
- ✅ **0 critical warnings** (Vacuum)
- ✅ **100% operation ID coverage** (35/35 endpoints)
- ✅ **Client generation success** (JS + TS)

## 🎯 Pro Tips

1. **Speed**: Use `make act-local-docs` for 95% of testing needs
2. **Debugging**: Add `--verbose` to any act command for details
3. **Performance**: Act reuses containers automatically
4. **Safety**: All sensitive files are in `.gitignore`
5. **Updates**: Vacuum provides better linting than old Spectral

## 🆘 Get Help

```bash
# Show all make targets
make help

# Act-specific help
make act-help

# Detailed documentation
less docs/LOCAL_TESTING.md
less docs/TESTING_STATUS.md
```

## 🔗 Quick Links

- **Full Guide**: `docs/LOCAL_TESTING.md`
- **Status Report**: `docs/TESTING_STATUS.md`
- **Act Documentation**: https://nektosact.com/
- **Vacuum Linting**: https://quobix.com/vacuum/