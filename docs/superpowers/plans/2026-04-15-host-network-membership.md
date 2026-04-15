# Host ↔ Network Membership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple `networks` from scan execution — the table becomes a user-curated catalog, host↔network membership is derived via CIDR containment, and existing ad-hoc `/32` pollution is cleaned up via an interactive CLI tool.

**Architecture:** Add a `host_network_memberships` SQL view that joins via `h.ip_address <<= n.cidr`. Replace `findOrCreateNetwork` (which creates `/32` pseudo-networks) with `findContainingNetwork` (pure lookup, returns `uuid.Nil` when no registered network contains the target). Backfill `host_count` when a network is registered after hosts already exist. Expose memberships via `GET /hosts/{id}/networks`. Provide `scanorama networks cleanup-adhoc` to detach-and-delete existing pollution without losing scan history.

**Tech Stack:** Go (backend, PostgreSQL via pq/sqlx), Cobra (CLI), React Query + TypeScript (frontend). Postgres `inet`/`cidr` types with GIST indexes.

**Spec:** `docs/superpowers/specs/2026-04-15-host-network-membership-design.md`

---

## File Structure

### New files
- `internal/db/025_host_network_memberships.sql` — view migration
- `internal/scanning/find_containing_network.go` — extracted lookup helper + tests live alongside
- `internal/scanning/find_containing_network_test.go`
- `cmd/cli/networks_cleanup.go` — `cleanup-adhoc` subcommand
- `cmd/cli/networks_cleanup_test.go`
- `frontend/src/api/hooks/use-host-networks.ts`
- `frontend/src/api/hooks/use-host-networks.test.ts`

### Modified files
- `internal/scanning/scan.go` — swap `findOrCreateNetwork` call site for new helper; remove old function
- `internal/scanning/scan_db_test.go` — update/replace `TestFindOrCreateNetwork_*` tests
- `internal/services/networks.go` — backfill `host_count` after `CreateNetwork`
- `internal/services/networks_test.go` — cover backfill
- `internal/services/hosts.go` — add `GetHostNetworks` service method
- `internal/services/hosts_test.go` — cover new method
- `internal/api/handlers/hosts.go` — add `GetHostNetworks` handler
- `internal/api/handlers/hosts_test.go` — cover handler
- `internal/api/routes.go` — register new route
- `docs/swagger_docs.go` — annotate new endpoint
- `cmd/cli/networks.go` — register `cleanup-adhoc` subcommand
- `frontend/src/routes/hosts.tsx` — consume new hook, render "Member of" section
- `frontend/src/routes/hosts.test.tsx` — mock new hook

---

## Task 1: Add `host_network_memberships` view migration

**Files:**
- Create: `internal/db/025_host_network_memberships.sql`

- [ ] **Step 1: Write the migration**

Create the file with this exact content:

```sql
-- Migration 025: Host ↔ Network membership view
--
-- Derived many-to-many relationship between hosts and registered networks.
-- A host is a member of every network whose CIDR contains its IP address.
-- Uses the existing GIST index on hosts.ip_address and networks.cidr for
-- fast lookups.  No materialisation; always reflects current data.

CREATE OR REPLACE VIEW host_network_memberships AS
SELECT
    h.id              AS host_id,
    n.id              AS network_id,
    h.ip_address      AS ip_address,
    n.cidr            AS cidr,
    masklen(n.cidr)   AS mask_len
FROM hosts h
JOIN networks n ON h.ip_address <<= n.cidr;

COMMENT ON VIEW host_network_memberships IS
    'Derived host↔network membership via CIDR containment. '
    'A host is a member of every registered network whose CIDR contains its IP. '
    'Use mask_len DESC for longest-prefix ordering.';
```

- [ ] **Step 2: Apply the migration locally and sanity-check**

Run:

```bash
make dev-db-reset   # or whatever the project's reset-migrations target is
# Alternatively, if no reset target, apply the file directly:
#   psql "$DATABASE_URL" -f internal/db/025_host_network_memberships.sql
```

Expected: no errors. Then:

```bash
psql "$DATABASE_URL" -c "SELECT * FROM host_network_memberships LIMIT 1;"
```

Expected: either empty result (if no data) or a row with columns `host_id`, `network_id`, `ip_address`, `cidr`, `mask_len`. Column types reported by `\d host_network_memberships` should include `INET` for `ip_address` and `CIDR` for `cidr`.

- [ ] **Step 3: Commit**

```bash
git add internal/db/025_host_network_memberships.sql
git commit -m "feat(db): add host_network_memberships view

Derived many-to-many membership between hosts and registered networks via
CIDR containment. Zero maintenance — always reflects current data using
existing GIST indexes on hosts.ip_address and networks.cidr."
```

---

## Task 2: Extract `findContainingNetwork` and replace `findOrCreateNetwork`

**Files:**
- Create: `internal/scanning/find_containing_network.go`
- Create: `internal/scanning/find_containing_network_test.go`
- Modify: `internal/scanning/scan.go` (replace call site, delete old function)
- Modify: `internal/scanning/scan_db_test.go` (update tests)

- [ ] **Step 1: Write the failing test**

Create `internal/scanning/find_containing_network_test.go`:

```go
// Package scanning — tests for findContainingNetwork.
package scanning

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlDB}, mock
}

const findContainingNetworkQuery = `SELECT id FROM networks WHERE $1::inet <<= cidr ORDER BY masklen(cidr) DESC LIMIT 1`

func TestFindContainingNetwork_NoMatch(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(findContainingNetworkQuery).
		WithArgs("10.0.0.5").
		WillReturnError(sql.ErrNoRows)

	id, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, id, "no containing network should yield uuid.Nil, not an error")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_SingleMatch(t *testing.T) {
	database, mock := newMockDB(t)
	want := uuid.New()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(want)
	mock.ExpectQuery(findContainingNetworkQuery).
		WithArgs("10.0.0.5").
		WillReturnRows(rows)

	got, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_LongestPrefixWins(t *testing.T) {
	// The SQL is responsible for ORDER BY masklen DESC LIMIT 1, so from the
	// Go side this test just asserts that the query we execute returns the
	// single row the DB would produce — we do not re-rank in Go.
	database, mock := newMockDB(t)
	slash24ID := uuid.New()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(slash24ID)
	mock.ExpectQuery(findContainingNetworkQuery).
		WithArgs("10.0.0.5").
		WillReturnRows(rows)

	got, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, slash24ID, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_DBError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(findContainingNetworkQuery).
		WithArgs("10.0.0.5").
		WillReturnError(assert.AnError)

	id, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.Error(t, err)
	assert.Equal(t, uuid.Nil, id)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/scanning -run TestFindContainingNetwork -v
```

Expected: FAIL with "undefined: findContainingNetwork".

- [ ] **Step 3: Implement the helper**

Create `internal/scanning/find_containing_network.go`:

```go
// Package scanning — helper that resolves a scan target to the registered
// network (if any) that should be stored as scan_jobs.network_id.
package scanning

import (
	"context"
	stderrors "errors"
	"database/sql"
	"fmt"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

// findContainingNetwork returns the UUID of the most-specific registered
// network whose CIDR contains target (passed as any valid PostgreSQL inet
// literal — a bare IP, /32, /128, or broader CIDR).  Returns uuid.Nil when
// no registered network contains the target; that is not an error, it is
// the common case for ad-hoc scans.  Returns a non-nil error only for
// genuine DB failures.
//
// Longest-prefix-match semantics: if both 10.0.0.0/16 and 10.0.0.0/24 are
// registered and target is 10.0.0.5, the /24 wins.
func findContainingNetwork(ctx context.Context, database *db.DB, target string) (uuid.UUID, error) {
	const query = `SELECT id FROM networks WHERE $1::inet <<= cidr ORDER BY masklen(cidr) DESC LIMIT 1`

	var id uuid.UUID
	err := database.QueryRowContext(ctx, query, target).Scan(&id)
	if err == nil {
		return id, nil
	}
	if stderrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	return uuid.Nil, fmt.Errorf("failed to look up containing network for %q: %w", target, err)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
go test ./internal/scanning -run TestFindContainingNetwork -v
```

Expected: PASS (all four subtests).

- [ ] **Step 5: Update the call site in `scan.go`**

Open `internal/scanning/scan.go`. Find the `else` branch around line 641 that calls `findOrCreateNetwork`. Replace lines 642–668 (the entire "No caller-supplied ID" block that invokes `findOrCreateNetwork` and constructs the scan job) with:

```go
} else {
	// No caller-supplied ID — standalone (non-API) scan run.
	// Resolve the target to an IP, then look up any registered network
	// that contains it.  Ad-hoc scans with no matching network simply
	// record scan_jobs.network_id = NULL; the catalog is user-curated.
	target := config.Targets[0]
	lookupKey := normaliseToCIDR(target)

	networkID, err := findContainingNetwork(ctx, database, lookupKey)
	if err != nil {
		logging.Error("Failed to look up containing network", "error", err)
		return fmt.Errorf("failed to look up containing network: %w", err)
	}
	if networkID == uuid.Nil {
		logging.Debug("Ad-hoc scan has no registered containing network", "target", target)
	} else {
		logging.Debug("Ad-hoc scan attached to registered network",
			"target", target, "network_id", networkID)
	}

	jobID = uuid.New()
	now := time.Now()
	scanJob := &db.ScanJob{
		ID:     jobID,
		Status: db.ScanJobStatusCompleted,
	}
	if networkID != uuid.Nil {
		scanJob.NetworkID = &networkID
	}
	scanJob.StartedAt = &result.StartTime
	scanJob.CompletedAt = &now
	scanJob.ScanStats = db.JSONB(statsJSON)

	logging.Debug("Creating scan job", "job_id", scanJob.ID, "network_id", scanJob.NetworkID)
	if err := jobRepo.Create(ctx, scanJob); err != nil {
		logging.Error("Failed to create scan job", "error", err)
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	logging.Debug("Successfully created scan job", "job_id", scanJob.ID)
}
```

- [ ] **Step 6: Delete the now-unused `findOrCreateNetwork` function**

In `internal/scanning/scan.go`, delete the entire `findOrCreateNetwork` function (lines 737–800 in the pre-change file). Also remove any now-unused imports: run `goimports -w internal/scanning/scan.go` afterwards.

- [ ] **Step 7: Update `scan_db_test.go` to reflect the new contract**

Open `internal/scanning/scan_db_test.go`. Delete every test named `TestFindOrCreateNetwork_*`. Add one new integration-style test verifying the attachment behaviour. Append this test:

```go
// TestAdhocScan_NoContainingNetwork verifies that a standalone scan run
// whose target is not inside any registered network stores NULL as the
// scan_jobs.network_id rather than inventing a /32 pseudo-network row.
func TestAdhocScan_NoContainingNetwork(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	// Seed: no networks at all.
	var netCount int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM networks`).Scan(&netCount))
	require.Equal(t, 0, netCount)

	ctx := context.Background()
	id, err := findContainingNetwork(ctx, database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, id)

	// After the lookup, nothing was inserted.
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM networks`).Scan(&netCount))
	assert.Equal(t, 0, netCount, "ad-hoc lookup must not create network rows")
}

// TestAdhocScan_AttachesToContainingNetwork verifies that a standalone
// scan of an IP inside a registered network attaches to it.
func TestAdhocScan_AttachesToContainingNetwork(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	want := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES ($1, 'lab', '10.0.0.0/24', '22,80', 'connect', true, true, 'tcp')
	`, want)
	require.NoError(t, err)

	got, err := findContainingNetwork(ctx, database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestAdhocScan_LongestPrefixWins verifies that when two registered networks
// both contain the target IP, the most-specific one is chosen.
func TestAdhocScan_LongestPrefixWins(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	slash16 := uuid.New()
	slash24 := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES
		  ($1, 'corp', '10.0.0.0/16', '22,80', 'connect', true, true, 'tcp'),
		  ($2, 'dmz',  '10.0.0.0/24', '22,80', 'connect', true, true, 'tcp')
	`, slash16, slash24)
	require.NoError(t, err)

	got, err := findContainingNetwork(ctx, database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, slash24, got, "longest-prefix (/24) must win over /16")
}
```

If `testutil.NewTestDB` doesn't exist in the project, use whatever existing helper the current `scan_db_test.go` uses to obtain a real DB — copy the import and setup pattern from the previous `TestFindOrCreateNetwork_*` tests you just deleted.

- [ ] **Step 8: Run the full scanning test suite**

Run:

```bash
go test -race ./internal/scanning/... -v
```

Expected: PASS. If it fails on `findOrCreateNetwork` references elsewhere (e.g. leftover test helpers), delete those too.

- [ ] **Step 9: Commit**

```bash
git add internal/scanning/
git commit -m "feat(scanning): replace findOrCreateNetwork with containment lookup

Ad-hoc scans no longer invent /32 pseudo-networks. Instead we look up any
registered network whose CIDR contains the target (longest-prefix match)
and attach via scan_jobs.network_id, or store NULL when no registered
network matches. networks stays a user-curated catalog."
```

---

## Task 3: Backfill `host_count` when a network is registered

**Files:**
- Modify: `internal/services/networks.go:644` (the `CreateNetwork` method)
- Modify: `internal/services/networks_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/networks_test.go`:

```go
// TestCreateNetwork_BackfillsHostCount verifies that when a network is
// registered after hosts already exist inside its CIDR, host_count and
// active_host_count reflect those existing hosts (not 0).
func TestCreateNetwork_BackfillsHostCount(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed four hosts inside 10.0.0.0/24; three up, one down.
	_, err := database.ExecContext(ctx, `
		INSERT INTO hosts (ip_address, status)
		VALUES ('10.0.0.1', 'up'),
		       ('10.0.0.2', 'up'),
		       ('10.0.0.3', 'up'),
		       ('10.0.0.4', 'down')
	`)
	require.NoError(t, err)

	// One host outside the range — must not be counted.
	_, err = database.ExecContext(ctx, `
		INSERT INTO hosts (ip_address, status) VALUES ('10.1.0.1', 'up')
	`)
	require.NoError(t, err)

	svc := NewNetworkService(database)
	network, err := svc.CreateNetwork(ctx, "lab", "10.0.0.0/24", "", "tcp", true, true)
	require.NoError(t, err)

	// Re-read to observe post-backfill counts.
	var hostCount, activeCount int
	require.NoError(t, database.QueryRow(
		`SELECT host_count, active_host_count FROM networks WHERE id = $1`, network.ID,
	).Scan(&hostCount, &activeCount))

	assert.Equal(t, 4, hostCount, "all four in-range hosts counted")
	assert.Equal(t, 3, activeCount, "only three up hosts counted as active")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/services -run TestCreateNetwork_BackfillsHostCount -v
```

Expected: FAIL — `host_count = 0`, not 4.

- [ ] **Step 3: Implement the backfill**

In `internal/services/networks.go`, inside `CreateNetwork`, immediately after the `Scan(&network.ID, ...)` call succeeds (and before the `return &network, nil`), add:

```go
	// Backfill host counts from any pre-existing hosts that fall inside
	// the newly registered CIDR.  The existing trigger only fires on
	// hosts INSERT/UPDATE/DELETE, so without this step host_count stays
	// at zero until something touches the hosts table.
	const backfillQuery = `
		UPDATE networks
		SET
			host_count = (
				SELECT COUNT(*) FROM hosts WHERE ip_address <<= $1
			),
			active_host_count = (
				SELECT COUNT(*) FROM hosts
				WHERE ip_address <<= $1 AND status = 'up'
			),
			updated_at = NOW()
		WHERE id = $2
		RETURNING host_count, active_host_count, updated_at
	`
	if err := s.database.QueryRowContext(
		ctx, backfillQuery, cidr, network.ID,
	).Scan(&network.HostCount, &network.ActiveHostCount, &network.UpdatedAt); err != nil {
		return nil, fmt.Errorf("failed to backfill host counts: %w", err)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
go test ./internal/services -run TestCreateNetwork_BackfillsHostCount -v
```

Expected: PASS.

- [ ] **Step 5: Run the full services test suite**

Run:

```bash
go test -race ./internal/services/... -v
```

Expected: PASS. Existing tests that create a network and immediately assert `host_count = 0` may now fail if there happen to be hosts in the test DB whose IPs fall inside the new CIDR — update those assertions to allow the real count or use a CIDR that's guaranteed empty.

- [ ] **Step 6: Commit**

```bash
git add internal/services/networks.go internal/services/networks_test.go
git commit -m "feat(services): backfill host_count on network creation

The update_network_host_counts trigger only fires on hosts table changes,
so registering a network after hosts exist left host_count at 0. Compute
counts once at creation time using ip_address <<= cidr."
```

---

## Task 4: Add `GET /hosts/{id}/networks` endpoint

**Files:**
- Modify: `internal/services/hosts.go` (add `GetHostNetworks` method)
- Modify: `internal/services/hosts_test.go`
- Modify: `internal/api/handlers/hosts.go` (add handler)
- Modify: `internal/api/handlers/hosts_test.go`
- Modify: `internal/api/routes.go`
- Modify: `docs/swagger_docs.go`

- [ ] **Step 1: Write the failing service test**

Append to `internal/services/hosts_test.go`:

```go
// TestGetHostNetworks_ReturnsContainingNetworks verifies that a host's
// membership list contains every registered network whose CIDR contains
// the host's IP, ordered by longest prefix first.
func TestGetHostNetworks_ReturnsContainingNetworks(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed: one host and three networks — two contain it, one does not.
	var hostID uuid.UUID
	require.NoError(t, database.QueryRowContext(ctx, `
		INSERT INTO hosts (ip_address, status) VALUES ('10.0.0.5', 'up')
		RETURNING id
	`).Scan(&hostID))

	slash16 := uuid.New()
	slash24 := uuid.New()
	unrelated := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES
		  ($1, 'corp',  '10.0.0.0/16', '22', 'connect', true, true, 'tcp'),
		  ($2, 'dmz',   '10.0.0.0/24', '22', 'connect', true, true, 'tcp'),
		  ($3, 'other', '192.168.0.0/24', '22', 'connect', true, true, 'tcp')
	`, slash16, slash24, unrelated)
	require.NoError(t, err)

	svc := NewHostService(database)
	got, err := svc.GetHostNetworks(ctx, hostID)
	require.NoError(t, err)

	require.Len(t, got, 2, "host should belong to exactly two networks")
	assert.Equal(t, slash24, got[0].ID, "longest prefix first")
	assert.Equal(t, slash16, got[1].ID)
}

// TestGetHostNetworks_NoMemberships returns an empty slice (not nil) when
// the host belongs to no registered network.
func TestGetHostNetworks_NoMemberships(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	var hostID uuid.UUID
	require.NoError(t, database.QueryRowContext(ctx, `
		INSERT INTO hosts (ip_address, status) VALUES ('192.0.2.1', 'up')
		RETURNING id
	`).Scan(&hostID))

	svc := NewHostService(database)
	got, err := svc.GetHostNetworks(ctx, hostID)
	require.NoError(t, err)
	assert.NotNil(t, got, "empty result must be [] not nil for JSON encoding")
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/services -run TestGetHostNetworks -v
```

Expected: FAIL — `GetHostNetworks` undefined.

- [ ] **Step 3: Implement the service method**

Append to `internal/services/hosts.go` (after the last existing method):

```go
// GetHostNetworks returns every registered network whose CIDR contains the
// host's IP address, ordered by longest prefix first.  Returns an empty
// slice (never nil) when the host belongs to no registered network, so the
// JSON wire format is "[]" not "null".
func (s *HostService) GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error) {
	const query = `
		SELECT n.id, n.name, n.cidr, n.description, n.discovery_method,
		       n.is_active, n.scan_enabled, n.last_discovery, n.last_scan,
		       n.host_count, n.active_host_count, n.created_at, n.updated_at, n.created_by
		FROM host_network_memberships m
		JOIN networks n ON n.id = m.network_id
		WHERE m.host_id = $1
		ORDER BY m.mask_len DESC
	`
	rows, err := s.database.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, fmt.Errorf("failed to query host networks: %w", err)
	}
	defer rows.Close()

	out := make([]*db.Network, 0)
	for rows.Next() {
		n := &db.Network{}
		if err := rows.Scan(
			&n.ID, &n.Name, &n.CIDR, &n.Description, &n.DiscoveryMethod,
			&n.IsActive, &n.ScanEnabled, &n.LastDiscovery, &n.LastScan,
			&n.HostCount, &n.ActiveHostCount, &n.CreatedAt, &n.UpdatedAt, &n.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan network row: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate network rows: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run the service test to verify it passes**

Run:

```bash
go test ./internal/services -run TestGetHostNetworks -v
```

Expected: PASS.

- [ ] **Step 5: Write the failing handler test**

Append to `internal/api/handlers/hosts_test.go`:

```go
// TestHostHandler_GetHostNetworks_Success verifies the happy-path
// JSON shape and status code.
func TestHostHandler_GetHostNetworks_Success(t *testing.T) {
	hostID := uuid.New()
	netA := &db.Network{ID: uuid.New(), Name: "dmz", CIDR: "10.0.0.0/24"}
	netB := &db.Network{ID: uuid.New(), Name: "corp", CIDR: "10.0.0.0/16"}

	svc := &mockHostServicer{
		getHostNetworksFn: func(_ context.Context, id uuid.UUID) ([]*db.Network, error) {
			assert.Equal(t, hostID, id)
			return []*db.Network{netA, netB}, nil
		},
	}
	h := NewHostHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/hosts/"+hostID.String()+"/networks", nil)
	req = mux.SetURLVars(req, map[string]string{"hostId": hostID.String()})
	w := httptest.NewRecorder()

	h.GetHostNetworks(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body []map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	require.Len(t, body, 2)
	assert.Equal(t, "dmz", body[0]["name"])
	assert.Equal(t, "10.0.0.0/24", body[0]["cidr"])
	assert.Equal(t, "corp", body[1]["name"])
}

// TestHostHandler_GetHostNetworks_EmptyIsArray verifies that no-memberships
// encodes as "[]\n", not "null\n".
func TestHostHandler_GetHostNetworks_EmptyIsArray(t *testing.T) {
	hostID := uuid.New()
	svc := &mockHostServicer{
		getHostNetworksFn: func(_ context.Context, _ uuid.UUID) ([]*db.Network, error) {
			return []*db.Network{}, nil
		},
	}
	h := NewHostHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/hosts/"+hostID.String()+"/networks", nil)
	req = mux.SetURLVars(req, map[string]string{"hostId": hostID.String()})
	w := httptest.NewRecorder()

	h.GetHostNetworks(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]\n", w.Body.String())
}

// TestHostHandler_GetHostNetworks_BadID returns 400 on a non-UUID path param.
func TestHostHandler_GetHostNetworks_BadID(t *testing.T) {
	h := NewHostHandler(&mockHostServicer{})
	req := httptest.NewRequest(http.MethodGet, "/hosts/not-a-uuid/networks", nil)
	req = mux.SetURLVars(req, map[string]string{"hostId": "not-a-uuid"})
	w := httptest.NewRecorder()

	h.GetHostNetworks(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHostHandler_GetHostNetworks_ServiceError returns 500.
func TestHostHandler_GetHostNetworks_ServiceError(t *testing.T) {
	hostID := uuid.New()
	svc := &mockHostServicer{
		getHostNetworksFn: func(_ context.Context, _ uuid.UUID) ([]*db.Network, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewHostHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/hosts/"+hostID.String()+"/networks", nil)
	req = mux.SetURLVars(req, map[string]string{"hostId": hostID.String()})
	w := httptest.NewRecorder()

	h.GetHostNetworks(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
```

Add to the `mockHostServicer` struct in the same file:

```go
	getHostNetworksFn func(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error)
```

And the method:

```go
func (m *mockHostServicer) GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error) {
	if m.getHostNetworksFn == nil {
		return []*db.Network{}, nil
	}
	return m.getHostNetworksFn(ctx, hostID)
}
```

- [ ] **Step 6: Run the handler tests to verify they fail**

Run:

```bash
go test ./internal/api/handlers -run TestHostHandler_GetHostNetworks -v
```

Expected: FAIL — method not defined on `HostHandler`.

- [ ] **Step 7: Implement the handler**

Add to the `HostServicer` interface in `internal/api/handlers/hosts.go`:

```go
	GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error)
```

Append to `internal/api/handlers/hosts.go`:

```go
// GetHostNetworks returns the list of registered networks containing this
// host, ordered by longest prefix first.  Empty list encodes as "[]".
//
// @Summary List networks a host is a member of
// @Tags hosts
// @Produce json
// @Param hostId path string true "Host UUID"
// @Success 200 {array} db.Network
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 500 {object} apierrors.ErrorResponse
// @Router /hosts/{hostId}/networks [get]
func (h *HostHandler) GetHostNetworks(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["hostId"]
	hostID, err := uuid.Parse(idStr)
	if err != nil {
		apierrors.WriteError(w, http.StatusBadRequest,
			apierrors.NewScanError(apierrors.CodeInvalidInput,
				fmt.Sprintf("invalid host id %q: %v", idStr, err)))
		return
	}

	networks, err := h.service.GetHostNetworks(r.Context(), hostID)
	if err != nil {
		apierrors.WriteError(w, http.StatusInternalServerError,
			apierrors.NewScanError(apierrors.CodeUnknown,
				fmt.Sprintf("failed to list host networks: %v", err)))
		return
	}
	if networks == nil {
		networks = make([]*db.Network, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(networks); err != nil {
		logging.Error("failed to encode host networks response", "error", err)
	}
}
```

- [ ] **Step 8: Register the route**

Open `internal/api/routes.go`. Find the block where other `/hosts/{hostId}/...` routes are registered. Add:

```go
	router.HandleFunc("/hosts/{hostId}/networks", hostHandler.GetHostNetworks).Methods(http.MethodGet)
```

Match the existing pattern for path segment naming — if the project uses `{id}` elsewhere, use `{id}` here too and update the handler accordingly.

- [ ] **Step 9: Regenerate swagger**

Run:

```bash
make docs
git status docs/swagger frontend/src/api/types.ts
```

Expected: regenerated files in `docs/swagger/` and `frontend/src/api/types.ts` reflecting the new endpoint.

- [ ] **Step 10: Run handler + service tests**

Run:

```bash
go test -race ./internal/api/handlers/... ./internal/services/... -v
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/services/hosts.go internal/services/hosts_test.go \
        internal/api/handlers/hosts.go internal/api/handlers/hosts_test.go \
        internal/api/routes.go docs/swagger/ frontend/src/api/types.ts \
        docs/swagger_docs.go
git commit -m "feat(api): add GET /hosts/{hostId}/networks

Returns every registered network whose CIDR contains the host, ordered
by longest prefix first. Backed by the host_network_memberships view.
Empty membership encodes as [] for wire consistency."
```

---

## Task 5: CLI `scanorama networks cleanup-adhoc` command

**Files:**
- Create: `cmd/cli/networks_cleanup.go`
- Create: `cmd/cli/networks_cleanup_test.go`
- Modify: `cmd/cli/networks.go` (register subcommand)

- [ ] **Step 1: Write the failing test**

Create `cmd/cli/networks_cleanup_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupAdhoc_DryRunTouchesNothing ensures --dry-run lists candidates
// without issuing any DELETE or UPDATE statements.
func TestCleanupAdhoc_DryRunTouchesNothing(t *testing.T) {
	database, cleanup := newTestDB(t)
	defer cleanup()
	ctx := context.Background()

	adhocID := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES ($1, 'Ad-hoc: 10.0.0.5/32', '10.0.0.5/32',
		        '22', 'connect', false, false, 'tcp')
	`, adhocID)
	require.NoError(t, err)

	var out bytes.Buffer
	err = runCleanupAdhoc(ctx, database, &out, cleanupOpts{DryRun: true, AssumeYes: true})
	require.NoError(t, err)

	assert.Contains(t, out.String(), "10.0.0.5/32", "dry-run lists the candidate")
	assert.Contains(t, out.String(), "dry-run", "output mentions dry-run")

	// Row must still exist.
	var count int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM networks WHERE id = $1`, adhocID,
	).Scan(&count))
	assert.Equal(t, 1, count, "dry-run must not delete")
}

// TestCleanupAdhoc_DeletesAndDetaches verifies that a real run detaches
// scan_jobs (sets network_id = NULL) before deleting the network row, so
// scan history is preserved.
func TestCleanupAdhoc_DeletesAndDetaches(t *testing.T) {
	database, cleanup := newTestDB(t)
	defer cleanup()
	ctx := context.Background()

	adhocID := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES ($1, 'Ad-hoc: 10.0.0.5/32', '10.0.0.5/32',
		        '22', 'connect', false, false, 'tcp')
	`, adhocID)
	require.NoError(t, err)

	jobID := uuid.New()
	_, err = database.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, network_id, status)
		VALUES ($1, $2, 'completed')
	`, jobID, adhocID)
	require.NoError(t, err)

	var out bytes.Buffer
	err = runCleanupAdhoc(ctx, database, &out, cleanupOpts{AssumeYes: true})
	require.NoError(t, err)

	// Network row gone.
	var netCount int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM networks WHERE id = $1`, adhocID,
	).Scan(&netCount))
	assert.Equal(t, 0, netCount)

	// Scan job still exists but detached.
	var jobCount int
	var jobNet *uuid.UUID
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*), MIN(network_id) FROM scan_jobs WHERE id = $1`, jobID,
	).Scan(&jobCount, &jobNet))
	assert.Equal(t, 1, jobCount, "scan history preserved")
	assert.Nil(t, jobNet, "network_id detached to NULL")

	assert.True(t,
		strings.Contains(out.String(), "deleted") || strings.Contains(out.String(), "removed"),
		"output reports the action taken")
}

// TestCleanupAdhoc_SpareUserRegistered verifies that networks without the
// "Ad-hoc:" name prefix are never touched, even when mask is /32.
func TestCleanupAdhoc_SpareUserRegistered(t *testing.T) {
	database, cleanup := newTestDB(t)
	defer cleanup()
	ctx := context.Background()

	userID := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES ($1, 'prod monitoring host', '10.0.0.5/32',
		        '22', 'connect', true, true, 'tcp')
	`, userID)
	require.NoError(t, err)

	var out bytes.Buffer
	err = runCleanupAdhoc(ctx, database, &out, cleanupOpts{AssumeYes: true})
	require.NoError(t, err)

	var count int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM networks WHERE id = $1`, userID,
	).Scan(&count))
	assert.Equal(t, 1, count, "user-registered /32 must not be swept")
}
```

If no `newTestDB` helper exists in `cmd/cli`, use the same helper pattern other CLI tests in the project use, or add a minimal one that returns a live connection to the configured test DB.

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./cmd/cli -run TestCleanupAdhoc -v
```

Expected: FAIL — `runCleanupAdhoc`, `cleanupOpts` undefined.

- [ ] **Step 3: Implement the command**

Create `cmd/cli/networks_cleanup.go`:

```go
// Package cli — networks cleanup-adhoc subcommand.
//
// Removes legacy /32 (and /128) "Ad-hoc: ..." network rows produced by
// the old findOrCreateNetwork path.  Scan history is preserved: any
// scan_jobs / discovery_jobs pointing at a target row have their
// network_id nulled out before the row is deleted, so the ON DELETE
// CASCADE does not wipe scan results.
package cli

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type cleanupOpts struct {
	DryRun    bool
	AssumeYes bool
}

type adhocCandidate struct {
	ID        uuid.UUID
	Name      string
	CIDR      string
	CreatedAt time.Time
	HostCount int
}

var networksCleanupAdhocCmd = &cobra.Command{
	Use:   "cleanup-adhoc",
	Short: "Delete legacy /32 Ad-hoc network rows created by old CLI scans",
	Long: `Removes rows in the networks table whose name begins with "Ad-hoc: ".
These were auto-created by CLI scans under the old data model. Scan history
pointing at them is preserved — scan_jobs.network_id is detached (NULLed)
before the network row is deleted.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		opts := cleanupOpts{
			DryRun:    cmd.Flag("dry-run").Changed && cmd.Flag("dry-run").Value.String() == "true",
			AssumeYes: cmd.Flag("yes").Changed && cmd.Flag("yes").Value.String() == "true",
		}
		database, err := openDB(cmd.Context())
		if err != nil {
			return err
		}
		defer database.Close()
		return runCleanupAdhoc(cmd.Context(), database, cmd.OutOrStdout(), opts)
	},
}

func init() {
	networksCleanupAdhocCmd.Flags().Bool("dry-run", false, "List candidates without deleting")
	networksCleanupAdhocCmd.Flags().Bool("yes", false, "Skip per-row confirmation prompt")
}

// runCleanupAdhoc is the testable core of the subcommand.
func runCleanupAdhoc(ctx context.Context, database *db.DB, out io.Writer, opts cleanupOpts) error {
	candidates, err := listAdhocCandidates(ctx, database)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Fprintln(out, "No ad-hoc network rows found.")
		return nil
	}

	fmt.Fprintf(out, "Found %d ad-hoc network row(s):\n", len(candidates))
	for _, c := range candidates {
		fmt.Fprintf(out, "  %s  %s  hosts=%d  created=%s\n",
			c.ID, c.CIDR, c.HostCount, c.CreatedAt.Format(time.RFC3339))
	}

	if opts.DryRun {
		fmt.Fprintln(out, "dry-run: no changes made.")
		return nil
	}

	if !opts.AssumeYes {
		fmt.Fprint(out, "Delete all candidates? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	removed, detached, err := deleteCandidates(ctx, database, candidates)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted %d network(s); detached %d scan/discovery job(s).\n", removed, detached)
	return nil
}

func listAdhocCandidates(ctx context.Context, database *db.DB) ([]adhocCandidate, error) {
	const query = `
		SELECT id, name, cidr::text, created_at, host_count
		FROM networks
		WHERE name LIKE 'Ad-hoc: %'
		ORDER BY created_at ASC
	`
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list ad-hoc networks: %w", err)
	}
	defer rows.Close()

	out := make([]adhocCandidate, 0)
	for rows.Next() {
		var c adhocCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.CIDR, &c.CreatedAt, &c.HostCount); err != nil {
			return nil, fmt.Errorf("failed to scan candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func deleteCandidates(ctx context.Context, database *db.DB, candidates []adhocCandidate) (removed, detached int, err error) {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	ids := make([]uuid.UUID, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE scan_jobs SET network_id = NULL WHERE network_id = ANY($1)`, pqUUIDArray(ids))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to detach scan_jobs: %w", err)
	}
	n, _ := res.RowsAffected()
	detached += int(n)

	res, err = tx.ExecContext(ctx,
		`UPDATE discovery_jobs SET network_id = NULL WHERE network_id = ANY($1)`, pqUUIDArray(ids))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to detach discovery_jobs: %w", err)
	}
	n, _ = res.RowsAffected()
	detached += int(n)

	res, err = tx.ExecContext(ctx,
		`DELETE FROM networks WHERE id = ANY($1)`, pqUUIDArray(ids))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to delete networks: %w", err)
	}
	n, _ = res.RowsAffected()
	removed = int(n)

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("failed to commit: %w", err)
	}
	return removed, detached, nil
}

// pqUUIDArray wraps a []uuid.UUID for use with the pq driver's ANY($1) syntax.
func pqUUIDArray(ids []uuid.UUID) interface{} {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	// pq.Array accepts []string; keep the dependency local to avoid widening
	// the package import surface.
	return pqStringArray(strs)
}
```

If `pqStringArray` / `openDB` helpers don't already exist in the CLI package, check how `cmd/cli/networks_handlers.go` acquires a DB connection and reuse that pattern. If `pq.Array` is needed, import `github.com/lib/pq` and call `pq.Array(ids)` directly instead of writing the wrappers — pick whichever the existing code uses.

- [ ] **Step 4: Register the subcommand**

In `cmd/cli/networks.go`, inside the `init()` block where other subcommands are added to `networksCmd`, append:

```go
	networksCmd.AddCommand(networksCleanupAdhocCmd)
```

- [ ] **Step 5: Run the test to verify it passes**

Run:

```bash
go test -race ./cmd/cli -run TestCleanupAdhoc -v
```

Expected: PASS.

- [ ] **Step 6: Smoke-test the command locally**

Run:

```bash
go run ./cmd/scanorama networks cleanup-adhoc --dry-run
```

Expected: either "No ad-hoc network rows found." or a list of candidates and "dry-run: no changes made."

- [ ] **Step 7: Commit**

```bash
git add cmd/cli/networks_cleanup.go cmd/cli/networks_cleanup_test.go cmd/cli/networks.go
git commit -m "feat(cli): add networks cleanup-adhoc subcommand

Removes legacy /32 Ad-hoc network rows produced by the old
findOrCreateNetwork path. Supports --dry-run and --yes. Preserves scan
history by NULL-ing scan_jobs.network_id and discovery_jobs.network_id
before deleting the network row."
```

---

## Task 6: Frontend — show host's network memberships

**Files:**
- Create: `frontend/src/api/hooks/use-host-networks.ts`
- Create: `frontend/src/api/hooks/use-host-networks.test.ts`
- Modify: `frontend/src/routes/hosts.tsx` (or the host-detail component — locate by searching for the file that uses `useHost`)
- Modify: `frontend/src/routes/hosts.test.tsx`

- [ ] **Step 1: Write the hook test**

Create `frontend/src/api/hooks/use-host-networks.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { api } from "../client";
import { useHostNetworks } from "./use-host-networks";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
  },
}));

const ok = <T,>(data: T) =>
  Promise.resolve({ data, error: undefined, response: new Response() });
const fail = (msg = "boom") =>
  Promise.resolve({
    data: undefined,
    error: { message: msg },
    response: new Response(null, { status: 500 }),
  });

describe("useHostNetworks", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns the list of networks on success", async () => {
    vi.mocked(api.GET).mockImplementation((path: string) => {
      expect(path).toBe("/hosts/{hostId}/networks");
      return ok([
        { id: "a", name: "dmz", cidr: "10.0.0.0/24" },
        { id: "b", name: "corp", cidr: "10.0.0.0/16" },
      ]);
    });

    const { result } = renderHookWithQuery(() => useHostNetworks("host-1"));
    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.[0].cidr).toBe("10.0.0.0/24");
  });

  it("surfaces errors", async () => {
    vi.mocked(api.GET).mockImplementation(() => fail());
    const { result } = renderHookWithQuery(() => useHostNetworks("host-1"));
    await vi.waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHookWithQuery(() => useHostNetworks(""));
    expect(result.current.fetchStatus).toBe("idle");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd frontend && npx vitest run src/api/hooks/use-host-networks.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement the hook**

Create `frontend/src/api/hooks/use-host-networks.ts`:

```ts
import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

export function useHostNetworks(id: string) {
  return useQuery({
    queryKey: ["hosts", id, "networks"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/hosts/{hostId}/networks", {
        params: { path: { hostId: id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data ?? [];
    },
    enabled: !!id,
    staleTime: 30_000,
  });
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
cd frontend && npx vitest run src/api/hooks/use-host-networks.test.ts
```

Expected: PASS.

- [ ] **Step 5: Consume the hook in the host-detail UI**

Find the component that renders the host-detail view. In this codebase it's in `frontend/src/routes/hosts.tsx` (check the file for a component that calls `useHost` — the membership section goes near where host metadata is rendered).

Add to the component:

```tsx
import { useHostNetworks } from "../api/hooks/use-host-networks";

// inside the component body, near useHost:
const { data: memberships } = useHostNetworks(hostId);

// in the JSX, in a sidebar or below the host metadata block:
{memberships && memberships.length > 0 && (
  <section data-testid="host-networks">
    <h3 className="text-sm font-medium text-gray-500">Member of</h3>
    <ul className="mt-1 space-y-1">
      {memberships.map((n) => (
        <li key={n.id}>
          <span className="font-mono text-xs">{n.cidr}</span>
          <span className="ml-2 text-sm">{n.name}</span>
        </li>
      ))}
    </ul>
  </section>
)}
```

Adapt class names to match the existing UI framework used in the component.

- [ ] **Step 6: Add a mock for the hook in the component test file**

In `frontend/src/routes/hosts.test.tsx`, add at the top with the other `vi.mock` calls:

```ts
vi.mock("../api/hooks/use-host-networks", () => ({
  useHostNetworks: vi.fn(),
}));
```

And in `setupDefaultMocks()` (or equivalent), add a default:

```ts
vi.mocked(useHostNetworks).mockReturnValue({
  data: [],
  isLoading: false,
  isError: false,
  error: null,
  isSuccess: true,
  fetchStatus: "idle",
} as any);
```

Add one new test:

```ts
it("renders 'Member of' section when host belongs to networks", async () => {
  vi.mocked(useHostNetworks).mockReturnValue({
    data: [{ id: "a", name: "dmz", cidr: "10.0.0.0/24" }],
    isLoading: false,
    isError: false,
    error: null,
    isSuccess: true,
    fetchStatus: "idle",
  } as any);

  renderWithRouter(<HostDetailRoute />);
  expect(await screen.findByTestId("host-networks")).toBeInTheDocument();
  expect(screen.getByText("10.0.0.0/24")).toBeInTheDocument();
  expect(screen.getByText("dmz")).toBeInTheDocument();
});
```

Replace `HostDetailRoute` with whatever the actual component name is.

- [ ] **Step 7: Run the full frontend test suite**

Run:

```bash
cd frontend && npm test
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/api/hooks/use-host-networks.ts \
        frontend/src/api/hooks/use-host-networks.test.ts \
        frontend/src/routes/hosts.tsx \
        frontend/src/routes/hosts.test.tsx
git commit -m "feat(frontend): show host's network memberships

Host-detail page lists every registered network containing the host,
most-specific first. Backed by the new GET /hosts/{hostId}/networks
endpoint via useHostNetworks."
```

---

## Task 7: Full suite + manual verification

**Files:** none — verification only.

- [ ] **Step 1: Run the full backend test suite**

Run:

```bash
go test -race ./internal/... ./cmd/...
```

Expected: PASS.

- [ ] **Step 2: Run the full frontend test suite**

Run:

```bash
cd frontend && npm test
```

Expected: PASS.

- [ ] **Step 3: Verify no swagger drift**

Run:

```bash
make docs
git diff --exit-code docs/swagger frontend/src/api/types.ts
```

Expected: empty diff.

- [ ] **Step 4: Manual smoke test — ad-hoc scan with no registered container**

With a running dev environment:

```bash
# Confirm no network contains 10.0.0.5 before scanning.
psql "$DATABASE_URL" -c \
  "SELECT id, cidr FROM networks WHERE '10.0.0.5'::inet <<= cidr;"
# Expected: no rows.

go run ./cmd/scanorama scan 10.0.0.5

# After the scan completes:
psql "$DATABASE_URL" -c \
  "SELECT id, network_id FROM scan_jobs ORDER BY completed_at DESC LIMIT 1;"
# Expected: network_id IS NULL.

psql "$DATABASE_URL" -c \
  "SELECT COUNT(*) FROM networks WHERE cidr = '10.0.0.5/32';"
# Expected: 0 (no pseudo-network created).
```

- [ ] **Step 5: Manual smoke test — scan with registered container**

```bash
# Register a /24.
go run ./cmd/scanorama networks add lab 10.0.0.0/24

# Scan a host inside it.
go run ./cmd/scanorama scan 10.0.0.5

# Verify the scan job attached to the /24.
psql "$DATABASE_URL" -c "
  SELECT sj.id, n.cidr
  FROM scan_jobs sj JOIN networks n ON sj.network_id = n.id
  ORDER BY sj.completed_at DESC LIMIT 1;
"
# Expected: network_id matches the /24.
```

- [ ] **Step 6: Manual smoke test — host-detail page**

Open the frontend, navigate to the host scanned above, verify the
"Member of" section lists `10.0.0.0/24  lab`.

- [ ] **Step 7: Manual smoke test — cleanup-adhoc**

```bash
# Plant a legacy row (simulating pre-upgrade state).
psql "$DATABASE_URL" -c "
  INSERT INTO networks (name, cidr, scan_ports, scan_type,
                       is_active, scan_enabled, discovery_method)
  VALUES ('Ad-hoc: 10.0.0.99/32', '10.0.0.99/32',
          '22', 'connect', false, false, 'tcp');
"

go run ./cmd/scanorama networks cleanup-adhoc --dry-run
# Expected: lists 10.0.0.99/32, says dry-run.

go run ./cmd/scanorama networks cleanup-adhoc --yes
# Expected: deleted 1 network.

psql "$DATABASE_URL" -c \
  "SELECT COUNT(*) FROM networks WHERE cidr = '10.0.0.99/32';"
# Expected: 0.
```

- [ ] **Step 8: Open the PR**

Ensure the branch is rebased on main, push, open a PR body that closes the
tracking issue (if one exists) and includes only manual/live checklist items
in the test plan (not things CI already runs).

---

## Self-Review

**Spec coverage:**
- §2.1 user-curated `networks` → Task 2 removes auto-create
- §2.2 derived membership via containment → Task 1 view + Task 4 endpoint
- §2.3 longest-prefix attachment → Task 2 helper + query
- §2.4 no new schema → Task 1 view only, no table changes
- §2.5 interactive cleanup, not migration → Task 5
- §2.6 scan history preservation → Task 5 detaches before delete
- Backfill on network creation (design §"Code changes / networks.go") → Task 3
- New endpoint (design) → Task 4
- Frontend surfacing (design) → Task 6

All spec requirements have a task.

**Placeholder scan:** none found.

**Type consistency:** `findContainingNetwork(ctx, database, target string) (uuid.UUID, error)` — signature identical in task 2 tests, implementation, and call site. `cleanupOpts{DryRun, AssumeYes}` identical across task 5 tests and code. `useHostNetworks(id: string)` consistent across hook, hook test, and component usage in task 6.
