# Adaptive Port Probing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When nmap finds an open port with no identified service, probe it with HTTP → HTTPS → SSH → plain TCP in sequence, stopping at first identification, and permanently mark the (host, port) pair so this extended probe never runs again.

**Architecture:** A new `extended_probe_done` boolean column on `port_banners` tracks whether the extended sequence has run. `grabOne` checks this flag in its default branch. A new `probeUnknown` method runs the sequence. A per-host cap of 20 extended probes per scan cycle is enforced in `EnrichHosts` via a counter in a new `AllowExtendedProbe` field on `PortInfo`.

**Tech Stack:** Go, `go-sqlmock` for DB unit tests, `httptest` + in-process SSH server for enrichment tests (existing pattern in `banner_zgrab2_test.go`).

---

## File Map

| File | Change |
|------|--------|
| `internal/db/024_port_banners_extended_probe.sql` | **Create** — migration adding `extended_probe_done` column |
| `internal/db/models.go` | **Modify** — add `ExtendedProbeDone bool` to `PortBanner` |
| `internal/db/repository_banners.go` | **Modify** — add `IsExtendedProbeDone` and `MarkExtendedProbeDone` methods |
| `internal/db/repository_banners_unit_test.go` | **Modify** — tests for the two new methods |
| `internal/enrichment/banner.go` | **Modify** — add `AllowExtendedProbe` to `PortInfo`, rate-cap logic in `EnrichHosts`, `probeUnknown` method, updated `grabOne` default branch |
| `internal/enrichment/banner_zgrab2_test.go` | **Modify** — tests for `probeUnknown` against in-process servers |

---

## Task 1: DB migration

**Files:**
- Create: `internal/db/024_port_banners_extended_probe.sql`

- [ ] **Step 1.1: Create the migration file**

```sql
-- Migration 024: track whether extended protocol probing has been attempted
-- for a port. Set once per (host_id, port) combination; never reset.

ALTER TABLE port_banners
    ADD COLUMN IF NOT EXISTS extended_probe_done BOOLEAN NOT NULL DEFAULT FALSE;
```

- [ ] **Step 1.2: Verify it applies cleanly against a running dev DB**

```bash
make migrate   # or however migrations are applied in this project
```

Look for any error output. If the migration tool is not `make migrate`, check `Makefile` for the correct target.

- [ ] **Step 1.3: Commit**

```bash
git add internal/db/024_port_banners_extended_probe.sql
git commit -m "feat(db): add extended_probe_done column to port_banners"
```

---

## Task 2: Model field

**Files:**
- Modify: `internal/db/models.go` around line 734

- [ ] **Step 2.1: Add the field to `PortBanner`**

In `internal/db/models.go`, add `ExtendedProbeDone` as the last field of `PortBanner`:

```go
// PortBanner is a raw service banner captured from a host/port.
type PortBanner struct {
	ID                  uuid.UUID `db:"id"                     json:"id"`
	HostID              uuid.UUID `db:"host_id"                json:"host_id"`
	Port                int       `db:"port"                   json:"port"`
	Protocol            string    `db:"protocol"               json:"protocol"`
	RawBanner           *string   `db:"raw_banner"             json:"raw_banner,omitempty"`
	Service             *string   `db:"service"                json:"service,omitempty"`
	Version             *string   `db:"version"                json:"version,omitempty"`
	HTTPTitle           *string   `db:"http_title"             json:"http_title,omitempty"`
	SSHKeyFingerprint   *string   `db:"ssh_key_fingerprint"    json:"ssh_key_fingerprint,omitempty"`
	HTTPStatusCode      *int16    `db:"http_status_code"       json:"http_status_code,omitempty"`
	HTTPRedirect        *string   `db:"http_redirect"          json:"http_redirect,omitempty"`
	HTTPResponseHeaders JSONB     `db:"http_response_headers"  json:"http_response_headers,omitempty"`
	ScannedAt           time.Time `db:"scanned_at"             json:"scanned_at"`
	ExtendedProbeDone   bool      `db:"extended_probe_done"    json:"-"`
}
```

- [ ] **Step 2.2: Confirm compilation**

```bash
go build ./internal/db/...
```

Expected: no output (clean build).

- [ ] **Step 2.3: Commit**

```bash
git add internal/db/models.go
git commit -m "feat(db): add ExtendedProbeDone field to PortBanner model"
```

---

## Task 3: Repository methods

**Files:**
- Modify: `internal/db/repository_banners.go`
- Modify: `internal/db/repository_banners_unit_test.go`

### 3a — Write the failing tests first

- [ ] **Step 3.1: Write tests for `IsExtendedProbeDone` and `MarkExtendedProbeDone`**

Append to `internal/db/repository_banners_unit_test.go`:

```go
// ── IsExtendedProbeDone ────────────────────────────────────────────────────

func TestBannerRepository_IsExtendedProbeDone_NoRow(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WithArgs(hostID, 9999, ProtocolTCP).
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"}))

	done, err := repo.IsExtendedProbeDone(context.Background(), hostID, 9999)
	require.NoError(t, err)
	assert.False(t, done)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_IsExtendedProbeDone_FalseRow(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WithArgs(hostID, 80, ProtocolTCP).
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"}).AddRow(false))

	done, err := repo.IsExtendedProbeDone(context.Background(), hostID, 80)
	require.NoError(t, err)
	assert.False(t, done)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_IsExtendedProbeDone_TrueRow(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WithArgs(hostID, 8080, ProtocolTCP).
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"}).AddRow(true))

	done, err := repo.IsExtendedProbeDone(context.Background(), hostID, 8080)
	require.NoError(t, err)
	assert.True(t, done)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_IsExtendedProbeDone_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WillReturnError(fmt.Errorf("connection lost"))

	done, err := repo.IsExtendedProbeDone(context.Background(), hostID, 22)
	require.Error(t, err)
	assert.False(t, done)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── MarkExtendedProbeDone ─────────────────────────────────────────────────

func TestBannerRepository_MarkExtendedProbeDone_OK(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(sqlmock.AnyArg(), hostID, 9999, ProtocolTCP, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.MarkExtendedProbeDone(context.Background(), hostID, 9999)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_MarkExtendedProbeDone_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnError(fmt.Errorf("disk full"))

	err := repo.MarkExtendedProbeDone(context.Background(), hostID, 9999)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 3.2: Run the tests and confirm they fail**

```bash
go test ./internal/db/... -run "TestBannerRepository_IsExtendedProbeDone|TestBannerRepository_MarkExtendedProbeDone" -v
```

Expected: FAIL — `repo.IsExtendedProbeDone undefined`, `repo.MarkExtendedProbeDone undefined`.

### 3b — Implement

- [ ] **Step 3.3: Add `IsExtendedProbeDone` and `MarkExtendedProbeDone` to `repository_banners.go`**

Append to `internal/db/repository_banners.go` before the closing of the file:

```go
// IsExtendedProbeDone reports whether extended protocol probing has already
// been attempted for the given host/port pair over TCP.
// Returns false when no banner row exists yet — caller should proceed with probing.
func (r *BannerRepository) IsExtendedProbeDone(ctx context.Context, hostID uuid.UUID, port int) (bool, error) {
	var done bool
	err := r.db.QueryRowContext(ctx,
		`SELECT extended_probe_done FROM port_banners
		 WHERE host_id = $1 AND port = $2 AND protocol = $3`,
		hostID, port, ProtocolTCP,
	).Scan(&done)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, sanitizeDBError("is extended probe done", err)
	}
	return done, nil
}

// MarkExtendedProbeDone records that extended protocol probing has been
// attempted for the given host/port pair. Inserts a minimal row if none
// exists; otherwise sets extended_probe_done = true on the existing row.
func (r *BannerRepository) MarkExtendedProbeDone(ctx context.Context, hostID uuid.UUID, port int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_banners (id, host_id, port, protocol, extended_probe_done, scanned_at)
		VALUES ($1, $2, $3, $4, true, $5)
		ON CONFLICT (host_id, port, protocol) DO UPDATE SET
			extended_probe_done = true`,
		uuid.New(), hostID, port, ProtocolTCP, time.Now().UTC())
	if err != nil {
		return sanitizeDBError("mark extended probe done", err)
	}
	return nil
}
```

Note: `stderrors` and `sql` are already imported in this file.

- [ ] **Step 3.4: Run the tests and confirm they pass**

```bash
go test ./internal/db/... -run "TestBannerRepository_IsExtendedProbeDone|TestBannerRepository_MarkExtendedProbeDone" -v
```

Expected: PASS for all 6 tests.

- [ ] **Step 3.5: Run the full db package tests to confirm no regressions**

```bash
go test ./internal/db/... -count=1
```

Expected: all tests pass.

- [ ] **Step 3.6: Commit**

```bash
git add internal/db/repository_banners.go internal/db/repository_banners_unit_test.go
git commit -m "feat(db): add IsExtendedProbeDone and MarkExtendedProbeDone to BannerRepository"
```

---

## Task 4: `PortInfo` field and rate cap in `EnrichHosts`

**Files:**
- Modify: `internal/enrichment/banner.go`

The `PortInfo` struct needs an `AllowExtendedProbe` field so `grabOne` knows whether the per-host cap has been reached. `EnrichHosts` sets this field before dispatching goroutines.

- [ ] **Step 4.1: Add `AllowExtendedProbe` to `PortInfo` and the cap constant**

In `internal/enrichment/banner.go`, update the constants block and the `PortInfo` struct:

```go
const (
	bannerReadBytes          = 1024
	bannerDialTimeout        = 5 * time.Second
	bannerReadTimeout        = 3 * time.Second
	bannerConcurrency        = 10
	maxExtendedProbesPerHost = 20

	zgrabUserAgent    = "Mozilla/5.0 zgrab/0.x"
	zgrabMaxSizeKB    = 256
	zgrabMaxRedirects = 3

	serviceSSH = "ssh"
)

// PortInfo carries a port number and its nmap-detected service name.
type PortInfo struct {
	Number             int
	Service            string // nmap-detected service name, e.g. "http", "ssh", ""
	AllowExtendedProbe bool   // set by EnrichHosts; true when within per-host extended-probe cap
}
```

- [ ] **Step 4.2: Add the rate-cap counter to `EnrichHosts`**

Replace the existing `EnrichHosts` function body with:

```go
// EnrichHosts grabs banners for all targets concurrently.
// Errors are logged rather than returned — enrichment is best-effort.
// For each host, at most maxExtendedProbesPerHost ports with an unidentified
// service are eligible for extended protocol probing.
func (g *BannerGrabber) EnrichHosts(ctx context.Context, targets []BannerTarget) {
	if len(targets) == 0 {
		return
	}

	// Mark which unknown-service ports are within the per-host extended-probe cap.
	unknownCount := make(map[uuid.UUID]int, len(targets))
	for i := range targets {
		for j := range targets[i].Ports {
			if targets[i].Ports[j].Service == "" {
				unknownCount[targets[i].HostID]++
				if unknownCount[targets[i].HostID] <= maxExtendedProbesPerHost {
					targets[i].Ports[j].AllowExtendedProbe = true
				}
			}
		}
	}

	sem := make(chan struct{}, bannerConcurrency)
	var wg sync.WaitGroup

	for _, t := range targets {
		for _, pi := range t.Ports {
			wg.Add(1)
			sem <- struct{}{}
			go func(target BannerTarget, p PortInfo) {
				defer wg.Done()
				defer func() { <-sem }()
				g.grabOne(ctx, target, p)
			}(t, pi)
		}
	}

	wg.Wait()
}
```

- [ ] **Step 4.3: Confirm compilation**

```bash
go build ./internal/enrichment/...
```

Expected: no output.

- [ ] **Step 4.4: Commit**

```bash
git add internal/enrichment/banner.go
git commit -m "feat(enrichment): add AllowExtendedProbe field and per-host rate cap to EnrichHosts"
```

---

## Task 5: `probeUnknown` and updated `grabOne`

**Files:**
- Modify: `internal/enrichment/banner.go`

- [ ] **Step 5.1: Write the failing tests first**

Append to `internal/enrichment/banner_zgrab2_test.go`:

```go
// ── probeUnknown ─────────────────────────────────────────────────────────────

// TestProbeUnknown_HTTP_StopsAfterSuccess verifies that when an HTTP server
// responds on the port, probeUnknown does not attempt HTTPS or SSH and sets
// the extended_probe_done flag.
func TestProbeUnknown_HTTP_StopsAfterSuccess(t *testing.T) {
	addr := startHTTPServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// HTTP grab stores banner; mark extended probe done.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host}
	g.probeUnknown(context.Background(), target, port)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestProbeUnknown_NoServer_SetsFlag verifies that probeUnknown sets
// extended_probe_done even when all probes fail (nothing is listening).
func TestProbeUnknown_NoServer_SetsFlag(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// All probes will fail (unreachable port). grabPlain will find nothing to store,
	// but MarkExtendedProbeDone still runs.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.probeUnknown(ctx, target, 19990)

	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 5.2: Run the tests and confirm they fail**

```bash
go test -tags '!race' ./internal/enrichment/... -run "TestProbeUnknown" -v
```

Expected: FAIL — `g.probeUnknown undefined`.

- [ ] **Step 5.3: Implement `probeUnknown`**

Add this method to `banner.go` after `grabOne`:

```go
// probeUnknown tries HTTP → HTTPS → SSH → plain TCP in sequence for a port
// with no nmap-identified service. Stops after the first probe that succeeds
// (returns no error). Always calls MarkExtendedProbeDone when finished so the
// sequence never repeats for this (host, port) pair.
func (g *BannerGrabber) probeUnknown(ctx context.Context, t BannerTarget, port int) {
	addr := fmt.Sprintf("%s:%d", t.IP, port)

	if err := g.grabZGrabHTTP(ctx, t, port); err == nil {
		g.markProbeDone(ctx, t.HostID, port)
		return
	}
	if err := g.grabZGrabHTTPS(ctx, t, port); err == nil {
		g.markProbeDone(ctx, t.HostID, port)
		return
	}
	if err := g.grabZGrabSSH(ctx, t, port, addr); err == nil {
		g.markProbeDone(ctx, t.HostID, port)
		return
	}
	g.grabPlain(ctx, t, port, addr)
	g.markProbeDone(ctx, t.HostID, port)
}

// markProbeDone calls MarkExtendedProbeDone and logs a warning on failure.
// Probe results are already stored; this is best-effort bookkeeping.
func (g *BannerGrabber) markProbeDone(ctx context.Context, hostID uuid.UUID, port int) {
	if err := g.repo.MarkExtendedProbeDone(ctx, hostID, port); err != nil {
		g.logger.Warn("failed to mark extended probe done",
			"host_id", hostID, "port", port, "error", err)
	}
}
```

- [ ] **Step 5.4: Update the `grabOne` default branch**

Replace the `default:` case in `grabOne`:

```go
default:
	if pi.Service == "" && pi.AllowExtendedProbe {
		done, _ := g.repo.IsExtendedProbeDone(ctx, t.HostID, pi.Number)
		if !done {
			g.probeUnknown(ctx, t, pi.Number)
			return
		}
	}
	g.grabPlain(ctx, t, pi.Number, addr)
```

`BannerRepository.IsExtendedProbeDone` and `MarkExtendedProbeDone` are methods on
`*db.BannerRepository`. The `repo` field on `BannerGrabber` is already typed as
`*db.BannerRepository`, so no interface changes are needed.

- [ ] **Step 5.5: Run `probeUnknown` tests**

```bash
go test -tags '!race' ./internal/enrichment/... -run "TestProbeUnknown" -v
```

Expected: PASS for both tests.

- [ ] **Step 5.6: Run the full enrichment test suite**

```bash
go test -tags '!race' ./internal/enrichment/... -count=1
```

Expected: all tests pass.

- [ ] **Step 5.7: Commit**

```bash
git add internal/enrichment/banner.go internal/enrichment/banner_zgrab2_test.go
git commit -m "feat(enrichment): add probeUnknown — adaptive protocol detection for unidentified ports"
```

---

## Task 6: `grabOne` routing tests

**Files:**
- Modify: `internal/enrichment/banner_zgrab2_test.go`

These tests verify that `grabOne` correctly routes through `probeUnknown` when the service is unknown and `AllowExtendedProbe` is true, and skips it when the flag is false or the probe has already been done.

- [ ] **Step 6.1: Write the failing tests**

Append to `internal/enrichment/banner_zgrab2_test.go`:

```go
// ── grabOne routing ───────────────────────────────────────────────────────────

// TestGrabOne_UnknownService_AllowedAndNotDone verifies that grabOne runs
// probeUnknown (extended sequence) when service is empty, AllowExtendedProbe
// is true, and the DB reports extended_probe_done = false.
func TestGrabOne_UnknownService_AllowedAndNotDone(t *testing.T) {
	addr := startHTTPServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// IsExtendedProbeDone returns false → probeUnknown runs.
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"})) // no row = false
	// HTTP grab succeeds → one banner INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// MarkExtendedProbeDone → one more INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "", AllowExtendedProbe: true}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_UnknownService_AlreadyDone verifies that grabOne falls back to
// plain TCP when extended_probe_done is already true, without calling probeUnknown.
func TestGrabOne_UnknownService_AlreadyDone(t *testing.T) {
	banner := "CUSTOM PROTO 1.0\r\n"
	addr := startTCPServer(t, banner)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// IsExtendedProbeDone returns true → skip probeUnknown, go to grabPlain.
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"}).AddRow(true))
	// grabPlain stores the banner.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "", AllowExtendedProbe: true}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_UnknownService_CapExceeded verifies that when AllowExtendedProbe
// is false (cap exceeded), grabOne goes directly to grabPlain without checking
// the DB or running probeUnknown.
func TestGrabOne_UnknownService_CapExceeded(t *testing.T) {
	banner := "CUSTOM PROTO 1.0\r\n"
	addr := startTCPServer(t, banner)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// No DB query expected — cap is exceeded, probeUnknown is skipped entirely.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "", AllowExtendedProbe: false}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 6.2: Run the tests**

```bash
go test -tags '!race' ./internal/enrichment/... -run "TestGrabOne_UnknownService" -v
```

Expected: PASS for all three tests.

- [ ] **Step 6.3: Run the full suite one more time**

```bash
go test -tags '!race' ./internal/enrichment/... -count=1
go test ./internal/db/... -count=1
```

Expected: all tests pass in both packages.

- [ ] **Step 6.4: Commit**

```bash
git add internal/enrichment/banner_zgrab2_test.go
git commit -m "test(enrichment): add grabOne routing tests for extended probe logic"
```

---

## Task 7: Final verification

- [ ] **Step 7.1: Run the fuller backend test suite**

```bash
go test -race ./internal/... -short
```

Expected: all tests pass (zgrab2 tests are excluded by the `!race` build tag and will not run here — that's expected).

- [ ] **Step 7.2: Run lint**

```bash
make lint
```

Expected: 0 issues. If any `lll` (line-length) violations appear, break the offending lines at a logical point — function argument lists and SQL strings are the most likely culprits.

- [ ] **Step 7.3: Verify swagger is not affected**

```bash
make docs
git diff --exit-code docs/swagger/ frontend/src/api/types.ts
```

Expected: no diff — this feature adds no new API endpoints or response types.
