# Settings Typed Accessors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four typed accessor methods to `SettingsRepository` that handle JSONB unquoting transparently, then migrate `PortListResolver` to use them instead of raw SQL with manual JSON parsing.

**Architecture:** New methods call the existing `GetSetting` and type-convert `Setting.Value` (raw JSONB text). All four follow the same pattern: unmarshal the JSON value, return `(T, error)`. `db.ErrNotFound` propagates unchanged so callers decide the fallback. No silent defaults at the repository layer. `PortListResolver` gets an internal `settingsRepo *SettingsRepository` field; its fail-open fallback logic stays in the service layer.

**Tech Stack:** Go, `encoding/json`, `go-sqlmock` (unit tests), existing `SettingsRepository` / `GetSetting` pattern.

---

## Context

### Settings table

```sql
CREATE TABLE settings (
    key         VARCHAR(100) PRIMARY KEY,
    value       JSONB        NOT NULL,
    description TEXT,
    type        VARCHAR(20)  NOT NULL DEFAULT 'string',
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

`GetSetting` queries `value::text`, so `Setting.Value` is the raw JSONB representation:
- String `"hello"` → `"hello"` (JSON-quoted)
- Integer `256` → `256` (bare)
- Boolean `true` → `true` (bare)
- Array `["a","b"]` → `["a","b"]`

### Existing `GetSetting` query

```go
SELECT key, value::text, COALESCE(description, ''), type, updated_at
FROM settings WHERE key = $1
```

Unit tests mock this query with `mock.ExpectQuery(`SELECT key, value`).WithArgs(key)`.

### Current footgun in `PortListResolver`

`readBasePorts` and `readLimit` run their own raw queries and do manual JSON unquoting:

```go
// readBasePorts: manual json.Unmarshal + fallback to raw string
// readLimit: strings.Trim(raw, `"`) + strconv.Atoi
```

After this refactor both methods use typed accessors and contain no JSON handling.

---

## Files

| File | Change |
|------|--------|
| `internal/db/repository_settings.go` | Add 4 typed accessor methods |
| `internal/db/repository_settings_unit_test.go` | Add tests for 4 accessors |
| `internal/services/portresolver.go` | Add `settingsRepo` field; simplify `readBasePorts`/`readLimit` |
| `internal/services/portresolver_test.go` | Update mock expectations to match `GetSetting` query |

---

## Task 1: Typed accessors on `SettingsRepository`

**Files:**
- Modify: `internal/db/repository_settings.go`

### Method signatures

```go
// GetStringSetting returns the unquoted string value for a string-typed setting.
// Returns ErrNotFound if the key does not exist.
func (r *SettingsRepository) GetStringSetting(ctx context.Context, key string) (string, error)

// GetIntSetting returns the integer value for an int-typed setting.
// Returns ErrNotFound if the key does not exist.
func (r *SettingsRepository) GetIntSetting(ctx context.Context, key string) (int, error)

// GetBoolSetting returns the boolean value for a bool-typed setting.
// Returns ErrNotFound if the key does not exist.
func (r *SettingsRepository) GetBoolSetting(ctx context.Context, key string) (bool, error)

// GetStringSliceSetting returns the []string value for a string[]-typed setting.
// Returns ErrNotFound if the key does not exist.
func (r *SettingsRepository) GetStringSliceSetting(ctx context.Context, key string) ([]string, error)
```

### Implementation pattern (same for all four)

```go
func (r *SettingsRepository) GetStringSetting(ctx context.Context, key string) (string, error) {
    s, err := r.GetSetting(ctx, key)
    if err != nil {
        return "", err
    }
    var val string
    if err := json.Unmarshal([]byte(s.Value), &val); err != nil {
        return "", fmt.Errorf("setting %q value is not a JSON string: %w", key, err)
    }
    return val, nil
}

func (r *SettingsRepository) GetIntSetting(ctx context.Context, key string) (int, error) {
    s, err := r.GetSetting(ctx, key)
    if err != nil {
        return 0, err
    }
    var val int
    if err := json.Unmarshal([]byte(s.Value), &val); err != nil {
        return 0, fmt.Errorf("setting %q value is not a JSON integer: %w", key, err)
    }
    return val, nil
}

func (r *SettingsRepository) GetBoolSetting(ctx context.Context, key string) (bool, error) {
    s, err := r.GetSetting(ctx, key)
    if err != nil {
        return false, err
    }
    var val bool
    if err := json.Unmarshal([]byte(s.Value), &val); err != nil {
        return false, fmt.Errorf("setting %q value is not a JSON boolean: %w", key, err)
    }
    return val, nil
}

func (r *SettingsRepository) GetStringSliceSetting(ctx context.Context, key string) ([]string, error) {
    s, err := r.GetSetting(ctx, key)
    if err != nil {
        return nil, err
    }
    var val []string
    if err := json.Unmarshal([]byte(s.Value), &val); err != nil {
        return nil, fmt.Errorf("setting %q value is not a JSON string array: %w", key, err)
    }
    return val, nil
}
```

`encoding/json` must be added to imports.

- [ ] **Step 1: Add the four methods to `repository_settings.go`**

Add after `GetSetting`, before `SetSetting`. Add `"encoding/json"` to imports.

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/db/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/db/repository_settings.go
git commit -m "feat(db): add typed accessors to SettingsRepository"
```

---

## Task 2: Unit tests for typed accessors

**Files:**
- Modify: `internal/db/repository_settings_unit_test.go`

### Test cases per accessor

Each accessor needs four tests (same structure for all four):
1. Happy path — key exists with correct JSON type
2. Not found — key missing → `ErrNotFound`
3. Wrong type — value is valid JSON but wrong type (e.g. `"123"` for GetIntSetting) → type error
4. DB error — query fails → error propagated

### Mock pattern (matches existing `makeSettingRow` helper and `GetSetting` query)

The query being mocked is `GetSetting`'s query:
```
SELECT key, value::text, COALESCE(description, ''), type, updated_at
FROM settings WHERE key = $1
```

Mock with:
```go
mock.ExpectQuery(`SELECT key, value`).
    WithArgs("my.key").
    WillReturnRows(makeSettingRow("my.key", `"hello"`, "", "string"))
```

### Full test examples

```go
func TestSettingsRepository_GetStringSetting_ReturnsUnquotedValue(t *testing.T) {
    db, mock := newMockDB(t)
    repo := NewSettingsRepository(db)

    mock.ExpectQuery(`SELECT key, value`).
        WithArgs("scan.label").
        WillReturnRows(makeSettingRow("scan.label", `"fast"`, "", "string"))

    val, err := repo.GetStringSetting(context.Background(), "scan.label")
    require.NoError(t, err)
    assert.Equal(t, "fast", val)
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_ReturnsErrNotFound(t *testing.T) {
    db, mock := newMockDB(t)
    repo := NewSettingsRepository(db)

    mock.ExpectQuery(`SELECT key, value`).
        WithArgs("missing.key").
        WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}))

    _, err := repo.GetStringSetting(context.Background(), "missing.key")
    require.Error(t, err)
    assert.True(t, errors.IsNotFound(err))
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_ReturnsErrorOnWrongType(t *testing.T) {
    db, mock := newMockDB(t)
    repo := NewSettingsRepository(db)

    mock.ExpectQuery(`SELECT key, value`).
        WithArgs("scan.label").
        WillReturnRows(makeSettingRow("scan.label", `123`, "", "int")) // int, not string

    _, err := repo.GetStringSetting(context.Background(), "scan.label")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "not a JSON string")
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_PropagatesDBError(t *testing.T) {
    db, mock := newMockDB(t)
    repo := NewSettingsRepository(db)

    mock.ExpectQuery(`SELECT key, value`).
        WithArgs("scan.label").
        WillReturnError(fmt.Errorf("conn error"))

    _, err := repo.GetStringSetting(context.Background(), "scan.label")
    require.Error(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}
```

Repeat the same four test cases for `GetIntSetting` (value `"5"` happy path, `"true"` wrong type), `GetBoolSetting` (value `"true"` happy path, `"123"` wrong type), and `GetStringSliceSetting` (value `'["icmp","arp"]'` happy path, `"single"` wrong type).

- [ ] **Step 1: Write the failing tests** (16 tests total: 4 per accessor × 4 accessors)

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/db/ -run "TestSettingsRepository_Get.*Setting" -v
```

Expected: compile error or FAIL (methods not yet called — but since Task 1 already added the methods, tests should fail only if assertions are wrong).

- [ ] **Step 3: Run tests to confirm they pass**

```bash
go test -race -count=1 ./internal/db/ -run "TestSettingsRepository_Get.*Setting" -v
```

Expected: 16 PASS.

- [ ] **Step 4: Run full db suite to confirm no regressions**

```bash
go test -race -count=1 ./internal/db/ -short
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/repository_settings_unit_test.go
git commit -m "test(db): add unit tests for typed SettingsRepository accessors"
```

---

## Task 3: Migrate `PortListResolver` to use typed accessors

**Files:**
- Modify: `internal/services/portresolver.go`

### Changes

1. Add `settingsRepo *db.SettingsRepository` field to `PortListResolver` struct.
2. In `NewPortListResolver`, create the repo from the existing `database *db.DB`:
   ```go
   return &PortListResolver{
       db:           database,
       settingsRepo: db.NewSettingsRepository(database),
       logger:       logger,
   }
   ```
3. Rewrite `readBasePorts` to use `r.settingsRepo.GetStringSetting`:

```go
func (r *PortListResolver) readBasePorts(ctx context.Context, stage string) string {
    fallback, ok := stageDefaultPorts[stage]
    if !ok {
        r.logger.Warn("unknown stage passed to port resolver, using broad fallback", "stage", stage)
        fallback = "1-1024"
    }

    key := "smartscan." + stage + ".ports"
    val, err := r.settingsRepo.GetStringSetting(ctx, key)
    if err != nil {
        if !errors.IsNotFound(err) {
            r.logger.Warn("failed to read base port setting, using default", "key", key, "error", err)
        }
        return fallback
    }
    if val == "" {
        return fallback
    }
    return val
}
```

4. Rewrite `readLimit` to use `r.settingsRepo.GetIntSetting`:

```go
func (r *PortListResolver) readLimit(ctx context.Context) int {
    const key = "smartscan.top_ports_limit"
    n, err := r.settingsRepo.GetIntSetting(ctx, key)
    if err != nil {
        if !errors.IsNotFound(err) {
            r.logger.Warn("failed to read top_ports_limit setting, using default", "error", err)
        }
        return defaultTopPortsLimit
    }
    if n <= 0 {
        r.logger.Warn("smartscan.top_ports_limit must be positive, using default", "value", n)
        return defaultTopPortsLimit
    }
    return n
}
```

5. Remove unused imports: `"database/sql"`, `"encoding/json"`, `"strings"` (used only by the old parsing code). Verify with `go build`.

Note: `errors.IsNotFound` is from `github.com/anstrom/scanorama/internal/errors` — the same package used by `SettingsRepository`. Check the import alias pattern in portresolver.go and use the same.

- [ ] **Step 1: Add `settingsRepo` field and update `NewPortListResolver`**

- [ ] **Step 2: Rewrite `readBasePorts`**

- [ ] **Step 3: Rewrite `readLimit`**

- [ ] **Step 4: Remove stale imports and verify build**

```bash
go build ./internal/services/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/services/portresolver.go
git commit -m "refactor(services): migrate PortListResolver to use typed SettingsRepository accessors"
```

---

## Task 4: Update `portresolver_test.go` mock expectations

**Files:**
- Modify: `internal/services/portresolver_test.go`

The existing tests mock `SELECT value::text FROM settings WHERE key = $1` directly. After migration, `GetStringSetting` and `GetIntSetting` each call `GetSetting`, whose query is:

```
SELECT key, value::text, COALESCE(description, ''), type, updated_at
FROM settings WHERE key = $1
```

All tests that previously had:
```go
mock.ExpectQuery(`SELECT value`).
    WithArgs("smartscan.os_detection.ports").
    WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22,443,8080"`))
```

Must become:
```go
mock.ExpectQuery(`SELECT key, value`).
    WithArgs("smartscan.os_detection.ports").
    WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
        AddRow("smartscan.os_detection.ports", `"22,443,8080"`, "", "string", time.Now()))
```

Similarly, limit queries returning `"256"` (bare integer) become full five-column rows with `value = "256"`.

All 19 existing tests in `portresolver_test.go` that set up settings mock expectations need this update. The error-return tests that use `WillReturnError` are unaffected structurally (just update the `ExpectQuery` pattern from `SELECT value` to `SELECT key, value`).

A helper `settingRow(key, value string) *sqlmock.Rows` can reduce repetition:

```go
func settingRow(key, value string) *sqlmock.Rows {
    return sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
        AddRow(key, value, "", "string", time.Now())
}
```

- [ ] **Step 1: Add `settingRow` helper to `portresolver_test.go`**

- [ ] **Step 2: Update all settings mock expectations** (both base-ports and limit queries in every test)

- [ ] **Step 3: Run tests to confirm they pass**

```bash
go test -race -count=1 ./internal/services/ -run "PortList|PortResolver" -v
```

Expected: all 19 PASS.

- [ ] **Step 4: Run full services suite**

```bash
go test -race -count=1 ./internal/services/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/portresolver_test.go
git commit -m "test(services): update portresolver mocks to match SettingsRepository query shape"
```

---

## Verification

```bash
go test -race ./internal/db/ ./internal/services/
```

All tests pass. No references to `strings.Trim` or manual `json.Unmarshal` remain in `portresolver.go`.
