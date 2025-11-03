# Claude AI Assistant Instructions for Scanorama Project

## Project Overview
- **Project**: Scanorama - Network scanning and discovery tool
- **Language**: Go 1.25.3
- **Current Version**: 0.2.0 (API refactoring in progress)
- **Root Directory**: `scanorama/`

## Diagnostic Workflow Priority

### 1. Real-time Diagnostics (Primary - Zed Editor)
When the user is using Zed editor:
- **Use Zed's real-time diagnostics** with `gopls` integration
- **Problems panel shortcuts**: 
  - macOS: `Cmd+Shift+M`
  - Linux/Windows: `Ctrl+Shift+M`
- **Live error checking**: Red underlines = errors, Yellow = warnings
- **Quick fixes**: `Cmd+.` (macOS) or `Ctrl+.` (others)

### 2. Make-based Workflow (Primary for CI/fixes)
Always use make targets for linting and code quality:

```bash
# CODE QUALITY COMMANDS (use in this order)
make lint-fix      # Auto-fix linting issues (alias for format)
make lint          # Check remaining issues after auto-fix
make quality       # Run lint + format + security checks
make ci            # Full CI pipeline (quality + test + build)

# DIAGNOSTIC COMMANDS
make test          # Run tests with proper database setup
make build         # Compile and check for build errors
make security      # Run security vulnerability scans

# DEVELOPMENT SETUP
make setup-hooks   # Set up git hooks for quality checks
make setup-dev-db  # Set up development database
```

### 3. Diagnostics Tool (Backup/Comprehensive)
Use when Zed is not available or for project-wide checks:
```bash
diagnostics()                                    # Check entire project
diagnostics(path: "scanorama/internal/api/...")  # Check specific file
```

### 4. Go Toolchain (Final validation)
```bash
go mod tidy        # Clean up module dependencies
go vet ./...       # Static analysis
go build ./...     # Compilation check
go test ./...      # Test execution
```

## Code Quality Standards

### Linting Configuration
- **Tool**: golangci-lint with comprehensive rule set
- **Config**: `.golangci.yml` (120 char line limit, cyclomatic complexity 15)
- **Enabled linters**: errcheck, gosec, govet, staticcheck, gocritic, and more
- **Auto-fix capability**: `make format` fixes many issues automatically

### Before Any Code Changes
1. **Check current state**: Run `make lint` to see existing issues
2. **Make incremental changes**: Fix one issue at a time when possible
3. **Validate changes**: Run `make lint` after each fix
4. **Final validation**: Run `make ci` before committing

### Issue Resolution Order
1. **Syntax errors** (blocking compilation)
2. **Import/dependency issues** 
3. **Type mismatches**
4. **Security vulnerabilities** (gosec findings)
5. **Code quality warnings**
6. **Style/formatting issues**

## Project-Specific Issues (Current State)

### Known Critical Issues
1. **Metrics Type Mismatch**: 
   - Problem: `metrics.Manager` vs `metrics.Registry` inconsistency
   - Files: `internal/api/handlers/*.go`
   - Fix: Change all `metrics.Manager` to `metrics.Registry`

2. **Missing Database Methods**:
   - Problem: Handlers expect methods not implemented in `db.DB`
   - Files: `internal/db/database.go`
   - Methods needed: `ListScans`, `CreateScan`, `GetScan`, etc.

3. **Import Dependencies**:
   - Always run `go mod tidy` after adding/removing imports
   - Dependencies are properly configured in `go.mod`

### Common Error Patterns
- **"undefined: metrics.Manager"** → Change to `metrics.Registry`
- **"has no field or method X"** → Implement missing database methods
- **"imported and not used"** → Remove unused imports or use `make format`
- **"cannot use X as Y"** → Type conversion/interface issues

## Development Workflow

### Starting Work
```bash
# Set up environment (first time only)
make setup-hooks
make setup-dev-db

# Before making changes (clean slate approach)
make lint-fix      # Auto-fix existing issues first
make lint          # Check what needs manual attention
```

### During Development
```bash
# Use Zed for real-time feedback
# OR check specific files
diagnostics(path: "scanorama/internal/api/...")

# Efficient issue resolution
make lint-fix      # Auto-fix 60-80% of issues immediately
make lint          # Check remaining manual fixes needed
```

### Before Committing
```bash
make ci            # Full pipeline check
# This runs: quality + test + build
```

### Fixing Code Quality Issues (Sequential Workflow)
```bash
# Step 1: Auto-fix what can be automated (saves 60-80% of manual work)
make lint-fix      # Handles formatting, imports, simple fixes

# Step 2: Check what remains  
make lint          # Focus on issues requiring analysis

# Step 3: Manual fixes for remaining issues
# (usually type issues, missing methods, logic errors)

# Step 4: Validate functionality
make ci            # Full pipeline validation
```

## Database Development

### Testing with Database
```bash
# The Makefile automatically handles database setup
make test          # Will start containers if needed
make setup-dev-db  # Manual database setup
```

### Database-related Issues
- Check `internal/db/database.go` for missing methods
- Repository pattern: Use `*Repository` structs for data access
- Migrations: SQL files in `internal/db/`

## Error Handling Patterns

### API Error Responses
- Use standardized `ErrorResponse` struct
- Include request IDs for tracing
- Log errors with context (method, path, etc.)

### Database Errors
- Wrap with `fmt.Errorf` for context
- Handle `sql.ErrNoRows` specifically
- Use transactions for batch operations

## Build and Deployment

### Build Targets
```bash
make build         # Standard build
make version       # Show version info
make clean         # Clean artifacts
```

### Integration Testing
```bash
make ci            # Full pipeline
make test          # Just tests
make quality       # Just quality checks
```

## Troubleshooting

### Language Server Issues
1. Restart language server in Zed: `Cmd+Shift+P` → "Restart Language Server"
2. Run `go mod tidy`
3. Check Go installation: `go version`

### Persistent Build Issues
1. Auto-fix first: `make lint-fix`
2. Clean everything: `make clean`
3. Tidy dependencies: `go mod tidy`
4. Run fresh build: `make build`

### Database Connection Issues
1. Check database status: `./scripts/check-db.sh`
2. Restart containers: `make setup-dev-db`
3. Check port conflicts: Default is 5432

## Git Workflow and Commit Strategy

### Conventional Commits
Use conventional commit format for all commits:

```bash
# Format: <type>(<scope>): <description>
feat(api): add health check endpoint
fix(db): resolve connection pool timeout issue
docs(readme): update installation instructions
refactor(handlers): extract common validation logic
test(integration): add database migration tests
chore(deps): update golang.org/x/net to v0.17.0
```

### Common Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Formatting changes (no code logic change)
- `refactor`: Code refactoring (no behavior change)
- `test`: Adding or updating tests
- `chore`: Maintenance tasks, dependency updates
- `perf`: Performance improvements
- `ci`: CI/CD changes

### Scope Examples:
- `api`, `db`, `scanner`, `config`, `metrics`, `handlers`, `middleware`

### Commit Message Style Guidelines

#### Use Precise, Technical Language
```bash
# ❌ Vague/excessive adjectives
git commit -m "feat(api): add new health endpoint"
git commit -m "fix(db): make connection handling better"
git commit -m "refactor(handlers): improve terrible code quality"

# ✅ Precise, technical descriptions
git commit -m "feat(api): add health endpoint with database connectivity check"
git commit -m "fix(db): resolve connection pool timeout after 30s idle"
git commit -m "refactor(handlers): extract error response helper function"
```

#### Imperative Mood, Present Tense
```bash
# ❌ Past tense/descriptive
git commit -m "feat(scanner): added port scanning functionality"
git commit -m "fix(api): fixed the broken endpoint"
git commit -m "docs: updated the documentation"

# ✅ Imperative mood (complete this sentence: "If applied, this commit will...")
git commit -m "feat(scanner): implement TCP port scanning with timeout"
git commit -m "fix(api): resolve nil pointer in request validation"
git commit -m "docs: document API authentication requirements"
```

#### Be Specific About Changes
```bash
# ❌ Generic descriptions
git commit -m "fix(handlers): fix issues"
git commit -m "refactor(db): clean up code"
git commit -m "chore: update dependencies"

# ✅ Specific technical details
git commit -m "fix(handlers): resolve metrics.Manager type mismatch"
git commit -m "refactor(db): extract repository pattern for scan operations"
git commit -m "chore(deps): update gorilla/mux to v1.8.1 for security patch"
```

#### Avoid Marketing Language
```bash
# ❌ Marketing/emotional language
git commit -m "feat(api): add concurrent scan capabilities"
git commit -m "fix(db): resolve query performance issues"
git commit -m "refactor: extract common validation logic"

# ✅ Technical, factual language
git commit -m "feat(api): implement concurrent scanning with worker pools"
git commit -m "fix(db): optimize query performance with connection pooling"
git commit -m "refactor: extract common validation logic to middleware"
```

#### Never Use Adjectives in Commit Messages
```bash
# ❌ Adjective-heavy commits (lazy writing)
git commit -m "feat: add comprehensive test coverage"
git commit -m "fix: improve excellent error handling"
git commit -m "test: add extensive middleware tests"
git commit -m "docs: create detailed documentation"

# ✅ Adjective-free commits (precise, factual)
git commit -m "feat: add test coverage for API handlers"
git commit -m "fix: resolve nil pointer in error handling"
git commit -m "test: add middleware CORS and logging tests"
git commit -m "docs: document authentication flow"
```

### Rebase Strategy (Avoid Fix Commit Noise)

#### During Development - Use Fixup Commits
```bash
# Make initial commit
git add .
git commit -m "feat(api): implement scan endpoints"

# Later, found issues via make lint-fix
git add .
git commit --fixup=HEAD -m "fix formatting and linting issues"

# Found more issues
git add .
git commit --fixup=HEAD~1 -m "fix type mismatches"

# Before pushing - clean up history
git rebase -i --autosquash HEAD~3
```

#### Alternative: Amend Strategy (for recent commits)
```bash
# Make changes
git add .
git commit -m "feat(api): implement scan endpoints"

# Found lint issues immediately  
make lint-fix
git add .
git commit --amend --no-edit  # Fold into previous commit
```

#### For Older Commits - Interactive Rebase
```bash
# Fix issues in commits that are 2-3 commits back
git add .
git commit --fixup=<commit-hash>

# Or use squash for more control
git commit --squash=<commit-hash> -m "additional context about fix"

# Clean up before pushing
git rebase -i --autosquash HEAD~5
```

### Recommended Development Flow
```bash
# 1. Start feature branch  
git checkout -b feat/api-pagination

# 2. Make meaningful commit with precise language
git add .
git commit -m "feat(api): implement pagination for scan results endpoint"

# 3. Quick quality check
make lint-fix && make lint

# 4. If issues found, fixup immediately (avoid "fix lint" commits)
git add .
git commit --fixup=HEAD

# 5. Continue development with technical precision...
git commit -m "feat(api): add filtering by scan status and date range"

# 6. Before pushing, clean history
git rebase -i --autosquash origin/main

# 7. Push clean, professional history
git push origin feat/api-pagination
```

### Anti-Patterns to Avoid

#### Noise Commits (Yak Shaving History)
```bash
# ❌ Don't create noise commits
git commit -m "fix lint"
git commit -m "fix formatting" 
git commit -m "oops fix typo"
git commit -m "actually fix the thing"
git commit -m "make it better"

# ✅ Use fixup/amend instead
git commit --fixup=HEAD~2
git rebase -i --autosquash HEAD~4
```

#### Imprecise Language
```bash
# ❌ Vague, adjective-heavy messages
git commit -m "fix(api): improve bad error handling"
git commit -m "feat(db): add better connection management"
git commit -m "refactor: make code cleaner and nicer"

# ✅ Precise, technical descriptions
git commit -m "fix(api): return 400 for malformed JSON requests"
git commit -m "feat(db): implement connection pooling with 25 max connections"
git commit -m "refactor: extract response writing to helper function"
```

### Quality Gate Integration
```bash
# Before any push
make ci            # Validate all checks pass
git rebase -i --autosquash origin/main  # Clean history
git push origin feature-branch
```

### Emergency Fixes
```bash
# For urgent fixes on existing commits
git add .
git commit --fixup=<urgent-commit-hash>
git rebase -i --autosquash <urgent-commit-hash>~1
git push --force-with-lease origin branch-name
```

### Make + Git Workflow Integration

### Development Cycle (Recommended)
```bash
# Start feature
git checkout -b feat/new-feature

# 1. Write code with meaningful chunks
# 2. Commit semantic unit
git add .
git commit -m "feat(api): implement scan result filtering"

# 3. Auto-fix quality issues immediately
make lint-fix

# 4. If auto-fix made changes, fold into commit
git add .
git commit --amend --no-edit

# 5. Check for manual fixes needed
make lint

# 6. Fix remaining issues, then fold again
git add .
git commit --amend --no-edit

# 7. Validate before moving on
make test
```

### Iterative Development Pattern
```bash
# Working on complex feature across multiple commits
git commit -m "feat(scanner): add port discovery logic"
git commit -m "feat(scanner): implement service detection"
git commit -m "feat(scanner): add banner grabbing"

# Now clean up all quality issues at once
make lint-fix
git add .
git commit --fixup=HEAD~2 -m "apply lint fixes to scanner feature"

make lint  # Check manual fixes needed
# Fix remaining issues...
git add .
git commit --fixup=HEAD~3 -m "resolve type mismatches in scanner"

# Clean history before push
git rebase -i --autosquash HEAD~5
```

### Pre-Commit Hook Integration
```bash
# The make targets integrate with git hooks
make setup-hooks  # Sets up pre-commit validation

# Hooks will automatically run:
# - make lint-fix (auto-fix issues)
# - make lint (validate clean)
# - make test (functionality check)
```

### Dealing with Large Lint Sessions
```bash
# When inheriting code with many lint issues
git checkout -b chore/lint-cleanup

# Fix everything possible automatically
make lint-fix
git add .
git commit -m "chore: auto-fix linting issues across codebase"

# Check what requires manual attention
make lint

# Group manual fixes by category
git add internal/api/
git commit -m "fix(api): resolve type mismatches in handlers"

git add internal/db/
git commit -m "fix(db): implement missing repository methods"

# Squash if too granular
git rebase -i HEAD~3  # Combine related fixes
```

### CI Integration Pattern
```bash
# Development cycle with CI mindset
make lint-fix      # Auto-fix (usually 60-80% of issues)
make lint          # Check remaining issues
make test          # Functionality validation
make build         # Compilation check

# If all pass, commit confidently
git add .
git commit -m "feat(api): add comprehensive error handling"

# If issues remain, fix incrementally
# Use --fixup to avoid noise commits
```

### Time-Saving Git Aliases
Add to `~/.gitconfig`:
```ini
[alias]
    # Quick fixup for recent commits
    fixup = "!f() { git commit --fixup=$1; }; f"
    
    # Auto-squash rebase
    squash-all = rebase -i --autosquash
    
    # Conventional commit helpers
    feat = "!f() { git commit -m \"feat($1): $2\"; }; f"
    fix = "!f() { git commit -m \"fix($1): $2\"; }; f"
    chore = "!f() { git commit -m \"chore($1): $2\"; }; f"
```

### Usage with aliases:
```bash
git feat api "add pagination support"
make lint-fix && git fixup HEAD
git squash-all HEAD~3
```

## Key Files to Monitor
- `go.mod` / `go.sum` - Dependencies
- `.golangci.yml` - Linting configuration  
- `Makefile` - Build and quality targets
- `internal/api/server.go` - Main API server
- `internal/metrics/metrics.go` - Metrics system
- `internal/db/database.go` - Database layer

## Success Criteria
- `make ci` passes without errors
- `make lint` shows no critical issues
- All tests pass with database
- Binary builds successfully
- Zed shows no red underlines

## Linter-Specific Issue Prevention

Based on the linters configured in `.golangci.yml`, anticipate and prevent these issues:

### Linter Reference Guide

#### errcheck - Error Checking
**Purpose**: Ensure all errors are handled
**Common violations**:
```go
// ❌ Ignoring errors
rows.Close()
json.NewEncoder(w).Encode(data)
os.Remove(filename)

// ✅ Handle all errors
if err := rows.Close(); err != nil {
    log.Printf("Failed to close rows: %v", err)
}
if err := json.NewEncoder(w).Encode(data); err != nil {
    s.logger.Error("Failed to encode response", "error", err)
    return
}
if err := os.Remove(filename); err != nil {
    return fmt.Errorf("failed to remove file: %w", err)
}
```

#### gosec - Security Issues
**Purpose**: Detect security vulnerabilities
**Common violations**:
```go
// ❌ SQL injection risk
query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID)

// ❌ Weak random number generation
rand.Seed(time.Now().Unix())

// ✅ Secure patterns
query := "SELECT * FROM users WHERE id = $1"
// Use crypto/rand for security-sensitive randomness
```

#### govet - Static Analysis
**Purpose**: Find suspicious constructs
**Watch for**: Printf format strings, struct tag formats, unreachable code

#### staticcheck - Advanced Static Analysis  
**Purpose**: Find bugs, performance issues, style violations
**Auto-fixes available**: Many issues fixed by `make lint-fix`

#### bodyclose, rowserrcheck, sqlclosecheck - Resource Management
**Purpose**: Ensure proper resource cleanup
```go
// ❌ Resource leaks
resp, _ := http.Get(url)
rows, _ := db.Query("SELECT ...")

// ✅ Proper cleanup
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()

rows, err := db.Query("SELECT ...")
if err != nil {
    return err
}
defer func() {
    if err := rows.Close(); err != nil {
        log.Printf("Failed to close rows: %v", err)
    }
}()
```

#### prealloc - Slice Performance
**Purpose**: Optimize slice allocations
```go
// ❌ Inefficient growth
var items []string
for _, item := range data {
    items = append(items, process(item))
}

// ✅ Pre-allocate when size is known
items := make([]string, 0, len(data))
for _, item := range data {
    items = append(items, process(item))
}
```

#### ineffassign, unparam, unused - Dead Code
**Purpose**: Remove unused variables, parameters, code
**Auto-fixes**: Most issues fixed by `make lint-fix`

#### unconvert - Unnecessary Conversions
```go
// ❌ Unnecessary conversion
result := []string(stringSlice)

// ✅ Direct usage
result := stringSlice
```

#### gocyclo, funlen, nestif - Complexity Control
**Limits**: 
- Functions: Max 100 lines, 50 statements
- Cyclomatic complexity: Max 15
- Nested if: Max 5 levels
**Solution**: Extract smaller functions

#### lll - Line Length
**Limit**: 120 characters
**Auto-fix**: `make lint-fix` handles most cases

#### mnd - Magic Numbers
**Purpose**: Eliminate magic numbers
```go
// ❌ Magic numbers
timeout := time.Second * 30
buffer := make([]byte, 1024)

// ✅ Named constants
const (
    DefaultTimeout = 30 * time.Second
    DefaultBufferSize = 1024
)
```

#### misspell - Spelling
**Purpose**: Catch spelling errors in code and comments
**Auto-fix**: `make lint-fix` corrects common misspellings

#### godot - Documentation
**Purpose**: Ensure comments end with periods
```go
// ❌ Missing period
// ProcessRequest handles the request

// ✅ Proper format
// ProcessRequest handles the request.
```

#### gocritic - Style and Performance
**Purpose**: Style and performance analysis
**Categories**: diagnostic, performance, style, opinionated

#### goconst - Repeated Strings
**Purpose**: Extract repeated string literals to constants
```go
// ❌ Repeated strings
log.Printf("error")
log.Printf("error")

// ✅ Use constant
const LogError = "error"
```

#### dupl - Code Duplication
**Purpose**: Detect duplicated code blocks
**Solution**: Extract common functionality

#### copyloopvar - Loop Variable Copies
**Purpose**: Prevent loop variable capture issues
**Auto-fix**: `make lint-fix` handles these

#### predeclared - Predeclared Identifiers
**Purpose**: Avoid shadowing built-in identifiers
```go
// ❌ Shadowing built-ins
func new() {}  // shadows built-in new()

// ✅ Different name
func newInstance() {}
```

#### nolintlint - Linting Directives
**Purpose**: Ensure proper usage of //nolint comments
**Format**: `//nolint:linter-name // explanation`

Based on the configured linters in `.golangci.yml`, anticipate and prevent these common issues:

### Error Handling (errcheck, govet)
```go
// ❌ Will trigger errcheck
rows.Close()
json.NewEncoder(w).Encode(data)

// ✅ Proper error handling
if err := rows.Close(); err != nil {
    log.Printf("Failed to close rows: %v", err)
}
if err := json.NewEncoder(w).Encode(data); err != nil {
    s.logger.Error("Failed to encode response", "error", err)
}
```

### Resource Management (bodyclose, rowserrcheck, sqlclosecheck)
```go
// ❌ Will trigger bodyclose/rowserrcheck
resp, _ := http.Get(url)
rows, _ := db.Query("SELECT ...")

// ✅ Proper resource cleanup
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()

rows, err := db.Query("SELECT ...")
if err != nil {
    return err
}
defer func() {
    if err := rows.Close(); err != nil {
        log.Printf("Failed to close rows: %v", err)
    }
}()
```

### Security (gosec)
```go
// ❌ Will trigger gosec G104 (audit errors not checked)
_ = os.Setenv("KEY", "value")  // Use explicit error handling instead

// ❌ Will trigger gosec G201 (SQL injection risk)
query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID)

// ✅ Secure patterns
if err := os.Setenv("KEY", "value"); err != nil {
    return fmt.Errorf("failed to set env: %w", err)
}

// Use parameterized queries
query := "SELECT * FROM users WHERE id = $1"
```

### Performance (prealloc, ineffassign)
```go
// ❌ Will trigger prealloc
var items []string
for _, item := range data {
    items = append(items, process(item))
}

// ✅ Pre-allocate with known capacity
items := make([]string, 0, len(data))
for _, item := range data {
    items = append(items, process(item))
}
```

### Code Quality (unused, unparam, unconvert)
```go
// ❌ Will trigger unused/unparam
func processData(data []string, unused int) []string {
    result := []string(data)  // unnecessary conversion
    return result
}

// ✅ Clean implementation
func processData(data []string) []string {
    return data  // or proper processing logic
}
```

### Complexity Limits (gocyclo, funlen, nestif)
- **Functions**: Max 100 lines, 50 statements
- **Cyclomatic complexity**: Max 15
- **Nested if statements**: Max 5 levels
- **Solution**: Break large functions into smaller, focused functions

### Magic Numbers (mnd)
```go
// ❌ Will trigger mnd
timeout := time.Second * 30
port := 8080

// ✅ Use named constants
const (
    DefaultTimeout = 30 * time.Second
    DefaultAPIPort = 8080
)
```

### Documentation (godot)
```go
// ❌ Missing period
// ProcessRequest handles the incoming request

// ✅ Proper documentation
// ProcessRequest handles the incoming request.
```

### Style Guidelines (misspell, whitespace, lll)
- **Line length**: Max 120 characters
- **Spelling**: Use US English (check variable names, comments)
- **Whitespace**: No trailing spaces, consistent indentation

### When Writing New Code:
1. **Always check errors**: Never ignore return values
2. **Use defer for cleanup**: Close resources properly
3. **Parameterized queries**: Prevent SQL injection
4. **Pre-allocate slices**: When size is known
5. **Keep functions small**: Under 100 lines
6. **Document exported functions**: End comments with periods
7. **Use constants**: For repeated values and configuration

### Efficient Development Workflow:
```bash
# After writing new code
make lint-fix      # Auto-fix most issues immediately
make lint          # Check what requires manual attention
make test          # Ensure functionality works

# Quick iteration cycle
make lint-fix && make lint && make test
```

### Development Tips:
1. **Start with auto-fix**: `make lint-fix` resolves 60-80% of issues automatically
2. **Fix in batches**: Run `make lint-fix` after each significant code change
3. **Use CI target**: `make ci` runs the full pipeline
4. **IDE integration**: Zed + auto-fix provides real-time feedback
5. **Commit early, fixup often**: Use `--fixup` to avoid yak shaving history
6. **Batch similar fixes**: Group lint fixes, type fixes, etc. before squashing
7. **Precise commit messages**: Avoid adjectives, use technical descriptions
8. **Imperative mood**: "fix timeout" not "fixed timeout" or "fixes timeout"

### Anti-Patterns (Avoid Yak Shaving):
```bash
# ❌ Creates noisy, imprecise history
git commit -m "add feature"
git commit -m "fix lint"
git commit -m "fix more lint"
git commit -m "make things better"
git commit -m "actually fix lint"
git commit -m "final perfect fix"

# ✅ Clean, precise history
git commit -m "feat(api): implement offset-based pagination for scan results"
# (use fixup commits during development)
git rebase -i --autosquash  # Before push
# Results in single focused commit
```

## Quick Reference: Scanorama-Specific Patterns

### API Handler Patterns
```go
// ✅ Standard handler signature
func (h *ScanHandler) ListScans(w http.ResponseWriter, r *http.Request) {
    // Always include request ID and timing
    start := time.Now()
    requestID := middleware.GetRequestID(r)
    
    h.logger.Info("API request started", 
        "request_id", requestID,
        "endpoint", "ListScans")
    
    // Record metrics
    h.metrics.Counter("api_requests_total", map[string]string{
        "endpoint": "list_scans",
        "method": r.Method,
    })
    
    defer func() {
        h.metrics.Histogram("api_request_duration_seconds", 
            time.Since(start).Seconds(), 
            map[string]string{"endpoint": "list_scans"})
    }()
}
```

### Database Repository Patterns
```go
// ✅ Repository method signature
func (r *ScanJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*ScanJob, error) {
    var job ScanJob
    query := `SELECT * FROM scan_jobs WHERE id = $1`
    
    if err := r.db.GetContext(ctx, &job, query, id); err != nil {
        if err == sql.ErrNoRows {
            return nil, fmt.Errorf("scan job not found: %w", err)
        }
        return nil, fmt.Errorf("failed to get scan job: %w", err)
    }
    
    return &job, nil
}
```

### Error Response Patterns
```go
// ✅ Standard error response
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, statusCode int, err error) {
    s.logger.Error("API error",
        "method", r.Method,
        "path", r.URL.Path,
        "status", statusCode,
        "error", err,
        "remote_addr", r.RemoteAddr)

    response := ErrorResponse{
        Error:     err.Error(),
        Timestamp: time.Now().UTC(),
        RequestID: getRequestID(r),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    
    if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
        s.logger.Error("Failed to encode error response", "error", encodeErr)
    }
}
```

### Resource Cleanup Patterns
```go
// ✅ Database row cleanup
rows, err := r.db.NamedQueryContext(ctx, query, target)
if err != nil {
    return fmt.Errorf("failed to create target: %w", err)
}
defer func() {
    if err := rows.Close(); err != nil {
        log.Printf("Failed to close rows: %v", err)
    }
}()
```

### Metric Recording Patterns
```go
// ✅ Counter metrics
h.metrics.Counter("scan_operations_total", map[string]string{
    "type": "port_scan",
    "status": "completed",
})

// ✅ Histogram metrics  
h.metrics.Histogram("scan_duration_seconds", duration.Seconds(), map[string]string{
    "type": "port_scan",
})

// ✅ Gauge metrics
h.metrics.Gauge("active_connections", float64(activeConns), map[string]string{
    "database": "postgres",
})
```

### SQL Query Formatting Patterns

#### Multi-line SELECT Statements
```go
// ✅ Well-formatted multi-line SELECT
query := `
    SELECT
        id, name, cidr, description, discovery_method,
        is_active, scan_enabled, last_discovery, last_scan,
        host_count, active_host_count, created_at, updated_at, created_by
    FROM networks
    WHERE is_active = true
    ORDER BY name`

// ❌ Poorly formatted query
query := `SELECT id, name, cidr, description, discovery_method, is_active, scan_enabled, last_discovery, last_scan, host_count, active_host_count, created_at, updated_at, created_by FROM networks WHERE is_active = true ORDER BY name`
```

#### Multi-line INSERT Statements
```go
// ✅ Well-formatted multi-line INSERT
query := `
    INSERT INTO network_exclusions (
        network_id,
        excluded_cidr,
        reason,
        enabled
    ) VALUES (
        $1, $2, $3, true
    )
    RETURNING
        id, network_id, excluded_cidr::text, reason, enabled,
        created_at, updated_at, created_by`

// ❌ Hard to read single-line INSERT
query := `INSERT INTO network_exclusions (network_id, excluded_cidr, reason, enabled) VALUES ($1, $2, $3, true) RETURNING id, network_id, excluded_cidr::text, reason, enabled, created_at, updated_at, created_by`
```

#### Complex UPDATE Statements
```go
// ✅ Well-formatted UPDATE
query := `
    UPDATE networks
    SET
        name = $2,
        cidr = $3,
        description = $4,
        discovery_method = $5,
        is_active = $6,
        updated_at = NOW()
    WHERE id = $1
    RETURNING
        id, name, cidr, description, discovery_method,
        is_active, scan_enabled, last_discovery, last_scan,
        host_count, active_host_count, created_at, updated_at, created_by`
```

#### SQL Function Calls
```go
// ✅ Clear function call formatting
query := `
    SELECT ip_address::text
    FROM generate_host_ips_with_exclusions($1::CIDR, $2, $3)
    ORDER BY ip_address`

// ✅ Complex query with joins
query := `
    SELECT
        n.id, n.name, n.cidr,
        COUNT(h.id) as host_count,
        COUNT(h.id) FILTER (WHERE h.status = 'up') as active_hosts
    FROM networks n
    LEFT JOIN hosts h ON h.ip_address << n.cidr
    WHERE n.is_active = true
    GROUP BY n.id, n.name, n.cidr
    ORDER BY n.name`
```

#### SQL Formatting Rules
1. **Indent nested clauses**: Use 4 spaces for indentation
2. **Align keywords**: Place SQL keywords (SELECT, FROM, WHERE, etc.) at consistent positions
3. **Break long column lists**: Split across multiple lines with proper alignment
4. **Group related columns**: Keep logically related columns together
5. **Use consistent casing**: Uppercase for SQL keywords, lowercase for identifiers
6. **Add line breaks before major clauses**: WHERE, ORDER BY, GROUP BY, etc.