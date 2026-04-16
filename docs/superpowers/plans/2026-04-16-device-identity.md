# Device Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a stable `Device` concept above raw host records so that scan history, tags, and notes survive MAC address randomization and IP churn.

**Architecture:** A `devices` table sits above `hosts`; hosts gain a nullable `device_id` FK. A post-discovery `DeviceMatcher` service scores existing devices using weighted signals (MAC, mDNS name, SNMP sysName, etc.) and auto-attaches or suggests. A new mDNS enricher queries each host via unicast PTR to capture stable `.local` names.

**Tech Stack:** Go 1.23, PostgreSQL (MACADDR type, GIN/GIST indexes), `github.com/miekg/dns` (unicast mDNS), React Query, gorilla/mux, go-sqlmock, gomock.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/db/026_devices.sql` | Create | Migration: 4 new tables + ALTER hosts |
| `internal/db/models_device.go` | Create | Device, DeviceKnownMAC, DeviceKnownName, DeviceSuggestion structs |
| `internal/db/repository_device.go` | Create | DeviceRepository — all device DB access |
| `internal/db/repository_host.go` | Modify | Add UpdateMDNSName; update GetHost/ListHosts to return device fields |
| `internal/enrichment/dns_quality.go` | Create | DNS name quality filter (rejects auto-generated PTR names) |
| `internal/enrichment/dns_quality_test.go` | Create | Table-driven tests for quality filter |
| `internal/enrichment/mdns.go` | Create | MDNSEnricher — unicast PTR via miekg/dns |
| `internal/enrichment/mdns_test.go` | Create | Unit tests with fake DNS server |
| `internal/services/device_matcher.go` | Create | DeviceMatcher — weighted scoring, auto-attach, suggestions |
| `internal/services/device_matcher_test.go` | Create | Unit tests with sqlmock |
| `internal/services/device.go` | Create | DeviceService — CRUD + attach/detach + suggestion accept/dismiss |
| `internal/services/device_test.go` | Create | Unit tests with sqlmock |
| `internal/api/handlers/interfaces.go` | Modify | Add DeviceServicer interface + go:generate directive |
| `internal/api/handlers/devices.go` | Create | DeviceHandler — all device endpoints |
| `internal/api/handlers/devices_test.go` | Create | Handler tests with mocks |
| `internal/api/handlers/mocks/mock_device_servicer.go` | Create | Generated mock (via mockgen) |
| `internal/api/routes.go` | Modify | Register device routes + wire DeviceHandler |
| `frontend/src/api/hooks/use-devices.ts` | Create | React Query hooks for devices + suggestions |
| `frontend/src/routes/devices.tsx` | Create | Device detail page |
| `frontend/src/routes/host-detail.tsx` | Modify | Add Device card |

---

## Task 1: DB Migration

**Files:**
- Create: `internal/db/026_devices.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Migration 026: Device identity
-- Stable device concept above raw host records.
-- A device survives MAC randomization and IP churn.

CREATE TABLE devices (
    id         UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(255) NOT NULL,
    notes      TEXT,
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    updated_at TIMESTAMPTZ  DEFAULT NOW()
);

-- One row per MAC address ever seen for a device.
-- UNIQUE on mac_address: a MAC can only belong to one device.
CREATE TABLE device_known_macs (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id   UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    mac_address MACADDR     NOT NULL,
    first_seen  TIMESTAMPTZ DEFAULT NOW(),
    last_seen   TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_mac UNIQUE (mac_address)
);

-- Stable names ever seen for a device.
-- source IN ('mdns','dns','snmp','netbios','user')
-- UNIQUE on (name, source): same name from same source = one row.
CREATE TABLE device_known_names (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id  UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    source     VARCHAR(20) NOT NULL
                   CHECK (source IN ('mdns', 'dns', 'snmp', 'netbios', 'user')),
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_name UNIQUE (name, source)
);

-- Low-confidence match candidates surfaced for user review.
CREATE TABLE device_suggestions (
    id                UUID    PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id           UUID    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    device_id         UUID    NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    confidence_score  INTEGER NOT NULL,
    confidence_reason TEXT,
    dismissed         BOOLEAN DEFAULT FALSE,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_suggestion UNIQUE (host_id, device_id)
);

-- hosts gains device FK and mDNS name cache.
ALTER TABLE hosts
    ADD COLUMN device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN mdns_name TEXT;

CREATE INDEX idx_hosts_device_id ON hosts(device_id) WHERE device_id IS NOT NULL;
```

- [ ] **Step 2: Verify it parses (no DB needed)**

```bash
psql --help > /dev/null && echo "psql available" || echo "psql not available — syntax check skipped"
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/026_devices.sql
git commit -m "feat(db): add devices tables and alter hosts migration 026"
```

---

## Task 2: DB Models

**Files:**
- Create: `internal/db/models_device.go`

- [ ] **Step 1: Write the models**

```go
package db

import (
	"time"

	"github.com/google/uuid"
)

// Device is a stable identity record that survives MAC and IP churn.
type Device struct {
	ID        uuid.UUID  `db:"id"         json:"id"`
	Name      string     `db:"name"       json:"name"`
	Notes     *string    `db:"notes"      json:"notes,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
}

// DeviceKnownMAC records one MAC address ever seen for a device.
type DeviceKnownMAC struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	DeviceID   uuid.UUID `db:"device_id"   json:"device_id"`
	MACAddress string    `db:"mac_address" json:"mac_address"`
	FirstSeen  time.Time `db:"first_seen"  json:"first_seen"`
	LastSeen   time.Time `db:"last_seen"   json:"last_seen"`
}

// DeviceKnownName records one (name, source) pair ever seen for a device.
// Source is one of: mdns, dns, snmp, netbios, user.
type DeviceKnownName struct {
	ID        uuid.UUID `db:"id"        json:"id"`
	DeviceID  uuid.UUID `db:"device_id" json:"device_id"`
	Name      string    `db:"name"      json:"name"`
	Source    string    `db:"source"    json:"source"`
	FirstSeen time.Time `db:"first_seen" json:"first_seen"`
	LastSeen  time.Time `db:"last_seen"  json:"last_seen"`
}

// DeviceSuggestion is a low-confidence host↔device match candidate.
type DeviceSuggestion struct {
	ID               uuid.UUID `db:"id"                json:"id"`
	HostID           uuid.UUID `db:"host_id"           json:"host_id"`
	DeviceID         uuid.UUID `db:"device_id"         json:"device_id"`
	ConfidenceScore  int       `db:"confidence_score"  json:"confidence_score"`
	ConfidenceReason *string   `db:"confidence_reason" json:"confidence_reason,omitempty"`
	Dismissed        bool      `db:"dismissed"         json:"dismissed"`
	CreatedAt        time.Time `db:"created_at"        json:"created_at"`
}

// DeviceDetail is the full device view returned by GET /devices/{id}.
type DeviceDetail struct {
	Device
	KnownMACs  []DeviceKnownMAC  `json:"known_macs"`
	KnownNames []DeviceKnownName `json:"known_names"`
	Hosts      []Host            `json:"hosts"`
}

// CreateDeviceInput holds fields for creating a device manually.
type CreateDeviceInput struct {
	Name  string  `json:"name"`
	Notes *string `json:"notes,omitempty"`
}

// UpdateDeviceInput holds fields for renaming or re-noting a device.
type UpdateDeviceInput struct {
	Name  *string `json:"name,omitempty"`
	Notes *string `json:"notes,omitempty"`
}

// DeviceSummary is the lightweight row returned by GET /devices.
type DeviceSummary struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	MACCount  int       `json:"mac_count"`
	HostCount int       `json:"host_count"`
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/db/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/db/models_device.go
git commit -m "feat(db): add Device, DeviceKnownMAC, DeviceKnownName, DeviceSuggestion models"
```

---

## Task 3: DeviceRepository

**Files:**
- Create: `internal/db/repository_device.go`

- [ ] **Step 1: Write failing test skeletons**

```go
// internal/db/repository_device_test.go
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

func TestDeviceRepository_CreateDevice(t *testing.T) {
	mock, repo := newDeviceRepo(t)
	ctx := context.Background()
	id := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`INSERT INTO devices`).
		WithArgs("My Router", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "notes", "created_at", "updated_at"}).
			AddRow(id, "My Router", nil, now, now))

	device, err := repo.CreateDevice(ctx, db.CreateDeviceInput{Name: "My Router"})
	require.NoError(t, err)
	assert.Equal(t, "My Router", device.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_GetDevice(t *testing.T) {
	mock, repo := newDeviceRepo(t)
	ctx := context.Background()
	id := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT id, name, notes, created_at, updated_at FROM devices WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "notes", "created_at", "updated_at"}).
			AddRow(id, "My Router", nil, now, now))

	device, err := repo.GetDevice(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, device.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newDeviceRepo(t *testing.T) (sqlmock.Sqlmock, *db.DeviceRepository) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	return mock, db.NewDeviceRepository(db.WrapDB(sqlDB))
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/db/... 2>&1 | head -20
```

Expected: `undefined: db.DeviceRepository`

- [ ] **Step 3: Write the repository**

```go
// internal/db/repository_device.go
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// DeviceRepository handles all device-related DB operations.
type DeviceRepository struct {
	db *DB
}

// NewDeviceRepository creates a new DeviceRepository.
func NewDeviceRepository(db *DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// ListDevices returns a summary list (name, mac_count, host_count).
func (r *DeviceRepository) ListDevices(ctx context.Context) ([]DeviceSummary, error) {
	q := `
		SELECT d.id, d.name,
		       COUNT(DISTINCT m.id) AS mac_count,
		       COUNT(DISTINCT h.id) AS host_count
		FROM devices d
		LEFT JOIN device_known_macs m ON m.device_id = d.id
		LEFT JOIN hosts h             ON h.device_id = d.id
		GROUP BY d.id, d.name
		ORDER BY d.name`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, sanitizeDBError("list devices", err)
	}
	defer rows.Close()

	result := make([]DeviceSummary, 0)
	for rows.Next() {
		var s DeviceSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.MACCount, &s.HostCount); err != nil {
			return nil, fmt.Errorf("scan device summary: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// CreateDevice inserts a new device record.
func (r *DeviceRepository) CreateDevice(ctx context.Context, input CreateDeviceInput) (*Device, error) {
	q := `INSERT INTO devices (name, notes) VALUES ($1, $2)
	      RETURNING id, name, notes, created_at, updated_at`
	d := &Device{}
	err := r.db.QueryRowContext(ctx, q, input.Name, input.Notes).
		Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, sanitizeDBError("create device", err)
	}
	return d, nil
}

// GetDevice retrieves a device by ID.
func (r *DeviceRepository) GetDevice(ctx context.Context, id uuid.UUID) (*Device, error) {
	q := `SELECT id, name, notes, created_at, updated_at FROM devices WHERE id = $1`
	d := &Device{}
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "device not found")
	}
	if err != nil {
		return nil, sanitizeDBError("get device", err)
	}
	return d, nil
}

// UpdateDevice updates name and/or notes.
func (r *DeviceRepository) UpdateDevice(ctx context.Context, id uuid.UUID, input UpdateDeviceInput) (*Device, error) {
	q := `
		UPDATE devices SET
		    name      = COALESCE($2, name),
		    notes     = COALESCE($3, notes),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, notes, created_at, updated_at`
	d := &Device{}
	err := r.db.QueryRowContext(ctx, q, id, input.Name, input.Notes).
		Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "device not found")
	}
	if err != nil {
		return nil, sanitizeDBError("update device", err)
	}
	return d, nil
}

// DeleteDevice removes a device; attached hosts have device_id set to NULL by the FK.
func (r *DeviceRepository) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return sanitizeDBError("delete device", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "device not found")
	}
	return nil
}

// GetDeviceDetail returns the full device with known MACs, names, and attached hosts.
func (r *DeviceRepository) GetDeviceDetail(ctx context.Context, id uuid.UUID) (*DeviceDetail, error) {
	dev, err := r.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &DeviceDetail{Device: *dev}

	// Known MACs
	macs, err := r.listKnownMACs(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.KnownMACs = macs

	// Known names
	names, err := r.listKnownNames(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.KnownNames = names

	// Attached hosts
	hosts, err := r.listAttachedHosts(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.Hosts = hosts

	return detail, nil
}

func (r *DeviceRepository) listKnownMACs(ctx context.Context, deviceID uuid.UUID) ([]DeviceKnownMAC, error) {
	q := `SELECT id, device_id, mac_address, first_seen, last_seen
	      FROM device_known_macs WHERE device_id = $1 ORDER BY last_seen DESC`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, sanitizeDBError("list known macs", err)
	}
	defer rows.Close()

	result := make([]DeviceKnownMAC, 0)
	for rows.Next() {
		var m DeviceKnownMAC
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.MACAddress, &m.FirstSeen, &m.LastSeen); err != nil {
			return nil, fmt.Errorf("scan known mac: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *DeviceRepository) listKnownNames(ctx context.Context, deviceID uuid.UUID) ([]DeviceKnownName, error) {
	q := `SELECT id, device_id, name, source, first_seen, last_seen
	      FROM device_known_names WHERE device_id = $1 ORDER BY source, name`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, sanitizeDBError("list known names", err)
	}
	defer rows.Close()

	result := make([]DeviceKnownName, 0)
	for rows.Next() {
		var n DeviceKnownName
		if err := rows.Scan(&n.ID, &n.DeviceID, &n.Name, &n.Source, &n.FirstSeen, &n.LastSeen); err != nil {
			return nil, fmt.Errorf("scan known name: %w", err)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

func (r *DeviceRepository) listAttachedHosts(ctx context.Context, deviceID uuid.UUID) ([]Host, error) {
	q := `SELECT id, ip_address, mac_address, hostname, status, os_family,
	             vendor, device_id, mdns_name, created_at, updated_at
	      FROM hosts WHERE device_id = $1 ORDER BY ip_address`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, sanitizeDBError("list attached hosts", err)
	}
	defer rows.Close()

	result := make([]Host, 0)
	for rows.Next() {
		var h Host
		if err := rows.Scan(
			&h.ID, &h.IPAddress, &h.MACAddress, &h.Hostname, &h.Status,
			&h.OSFamily, &h.Vendor, &h.DeviceID, &h.MDNSName,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attached host: %w", err)
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// AttachHost sets hosts.device_id = deviceID.
func (r *DeviceRepository) AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET device_id = $1, updated_at = NOW() WHERE id = $2`, deviceID, hostID)
	if err != nil {
		return sanitizeDBError("attach host", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "host not found")
	}
	return nil
}

// DetachHost sets hosts.device_id = NULL.
func (r *DeviceRepository) DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET device_id = NULL, updated_at = NOW() WHERE id = $2 AND device_id = $1`,
		deviceID, hostID)
	if err != nil {
		return sanitizeDBError("detach host", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "host not found or not attached to this device")
	}
	return nil
}

// UpsertKnownMAC inserts or updates a known MAC record for a device.
func (r *DeviceRepository) UpsertKnownMAC(ctx context.Context, deviceID uuid.UUID, mac string) error {
	q := `
		INSERT INTO device_known_macs (device_id, mac_address, first_seen, last_seen)
		VALUES ($1, $2::macaddr, NOW(), NOW())
		ON CONFLICT (mac_address) DO UPDATE
		    SET last_seen = NOW()
		    WHERE device_known_macs.device_id = $1`
	_, err := r.db.ExecContext(ctx, q, deviceID, mac)
	return sanitizeDBError("upsert known mac", err)
}

// UpsertKnownName inserts or updates a known name record for a device.
func (r *DeviceRepository) UpsertKnownName(ctx context.Context, deviceID uuid.UUID, name, source string) error {
	q := `
		INSERT INTO device_known_names (device_id, name, source, first_seen, last_seen)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (name, source) DO UPDATE
		    SET last_seen = NOW()
		    WHERE device_known_names.device_id = $1`
	_, err := r.db.ExecContext(ctx, q, deviceID, name, source)
	return sanitizeDBError("upsert known name", err)
}

// UpsertSuggestion inserts or updates a device suggestion.
func (r *DeviceRepository) UpsertSuggestion(
	ctx context.Context, hostID, deviceID uuid.UUID, score int, reason string,
) error {
	q := `
		INSERT INTO device_suggestions (host_id, device_id, confidence_score, confidence_reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_id, device_id) DO UPDATE
		    SET confidence_score = $3,
		        confidence_reason = $4,
		        dismissed = FALSE`
	_, err := r.db.ExecContext(ctx, q, hostID, deviceID, score, reason)
	return sanitizeDBError("upsert suggestion", err)
}

// AcceptSuggestion attaches the host, learns signals, and removes the suggestion row.
func (r *DeviceRepository) AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	var hostID, deviceID uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT host_id, device_id FROM device_suggestions WHERE id = $1 AND dismissed = FALSE`,
		suggestionID).Scan(&hostID, &deviceID)
	if stderrors.Is(err, sql.ErrNoRows) {
		return errors.NewScanError(errors.CodeNotFound, "suggestion not found")
	}
	if err != nil {
		return sanitizeDBError("get suggestion for accept", err)
	}
	if err := r.AttachHost(ctx, deviceID, hostID); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM device_suggestions WHERE id = $1`, suggestionID)
	return sanitizeDBError("delete accepted suggestion", err)
}

// DismissSuggestion marks a suggestion as dismissed.
func (r *DeviceRepository) DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_suggestions SET dismissed = TRUE WHERE id = $1`, suggestionID)
	if err != nil {
		return sanitizeDBError("dismiss suggestion", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "suggestion not found")
	}
	return nil
}

// FindDeviceByMAC returns the device that owns the given MAC address, or nil.
func (r *DeviceRepository) FindDeviceByMAC(ctx context.Context, mac string) (*Device, error) {
	q := `SELECT d.id, d.name, d.notes, d.created_at, d.updated_at
	      FROM devices d
	      JOIN device_known_macs m ON m.device_id = d.id
	      WHERE m.mac_address = $1::macaddr`
	d := &Device{}
	err := r.db.QueryRowContext(ctx, q, mac).
		Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // intentional: no match is not an error
	}
	if err != nil {
		return nil, sanitizeDBError("find device by mac", err)
	}
	return d, nil
}

// FindDevicesByName returns all devices that have the given name from any source.
func (r *DeviceRepository) FindDevicesByName(ctx context.Context, name, source string) ([]*Device, error) {
	q := `SELECT d.id, d.name, d.notes, d.created_at, d.updated_at
	      FROM devices d
	      JOIN device_known_names n ON n.device_id = d.id
	      WHERE n.name = $1 AND n.source = $2`
	rows, err := r.db.QueryContext(ctx, q, name, source)
	if err != nil {
		return nil, sanitizeDBError("find devices by name", err)
	}
	defer rows.Close()

	result := make([]*Device, 0)
	for rows.Next() {
		d := &Device{}
		if err := rows.Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan device by name: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetSuggestionsForDiscovery returns non-dismissed suggestions for a set of host IDs.
func (r *DeviceRepository) GetSuggestionsForDiscovery(
	ctx context.Context, hostIDs []uuid.UUID,
) ([]DeviceSuggestion, error) {
	if len(hostIDs) == 0 {
		return make([]DeviceSuggestion, 0), nil
	}

	q := `
		SELECT s.id, s.host_id, s.device_id, s.confidence_score, s.confidence_reason,
		       s.dismissed, s.created_at
		FROM device_suggestions s
		WHERE s.host_id = ANY($1) AND s.dismissed = FALSE`

	ids := make([]string, len(hostIDs))
	for i, id := range hostIDs {
		ids[i] = id.String()
	}

	rows, err := r.db.QueryContext(ctx, q, hostIDsToArray(hostIDs))
	if err != nil {
		return nil, sanitizeDBError("get suggestions for discovery", err)
	}
	defer rows.Close()

	result := make([]DeviceSuggestion, 0)
	for rows.Next() {
		var s DeviceSuggestion
		if err := rows.Scan(
			&s.ID, &s.HostID, &s.DeviceID, &s.ConfidenceScore,
			&s.ConfidenceReason, &s.Dismissed, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// AllDevicesWithSignals returns all devices with their known MACs and names for matching.
// Used by DeviceMatcher to score candidates without N+1 queries.
type DeviceSignals struct {
	Device     Device
	KnownMACs  []string // MAC address strings
	KnownNames []struct{ Name, Source string }
}

func (r *DeviceRepository) AllDevicesWithSignals(ctx context.Context) ([]DeviceSignals, error) {
	q := `
		SELECT d.id, d.name, d.notes, d.created_at, d.updated_at,
		       m.mac_address, n.name, n.source
		FROM devices d
		LEFT JOIN device_known_macs  m ON m.device_id = d.id
		LEFT JOIN device_known_names n ON n.device_id = d.id`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, sanitizeDBError("all devices with signals", err)
	}
	defer rows.Close()

	byID := map[uuid.UUID]*DeviceSignals{}
	order := []uuid.UUID{}
	for rows.Next() {
		var (
			d       Device
			mac     *string
			name    *string
			source  *string
		)
		if err := rows.Scan(
			&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt,
			&mac, &name, &source,
		); err != nil {
			return nil, fmt.Errorf("scan device signals: %w", err)
		}
		if _, ok := byID[d.ID]; !ok {
			byID[d.ID] = &DeviceSignals{Device: d}
			order = append(order, d.ID)
		}
		sig := byID[d.ID]
		if mac != nil {
			sig.KnownMACs = append(sig.KnownMACs, *mac)
		}
		if name != nil && source != nil {
			sig.KnownNames = append(sig.KnownNames, struct{ Name, Source string }{*name, *source})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device signals: %w", err)
	}

	result := make([]DeviceSignals, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

// hostIDsToArray converts a slice of UUIDs to the pq array format for $1 = ANY($1).
func hostIDsToArray(ids []uuid.UUID) interface{} {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	return pq.Array(strs)
}

// UpdateMDNSName writes the mDNS .local name to hosts.mdns_name.
func (r *DeviceRepository) UpdateMDNSName(ctx context.Context, hostID uuid.UUID, name string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET mdns_name = $2, updated_at = NOW() WHERE id = $1`, hostID, name)
	return sanitizeDBError("update mdns name", err)
}

// GetKnownMACsForDevice returns MAC strings for the device.
func (r *DeviceRepository) GetKnownMACsForDevice(ctx context.Context, deviceID uuid.UUID) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mac_address FROM device_known_macs WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, sanitizeDBError("get known macs", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("scan mac: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// Ensure pq is imported for hostIDsToArray.
var _ = time.Now
```

- [ ] **Step 4: Add pq import (models already imports it; add to repository)**

Add `"github.com/lib/pq"` to the import block in `repository_device.go` and remove the `var _ = time.Now` placeholder.

- [ ] **Step 5: Build**

```bash
go build ./internal/db/...
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/db/... -run TestDeviceRepository -v
```

Expected: PASS (sqlmock expectations matched).

- [ ] **Step 7: Commit**

```bash
git add internal/db/repository_device.go internal/db/repository_device_test.go
git commit -m "feat(db): add DeviceRepository with CRUD, signals, and suggestion methods"
```

---

## Task 4: Update Host Model and HostRepository

**Files:**
- Modify: `internal/db/models.go` (Host struct)
- Modify: `internal/db/repository_host.go` (UpdateMDNSName, scan device fields)

- [ ] **Step 1: Add device fields to Host struct**

In `internal/db/models.go`, find the `Host` struct and add:

```go
DeviceID   *uuid.UUID `db:"device_id"  json:"device_id,omitempty"`
MDNSName   *string    `db:"mdns_name"  json:"mdns_name,omitempty"`
DeviceName *string    `db:"-"          json:"device_name,omitempty"` // populated by join, not a column
```

- [ ] **Step 2: Update GetHost query to join device name**

In `internal/db/repository_host.go`, in the `GetHost` query, add:

```sql
LEFT JOIN devices dv ON dv.id = h.device_id
```

and add `dv.name` to the SELECT, scan `&h.DeviceID, &h.MDNSName, &h.DeviceName` at the end of the Scan call.

- [ ] **Step 3: Update ListHosts query similarly**

In the `ListHosts` query, add the same LEFT JOIN and scan `device_id`, `mdns_name`, and device `name`.

- [ ] **Step 4: Build**

```bash
go build ./internal/db/...
```

- [ ] **Step 5: Run existing host tests to confirm no regressions**

```bash
go test ./internal/db/... -run TestHost -v -short
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/models.go internal/db/repository_host.go
git commit -m "feat(db): add device_id and mdns_name to Host model and repository queries"
```

---

## Task 5: DNS Quality Filter

**Files:**
- Create: `internal/enrichment/dns_quality.go`
- Create: `internal/enrichment/dns_quality_test.go`

- [ ] **Step 1: Write the failing tests first**

```go
// internal/enrichment/dns_quality_test.go
package enrichment_test

import (
	"testing"

	"github.com/anstrom/scanorama/internal/enrichment"
)

func TestDNSNameQuality(t *testing.T) {
	accept := []string{
		"pihole.local",
		"synology.home",
		"router.lan",
		"myserver.example.com",
	}
	reject := []string{
		"192-168-1-50.local",          // IP literal
		"10.0.0.1",                    // raw IP
		"dhcp-192.example.com",        // DHCP prefix
		"host-10-0-0-1.isp.net",       // host- prefix
		"ip-172-16-0-5.ec2.internal",  // ip- prefix
		"client-abcd.corp",            // client- prefix
		"1.0.168.192.in-addr.arpa",    // pure numeric labels (reversed IP)
		"broadband.12345.dynamic.net", // dynamic suffix
		"pool.dhcp.example.com",       // .dhcp. suffix
		"abc.broadband.isp.com",       // broadband suffix
	}

	for _, name := range accept {
		if !enrichment.DNSNameIsUsable(name) {
			t.Errorf("expected %q to pass quality filter, but it was rejected", name)
		}
	}
	for _, name := range reject {
		if enrichment.DNSNameIsUsable(name) {
			t.Errorf("expected %q to be rejected by quality filter, but it passed", name)
		}
	}
}
```

- [ ] **Step 2: Run — expect failure**

```bash
go test ./internal/enrichment/... -run TestDNSNameQuality -v
```

Expected: `undefined: enrichment.DNSNameIsUsable`

- [ ] **Step 3: Write the filter**

```go
// internal/enrichment/dns_quality.go
package enrichment

import (
	"net"
	"regexp"
	"strings"
)

// dhcpPrefixes are hostname prefixes commonly generated by DHCP servers.
var dhcpPrefixes = []string{"dhcp-", "host-", "ip-", "client-"}

// noiseSuffixes are domain label patterns generated by ISPs or DHCP infrastructure.
var noiseSuffixes = []string{".dynamic.", ".dhcp.", ".broadband."}

// ipLiteralPattern matches labels that look like IP address octets separated by hyphens.
var ipLiteralPattern = regexp.MustCompile(`\b\d{1,3}[-\.]\d{1,3}[-\.]\d{1,3}[-\.]\d{1,3}\b`)

// DNSNameIsUsable returns true if the DNS PTR name is likely a stable, human-assigned
// hostname and not a DHCP-generated or ISP-generated artefact.
//
// Names are rejected if they:
//   - Are parseable as a raw IP address
//   - Contain an IP-literal (digits separated by hyphens)
//   - Start with a DHCP-generated prefix (dhcp-, host-, ip-, client-)
//   - Contain an ISP-generated noise domain (.dynamic., .dhcp., .broadband.)
//   - Consist entirely of numeric labels (e.g. reversed-IP PTR arpa names)
func DNSNameIsUsable(name string) bool {
	lower := strings.ToLower(strings.TrimSuffix(name, "."))

	// Raw IP address
	if net.ParseIP(lower) != nil {
		return false
	}

	// IP literal embedded in hostname (192-168-1-50, 10.0.0.1 etc.)
	if ipLiteralPattern.MatchString(lower) {
		return false
	}

	// DHCP-generated prefix on the leftmost label
	leftmost := lower
	if idx := strings.IndexByte(lower, '.'); idx >= 0 {
		leftmost = lower[:idx]
	}
	for _, pfx := range dhcpPrefixes {
		if strings.HasPrefix(leftmost, pfx) {
			return false
		}
	}

	// ISP/infrastructure domain fragment anywhere in the FQDN
	for _, sfx := range noiseSuffixes {
		if strings.Contains(lower, sfx) {
			return false
		}
	}

	// Entirely numeric labels (e.g. 1.0.168.192.in-addr.arpa — PTR arpa form)
	labels := strings.Split(lower, ".")
	allNumeric := true
	for _, label := range labels {
		for _, ch := range label {
			if ch < '0' || ch > '9' {
				allNumeric = false
				break
			}
		}
		if !allNumeric {
			break
		}
	}
	if allNumeric {
		return false
	}

	return true
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/enrichment/... -run TestDNSNameQuality -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/enrichment/dns_quality.go internal/enrichment/dns_quality_test.go
git commit -m "feat(enrichment): add DNS name quality filter"
```

---

## Task 6: mDNS Enricher

**Files:**
- Modify: `go.mod` / `go.sum` (add miekg/dns)
- Create: `internal/enrichment/mdns.go`
- Create: `internal/enrichment/mdns_test.go`

- [ ] **Step 1: Add miekg/dns dependency**

```bash
go get github.com/miekg/dns@latest
```

- [ ] **Step 2: Write the failing test**

```go
// internal/enrichment/mdns_test.go
package enrichment_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/enrichment"
)

func TestMDNSEnricher_Enrich_Success(t *testing.T) {
	// Start a fake UDP "mDNS" responder on a random port.
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)

	// Serve one PTR response.
	go func() {
		buf := make([]byte, 512)
		n, remote, _ := pc.ReadFrom(buf)
		req := new(dns.Msg)
		if parseErr := req.Unpack(buf[:n]); parseErr != nil {
			return
		}
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = append(resp.Answer, &dns.PTR{
			Hdr: dns.RR_Header{
				Name:   req.Question[0].Name,
				Rrtype: dns.TypePTR,
				Class:  dns.ClassINET,
				Ttl:    120,
			},
			Ptr: "mydevice.local.",
		})
		b, _ := resp.Pack()
		pc.WriteTo(b, remote)
	}()

	e := enrichment.NewMDNSEnricher(
		enrichment.WithMDNSTimeout(2*time.Second),
		enrichment.WithMDNSPort(addr.Port),
	)

	name, err := e.Enrich(context.Background(), "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(name, ".local"), "expected .local suffix, got %q", name)
}

func TestMDNSEnricher_Enrich_NoResponse(t *testing.T) {
	// Port with nothing listening — should time out cleanly.
	e := enrichment.NewMDNSEnricher(
		enrichment.WithMDNSTimeout(100*time.Millisecond),
		enrichment.WithMDNSPort(1), // port 1 is unreachable
	)
	name, err := e.Enrich(context.Background(), "127.0.0.1")
	assert.NoError(t, err)   // timeout is not an error
	assert.Empty(t, name)
}
```

- [ ] **Step 3: Run — expect compile failure**

```bash
go test ./internal/enrichment/... -run TestMDNSEnricher -v 2>&1 | head -20
```

Expected: `undefined: enrichment.NewMDNSEnricher`

- [ ] **Step 4: Write the enricher**

```go
// internal/enrichment/mdns.go
package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	mdnsDefaultTimeout = 2 * time.Second
	mdnsDefaultPort    = 5353
)

// MDNSEnricher queries a host directly (unicast) on its mDNS port via a
// DNS PTR query for the reversed-IP .in-addr.arpa name. Apple, Android, and
// Linux/avahi devices respond with a stable .local name.
type MDNSEnricher struct {
	timeout time.Duration
	port    int
	logger  *slog.Logger
}

// MDNSOption configures MDNSEnricher.
type MDNSOption func(*MDNSEnricher)

// WithMDNSTimeout sets the per-query timeout.
func WithMDNSTimeout(d time.Duration) MDNSOption {
	return func(e *MDNSEnricher) { e.timeout = d }
}

// WithMDNSPort overrides the default port (5353). Intended for testing only.
func WithMDNSPort(p int) MDNSOption {
	return func(e *MDNSEnricher) { e.port = p }
}

// NewMDNSEnricher creates a new MDNSEnricher with the given options.
func NewMDNSEnricher(opts ...MDNSOption) *MDNSEnricher {
	e := &MDNSEnricher{
		timeout: mdnsDefaultTimeout,
		port:    mdnsDefaultPort,
		logger:  slog.Default(),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Enrich sends a unicast DNS PTR query to <ip>:port and returns the resolved
// .local name. Returns ("", nil) when the host does not respond or has no PTR.
func (e *MDNSEnricher) Enrich(ctx context.Context, ip string) (string, error) {
	arpa, err := dns.ReverseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("mdns: reverse addr for %s: %w", ip, err)
	}

	msg := new(dns.Msg)
	msg.SetQuestion(arpa, dns.TypePTR)
	msg.RecursionDesired = false

	target := net.JoinHostPort(ip, fmt.Sprintf("%d", e.port))

	client := &dns.Client{
		Net:     "udp",
		Timeout: e.timeout,
	}

	// Respect the caller's context deadline if it's tighter than our timeout.
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < client.Timeout {
			client.Timeout = remaining
		}
	}

	resp, _, err := client.Exchange(msg, target)
	if err != nil {
		// Timeout / connection refused — not an error, host just doesn't respond.
		e.logger.Debug("mdns: no response", "ip", ip, "error", err)
		return "", nil
	}
	if resp == nil || len(resp.Answer) == 0 {
		return "", nil
	}

	for _, rr := range resp.Answer {
		if ptr, ok := rr.(*dns.PTR); ok {
			name := strings.TrimSuffix(ptr.Ptr, ".")
			e.logger.Debug("mdns: resolved", "ip", ip, "name", name)
			return name, nil
		}
	}
	return "", nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/enrichment/... -run TestMDNSEnricher -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/enrichment/mdns.go internal/enrichment/mdns_test.go
git commit -m "feat(enrichment): add MDNSEnricher with unicast PTR query"
```

---

## Task 7: DeviceMatcher Service

**Files:**
- Create: `internal/services/device_matcher.go`
- Create: `internal/services/device_matcher_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/services/device_matcher_test.go
package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

type mockMatcherRepo struct {
	devices      []db.DeviceSignals
	attached     map[uuid.UUID]uuid.UUID // hostID -> deviceID
	suggestions  []db.DeviceSuggestion
	upsertedMACs []string
}

func (m *mockMatcherRepo) AllDevicesWithSignals(_ context.Context) ([]db.DeviceSignals, error) {
	return m.devices, nil
}
func (m *mockMatcherRepo) AttachHost(_ context.Context, deviceID, hostID uuid.UUID) error {
	if m.attached == nil {
		m.attached = map[uuid.UUID]uuid.UUID{}
	}
	m.attached[hostID] = deviceID
	return nil
}
func (m *mockMatcherRepo) UpsertKnownMAC(_ context.Context, _ uuid.UUID, mac string) error {
	m.upsertedMACs = append(m.upsertedMACs, mac)
	return nil
}
func (m *mockMatcherRepo) UpsertKnownName(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *mockMatcherRepo) UpsertSuggestion(_ context.Context, _, _ uuid.UUID, _ int, _ string) error {
	return nil
}

func TestDeviceMatcher_AutoAttach_ByGlobalMAC(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{
				Device:    db.Device{ID: deviceID, Name: "My Router"},
				KnownMACs: []string{"aa:bb:cc:dd:ee:ff"}, // globally-administered
			},
		},
	}
	svc := services.NewDeviceMatcher(repo)

	host := &db.Host{
		ID:         hostID,
		MACAddress: ptr("aa:bb:cc:dd:ee:ff"),
	}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, deviceID, repo.attached[hostID], "host should be auto-attached")
}

func TestDeviceMatcher_Suggestion_OnLowScore(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()

	var suggested bool
	repo := &mockMatcherRepoWithSuggest{
		mockMatcherRepo: mockMatcherRepo{
			devices: []db.DeviceSignals{
				{
					Device:    db.Device{ID: deviceID, Name: "Unknown Device"},
					KnownMACs: []string{"02:ab:cd:ef:01:23"}, // locally-administered (randomized)
				},
			},
		},
		onUpsertSuggestion: func() { suggested = true },
	}
	svc := services.NewDeviceMatcher(repo)

	host := &db.Host{
		ID:         hostID,
		MACAddress: ptr("02:ab:cd:ef:01:23"),
	}

	require.NoError(t, svc.MatchHost(context.Background(), host))
	assert.True(t, suggested, "expected a suggestion for locally-administered MAC match")
	assert.Empty(t, repo.attached, "should not auto-attach on low score")
}

type mockMatcherRepoWithSuggest struct {
	mockMatcherRepo
	onUpsertSuggestion func()
}

func (m *mockMatcherRepoWithSuggest) UpsertSuggestion(_ context.Context, _, _ uuid.UUID, _ int, _ string) error {
	m.onUpsertSuggestion()
	return nil
}

func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/services/... -run TestDeviceMatcher -v 2>&1 | head -20
```

Expected: `undefined: services.DeviceMatcher`

- [ ] **Step 3: Write the matcher**

```go
// internal/services/device_matcher.go
package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Signal weights
	weightStableMAC   = 3
	weightMDNSName    = 3
	weightSNMPName    = 3
	weightNetBIOS     = 3
	weightUserName    = 3
	weightDNSName     = 2
	weightBanner      = 2
	weightRandomMAC   = 1
	weightHistoricalIP = 1
	weightOSFamily    = 1
	weightVendorOUI   = 1
	weightPortSet     = 1

	autoAttachThreshold = 3
)

// matcherRepository is the DB interface required by DeviceMatcher.
type matcherRepository interface {
	AllDevicesWithSignals(ctx context.Context) ([]db.DeviceSignals, error)
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	UpsertKnownMAC(ctx context.Context, deviceID uuid.UUID, mac string) error
	UpsertKnownName(ctx context.Context, deviceID uuid.UUID, name, source string) error
	UpsertSuggestion(ctx context.Context, hostID, deviceID uuid.UUID, score int, reason string) error
}

// DeviceMatcher scores existing devices against a host's signals and
// auto-attaches or creates suggestions based on confidence thresholds.
type DeviceMatcher struct {
	repo   matcherRepository
	logger *slog.Logger
}

// NewDeviceMatcher creates a DeviceMatcher.
func NewDeviceMatcher(repo matcherRepository) *DeviceMatcher {
	return &DeviceMatcher{repo: repo, logger: slog.Default()}
}

// deviceScore holds the running score and reason string for one candidate device.
type deviceScore struct {
	deviceID uuid.UUID
	score    int
	reasons  []string
}

func (s *deviceScore) add(weight int, reason string) {
	s.score += weight
	s.reasons = append(s.reasons, reason)
}

func (s *deviceScore) reason() string {
	return strings.Join(s.reasons, " · ")
}

// MatchHost scores all known devices against the host and auto-attaches or suggests.
func (m *DeviceMatcher) MatchHost(ctx context.Context, host *db.Host) error {
	devices, err := m.repo.AllDevicesWithSignals(ctx)
	if err != nil {
		return fmt.Errorf("device matcher: load devices: %w", err)
	}
	if len(devices) == 0 {
		return nil
	}

	scores := make([]deviceScore, 0, len(devices))
	for _, sig := range devices {
		sc := deviceScore{deviceID: sig.Device.ID}
		m.scoreMAC(host, sig, &sc)
		m.scoreNames(host, sig, &sc)
		m.scoreOS(host, sig, &sc)
		m.scoreVendor(host, sig, &sc)
		if sc.score > 0 {
			scores = append(scores, sc)
		}
	}
	if len(scores) == 0 {
		return nil
	}

	// Find the maximum score.
	maxScore := 0
	for _, sc := range scores {
		if sc.score > maxScore {
			maxScore = sc.score
		}
	}

	// Collect all candidates at maximum score.
	var top []deviceScore
	for _, sc := range scores {
		if sc.score == maxScore {
			top = append(top, sc)
		}
	}

	if maxScore >= autoAttachThreshold && len(top) == 1 {
		return m.autoAttach(ctx, host, top[0])
	}

	// Suggest all candidates with score ≥ 1.
	for _, sc := range scores {
		if sc.score >= 1 {
			if err := m.repo.UpsertSuggestion(ctx, host.ID, sc.deviceID, sc.score, sc.reason()); err != nil {
				m.logger.Warn("device matcher: upsert suggestion failed",
					"host_id", host.ID, "device_id", sc.deviceID, "error", err)
			}
		}
	}
	return nil
}

func (m *DeviceMatcher) autoAttach(ctx context.Context, host *db.Host, sc deviceScore) error {
	if err := m.repo.AttachHost(ctx, sc.deviceID, host.ID); err != nil {
		return fmt.Errorf("device matcher: attach host: %w", err)
	}
	m.logger.Info("device matcher: auto-attached",
		"host_id", host.ID, "device_id", sc.deviceID, "score", sc.score, "reason", sc.reason())

	// Learn signals: record this host's MAC and names.
	if host.MACAddress != nil && *host.MACAddress != "" {
		if err := m.repo.UpsertKnownMAC(ctx, sc.deviceID, *host.MACAddress); err != nil {
			m.logger.Warn("device matcher: upsert known mac failed", "error", err)
		}
	}
	if host.MDNSName != nil && *host.MDNSName != "" {
		if err := m.repo.UpsertKnownName(ctx, sc.deviceID, *host.MDNSName, "mdns"); err != nil {
			m.logger.Warn("device matcher: upsert known mdns name failed", "error", err)
		}
	}
	if host.Hostname != nil && *host.Hostname != "" {
		if err := m.repo.UpsertKnownName(ctx, sc.deviceID, *host.Hostname, "dns"); err != nil {
			m.logger.Warn("device matcher: upsert known dns name failed", "error", err)
		}
	}
	return nil
}

func (m *DeviceMatcher) scoreMAC(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	if host.MACAddress == nil || *host.MACAddress == "" {
		return
	}
	mac := strings.ToLower(*host.MACAddress)
	for _, known := range sig.KnownMACs {
		if strings.ToLower(known) == mac {
			if isGloballyAdministeredMAC(mac) {
				sc.add(weightStableMAC, "MAC:stable")
			} else {
				sc.add(weightRandomMAC, "MAC:random")
			}
			return
		}
	}
}

// isGloballyAdministeredMAC returns true when the MAC's U/L bit is 0
// (IEEE globally-administered, i.e. hardware-burned address).
// A locally-administered (potentially randomized) MAC has the U/L bit set.
func isGloballyAdministeredMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) == 0 {
		return false
	}
	return hw[0]&0x02 == 0
}

func (m *DeviceMatcher) scoreNames(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	for _, known := range sig.KnownNames {
		switch known.Source {
		case "mdns":
			if host.MDNSName != nil && strings.EqualFold(*host.MDNSName, known.Name) {
				sc.add(weightMDNSName, "mDNS:"+known.Name)
			}
		case "dns":
			if host.Hostname != nil && strings.EqualFold(*host.Hostname, known.Name) {
				sc.add(weightDNSName, "DNS:"+known.Name)
			}
		case "snmp":
			// SNMP sysName is not yet on host struct; skip for now.
		case "netbios":
			// NetBIOS name is not yet on host struct; skip for now.
		}
	}
}

func (m *DeviceMatcher) scoreOS(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	// OS family scoring deferred until device has os_family stored.
	// Placeholder: no-op until device_known_attributes table is added.
	_ = host
	_ = sig
}

func (m *DeviceMatcher) scoreVendor(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	// Vendor/OUI scoring deferred similarly.
	_ = host
	_ = sig
}

// MatchHosts runs MatchHost for each host in the slice. Errors are logged but not fatal.
func (m *DeviceMatcher) MatchHosts(ctx context.Context, hosts []*db.Host) {
	for _, h := range hosts {
		if ctx.Err() != nil {
			return
		}
		if err := m.MatchHost(ctx, h); err != nil {
			m.logger.Warn("device matcher: match failed", "host_id", h.ID, "error", err)
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/services/... -run TestDeviceMatcher -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/device_matcher.go internal/services/device_matcher_test.go
git commit -m "feat(services): add DeviceMatcher with weighted signal scoring"
```

---

## Task 8: DeviceService

**Files:**
- Create: `internal/services/device.go`
- Create: `internal/services/device_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/services/device_test.go
package services_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

type mockDeviceRepo struct {
	device    *db.Device
	createErr error
	getErr    error
}

func (m *mockDeviceRepo) CreateDevice(_ context.Context, input db.CreateDeviceInput) (*db.Device, error) {
	return m.device, m.createErr
}
func (m *mockDeviceRepo) GetDevice(_ context.Context, _ uuid.UUID) (*db.Device, error) {
	return m.device, m.getErr
}
func (m *mockDeviceRepo) GetDeviceDetail(_ context.Context, _ uuid.UUID) (*db.DeviceDetail, error) {
	return nil, nil
}
func (m *mockDeviceRepo) UpdateDevice(_ context.Context, _ uuid.UUID, _ db.UpdateDeviceInput) (*db.Device, error) {
	return m.device, nil
}
func (m *mockDeviceRepo) DeleteDevice(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) ListDevices(_ context.Context) ([]db.DeviceSummary, error) {
	return make([]db.DeviceSummary, 0), nil
}
func (m *mockDeviceRepo) AttachHost(_ context.Context, _, _ uuid.UUID) error    { return nil }
func (m *mockDeviceRepo) DetachHost(_ context.Context, _, _ uuid.UUID) error    { return nil }
func (m *mockDeviceRepo) AcceptSuggestion(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) DismissSuggestion(_ context.Context, _ uuid.UUID) error { return nil }

func TestDeviceService_CreateDevice_RequiresName(t *testing.T) {
	svc := services.NewDeviceService(&mockDeviceRepo{})
	_, err := svc.CreateDevice(context.Background(), db.CreateDeviceInput{Name: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestDeviceService_CreateDevice_Success(t *testing.T) {
	id := uuid.New()
	repo := &mockDeviceRepo{device: &db.Device{ID: id, Name: "Router"}}
	svc := services.NewDeviceService(repo)
	d, err := svc.CreateDevice(context.Background(), db.CreateDeviceInput{Name: "Router"})
	require.NoError(t, err)
	assert.Equal(t, id, d.ID)
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/services/... -run TestDeviceService -v 2>&1 | head -10
```

- [ ] **Step 3: Write the service**

```go
// internal/services/device.go
package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// deviceRepository defines data-access operations required by DeviceService.
type deviceRepository interface {
	ListDevices(ctx context.Context) ([]db.DeviceSummary, error)
	CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error)
	GetDevice(ctx context.Context, id uuid.UUID) (*db.Device, error)
	GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error)
	UpdateDevice(ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput) (*db.Device, error)
	DeleteDevice(ctx context.Context, id uuid.UUID) error
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error
	DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error
}

// DeviceService handles business logic for device management.
type DeviceService struct {
	repo   deviceRepository
	logger *slog.Logger
}

// NewDeviceService creates a new DeviceService.
func NewDeviceService(repo deviceRepository, opts ...func(*DeviceService)) *DeviceService {
	s := &DeviceService{repo: repo, logger: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ListDevices returns all devices with MAC and host counts.
func (s *DeviceService) ListDevices(ctx context.Context) ([]db.DeviceSummary, error) {
	return s.repo.ListDevices(ctx)
}

// CreateDevice validates input and creates a new device.
func (s *DeviceService) CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error) {
	if input.Name == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "device name is required")
	}
	return s.repo.CreateDevice(ctx, input)
}

// GetDevice returns a device by ID.
func (s *DeviceService) GetDevice(ctx context.Context, id uuid.UUID) (*db.Device, error) {
	return s.repo.GetDevice(ctx, id)
}

// GetDeviceDetail returns the full device view.
func (s *DeviceService) GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error) {
	return s.repo.GetDeviceDetail(ctx, id)
}

// UpdateDevice updates name and/or notes.
func (s *DeviceService) UpdateDevice(ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput) (*db.Device, error) {
	return s.repo.UpdateDevice(ctx, id, input)
}

// DeleteDevice removes a device; attached hosts become unidentified.
func (s *DeviceService) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDevice(ctx, id)
}

// AttachHost manually attaches a host to a device.
func (s *DeviceService) AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	return s.repo.AttachHost(ctx, deviceID, hostID)
}

// DetachHost removes a host from a device.
func (s *DeviceService) DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	return s.repo.DetachHost(ctx, deviceID, hostID)
}

// AcceptSuggestion attaches the host and removes the suggestion.
func (s *DeviceService) AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	return s.repo.AcceptSuggestion(ctx, suggestionID)
}

// DismissSuggestion marks a suggestion as dismissed.
func (s *DeviceService) DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	return s.repo.DismissSuggestion(ctx, suggestionID)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/services/... -run TestDeviceService -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/device.go internal/services/device_test.go
git commit -m "feat(services): add DeviceService CRUD and attach/detach"
```

---

## Task 9: DeviceHandler and Interface

**Files:**
- Modify: `internal/api/handlers/interfaces.go`
- Create: `internal/api/handlers/devices.go`
- Create: `internal/api/handlers/devices_test.go`
- Create: `internal/api/handlers/mocks/mock_device_servicer.go` (generated)

- [ ] **Step 1: Add DeviceServicer to interfaces.go**

In `internal/api/handlers/interfaces.go`, add after `GroupServicer`:

```go
// DeviceServicer is the service-level interface consumed by DeviceHandler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -destination mocks/mock_device_servicer.go -package mocks github.com/anstrom/scanorama/internal/api/handlers DeviceServicer
type DeviceServicer interface {
	ListDevices(ctx context.Context) ([]db.DeviceSummary, error)
	CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error)
	GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error)
	UpdateDevice(ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput) (*db.Device, error)
	DeleteDevice(ctx context.Context, id uuid.UUID) error
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error
	DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error
}
```

- [ ] **Step 2: Generate the mock**

```bash
go generate ./internal/api/handlers/...
```

Expected: creates `internal/api/handlers/mocks/mock_device_servicer.go`.

- [ ] **Step 3: Write the handler**

```go
// internal/api/handlers/devices.go
package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// DeviceHandler handles HTTP requests for device management.
type DeviceHandler struct {
	svc     DeviceServicer
	metrics *metrics.Metrics
}

// NewDeviceHandler creates a DeviceHandler.
func NewDeviceHandler(svc DeviceServicer, m *metrics.Metrics) *DeviceHandler {
	return &DeviceHandler{svc: svc, metrics: m}
}

// ListDevices handles GET /api/v1/devices.
func (h *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.ListDevices(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// CreateDevice handles POST /api/v1/devices.
func (h *DeviceHandler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	var input db.CreateDeviceInput
	if !parseJSON(w, r, &input) {
		return
	}
	device, err := h.svc.CreateDevice(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, device)
}

// GetDevice handles GET /api/v1/devices/{id}.
func (h *DeviceHandler) GetDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, mux.Vars(r)["id"])
	if !ok {
		return
	}
	detail, err := h.svc.GetDeviceDetail(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// UpdateDevice handles PUT /api/v1/devices/{id}.
func (h *DeviceHandler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, mux.Vars(r)["id"])
	if !ok {
		return
	}
	var input db.UpdateDeviceInput
	if !parseJSON(w, r, &input) {
		return
	}
	device, err := h.svc.UpdateDevice(r.Context(), id, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// DeleteDevice handles DELETE /api/v1/devices/{id}.
func (h *DeviceHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, mux.Vars(r)["id"])
	if !ok {
		return
	}
	if err := h.svc.DeleteDevice(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AttachHost handles POST /api/v1/devices/{id}/hosts/{host_id}.
func (h *DeviceHandler) AttachHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID, ok := parseUUID(w, vars["id"])
	if !ok {
		return
	}
	hostID, ok := parseUUID(w, vars["host_id"])
	if !ok {
		return
	}
	if err := h.svc.AttachHost(r.Context(), deviceID, hostID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DetachHost handles DELETE /api/v1/devices/{id}/hosts/{host_id}.
func (h *DeviceHandler) DetachHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID, ok := parseUUID(w, vars["id"])
	if !ok {
		return
	}
	hostID, ok := parseUUID(w, vars["host_id"])
	if !ok {
		return
	}
	if err := h.svc.DetachHost(r.Context(), deviceID, hostID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AcceptSuggestion handles POST /api/v1/devices/suggestions/{id}/accept.
func (h *DeviceHandler) AcceptSuggestion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, mux.Vars(r)["id"])
	if !ok {
		return
	}
	if err := h.svc.AcceptSuggestion(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DismissSuggestion handles POST /api/v1/devices/suggestions/{id}/dismiss.
func (h *DeviceHandler) DismissSuggestion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, mux.Vars(r)["id"])
	if !ok {
		return
	}
	if err := h.svc.DismissSuggestion(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Write handler tests**

```go
// internal/api/handlers/devices_test.go
package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers"
	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
)

func TestDeviceHandler_ListDevices_200(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockDeviceServicer(ctrl)
	svc.EXPECT().ListDevices(gomock.Any()).Return([]db.DeviceSummary{
		{ID: uuid.New(), Name: "Router", MACCount: 1, HostCount: 2},
	}, nil)

	h := handlers.NewDeviceHandler(svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	h.ListDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	devs, ok := raw["devices"].([]any)
	require.True(t, ok)
	assert.Len(t, devs, 1)
}

func TestDeviceHandler_CreateDevice_400_MissingName(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockDeviceServicer(ctrl)
	// CreateDevice returns validation error from service.
	svc.EXPECT().CreateDevice(gomock.Any(), gomock.Any()).Return(nil,
		errors.NewScanError(errors.CodeValidation, "device name is required"))

	h := handlers.NewDeviceHandler(svc, nil)
	body, _ := json.Marshal(db.CreateDeviceInput{Name: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateDevice(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeviceHandler_GetDevice_404(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockDeviceServicer(ctrl)
	id := uuid.New()
	svc.EXPECT().GetDeviceDetail(gomock.Any(), id).Return(nil,
		errors.NewScanError(errors.CodeNotFound, "device not found"))

	r := mux.NewRouter()
	h := handlers.NewDeviceHandler(svc, nil)
	r.HandleFunc("/api/v1/devices/{id}", h.GetDevice).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

Note: add `"github.com/anstrom/scanorama/internal/errors"` to imports in test file.

- [ ] **Step 5: Build and test**

```bash
go build ./internal/api/handlers/...
go test ./internal/api/handlers/... -run TestDeviceHandler -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/interfaces.go \
        internal/api/handlers/devices.go \
        internal/api/handlers/devices_test.go \
        internal/api/handlers/mocks/mock_device_servicer.go
git commit -m "feat(api): add DeviceHandler and DeviceServicer interface"
```

---

## Task 10: Register Routes

**Files:**
- Modify: `internal/api/routes.go`

- [ ] **Step 1: Wire the handler and register routes**

In `internal/api/routes.go`, in `setupRoutes()`, add after `groupHandler`:

```go
deviceHandler := apihandlers.NewDeviceHandler(
    services.NewDeviceService(db.NewDeviceRepository(s.database)), s.metrics)
```

Add a call at the end of `setupRoutes()`:

```go
s.setupDeviceRoutes(api, deviceHandler)
```

Add the new `setupDeviceRoutes` function:

```go
// setupDeviceRoutes registers device CRUD, attach/detach, and suggestion endpoints.
func (s *Server) setupDeviceRoutes(api *mux.Router, h *apihandlers.DeviceHandler) {
	// Fixed-path routes before /{id}.
	api.HandleFunc("/devices/suggestions/{id}/accept",  h.AcceptSuggestion).Methods("POST")
	api.HandleFunc("/devices/suggestions/{id}/dismiss", h.DismissSuggestion).Methods("POST")
	api.HandleFunc("/devices",                          h.ListDevices).Methods("GET")
	api.HandleFunc("/devices",                          h.CreateDevice).Methods("POST")
	api.HandleFunc("/devices/{id}",                     h.GetDevice).Methods("GET")
	api.HandleFunc("/devices/{id}",                     h.UpdateDevice).Methods("PUT")
	api.HandleFunc("/devices/{id}",                     h.DeleteDevice).Methods("DELETE")
	api.HandleFunc("/devices/{id}/hosts/{host_id}",     h.AttachHost).Methods("POST")
	api.HandleFunc("/devices/{id}/hosts/{host_id}",     h.DetachHost).Methods("DELETE")
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/api/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/api/routes.go
git commit -m "feat(api): register device routes"
```

---

## Task 11: Swagger Docs

- [ ] **Step 1: Add swagger annotations to devices.go**

Above each handler in `internal/api/handlers/devices.go`, add minimal swagger comment blocks. Example for `ListDevices`:

```go
// ListDevices godoc
//
//	@Summary     List devices
//	@Tags        devices
//	@Produce     json
//	@Success     200  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices [get]
func (h *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
```

Add similar annotations for all other handlers.

- [ ] **Step 2: Regenerate swagger**

```bash
make docs
```

- [ ] **Step 3: Confirm no drift**

```bash
git diff --exit-code docs/swagger/ frontend/src/api/types.ts
```

Expected: exit 0 (or only device-related additions).

- [ ] **Step 4: Commit**

```bash
git add docs/swagger/ frontend/src/api/types.ts
git commit -m "chore(docs): regenerate swagger for device endpoints"
```

---

## Task 12: Frontend Hooks

**Files:**
- Create: `frontend/src/api/hooks/use-devices.ts`

- [ ] **Step 1: Write the hooks**

```typescript
// frontend/src/api/hooks/use-devices.ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

export function useDevices() {
  return useQuery({
    queryKey: ["devices"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/devices");
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    staleTime: 30_000,
  });
}

export function useDevice(id: string) {
  return useQuery({
    queryKey: ["devices", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/devices/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!id,
    staleTime: 30_000,
  });
}

export function useCreateDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { name: string; notes?: string }) => {
      const { data, error, response } = await api.POST("/devices", { body: input });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["devices"] }),
  });
}

export function useUpdateDevice(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { name?: string; notes?: string }) => {
      const { data, error, response } = await api.PUT("/devices/{id}", {
        params: { path: { id } },
        body: input,
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices"] });
      qc.invalidateQueries({ queryKey: ["devices", id] });
    },
  });
}

export function useDeleteDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/devices/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["devices"] }),
  });
}

export function useAttachHost(deviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (hostId: string) => {
      const { error, response } = await api.POST("/devices/{id}/hosts/{host_id}", {
        params: { path: { id: deviceId, host_id: hostId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["devices", deviceId] }),
  });
}

export function useDetachHost(deviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (hostId: string) => {
      const { error, response } = await api.DELETE("/devices/{id}/hosts/{host_id}", {
        params: { path: { id: deviceId, host_id: hostId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["devices", deviceId] }),
  });
}

export function useAcceptSuggestion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (suggestionId: string) => {
      const { error, response } = await api.POST("/devices/suggestions/{id}/accept", {
        params: { path: { id: suggestionId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices"] });
      qc.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}

export function useDismissSuggestion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (suggestionId: string) => {
      const { error, response } = await api.POST("/devices/suggestions/{id}/dismiss", {
        params: { path: { id: suggestionId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["discovery"] }),
  });
}
```

- [ ] **Step 2: Write hook tests**

```typescript
// frontend/src/api/hooks/use-devices.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../../test/utils";
import { useDevices, useCreateDevice } from "./use-devices";
import * as apiClient from "../client";

vi.mock("../client");

const ok = (data: unknown) =>
  Promise.resolve({ data, error: undefined, response: new Response() });
const fail = (msg = "error") =>
  Promise.resolve({ data: undefined, error: { message: msg }, response: new Response() });

beforeEach(() => vi.resetAllMocks());

describe("useDevices", () => {
  it("returns device list on success", async () => {
    vi.mocked(apiClient.api.GET).mockResolvedValue(
      ok({ devices: [{ id: "abc", name: "Router", mac_count: 1, host_count: 2 }] }) as any
    );
    const { result, waitForNextUpdate } = renderHookWithQuery(() => useDevices());
    await waitForNextUpdate();
    expect(result.current.data?.devices).toHaveLength(1);
  });

  it("sets error on failure", async () => {
    vi.mocked(apiClient.api.GET).mockResolvedValue(fail("server error") as any);
    const { result, waitForNextUpdate } = renderHookWithQuery(() => useDevices());
    await waitForNextUpdate();
    expect(result.current.error).toBeTruthy();
  });
});

describe("useCreateDevice", () => {
  it("invalidates devices query on success", async () => {
    vi.mocked(apiClient.api.POST).mockResolvedValue(
      ok({ id: "new-id", name: "Switch" }) as any
    );
    const { result } = renderHookWithQuery(() => useCreateDevice());
    await result.current.mutateAsync({ name: "Switch" });
    expect(apiClient.api.POST).toHaveBeenCalledWith("/devices", expect.any(Object));
  });
});
```

- [ ] **Step 3: Run tests**

```bash
cd frontend && npm test -- use-devices --run
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/hooks/use-devices.ts frontend/src/api/hooks/use-devices.test.ts
git commit -m "feat(frontend): add device React Query hooks"
```

---

## Task 13: Device Detail Page

**Files:**
- Create: `frontend/src/routes/devices.tsx`

- [ ] **Step 1: Write the page**

```tsx
// frontend/src/routes/devices.tsx
import { useParams, Link } from "react-router-dom";
import { useDevice, useDetachHost } from "../api/hooks/use-devices";

export default function DeviceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: device, isLoading, error } = useDevice(id ?? "");
  const detach = useDetachHost(id ?? "");

  if (isLoading) return <div className="animate-pulse h-8 bg-gray-200 rounded" />;
  if (error || !device) return <p className="text-red-500">Device not found.</p>;

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">{device.name}</h1>
      {device.notes && <p className="text-gray-600">{device.notes}</p>}

      <section>
        <h2 className="text-lg font-semibold mb-2">Known MACs</h2>
        {device.known_macs.length === 0 ? (
          <p className="text-gray-400">No MACs recorded.</p>
        ) : (
          <ul className="space-y-1">
            {device.known_macs.map((m: any) => (
              <li key={m.id} className="font-mono text-sm">{m.mac_address}</li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h2 className="text-lg font-semibold mb-2">Known Names</h2>
        {device.known_names.length === 0 ? (
          <p className="text-gray-400">No names recorded.</p>
        ) : (
          <ul className="space-y-1">
            {device.known_names.map((n: any) => (
              <li key={n.id} className="flex items-center gap-2">
                <span className="text-xs bg-blue-100 text-blue-700 px-1 rounded">{n.source}</span>
                <span>{n.name}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h2 className="text-lg font-semibold mb-2">Attached Hosts</h2>
        {device.hosts.length === 0 ? (
          <p className="text-gray-400">No hosts attached.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left border-b">
                <th className="py-1">IP</th>
                <th>MAC</th>
                <th>Hostname</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {device.hosts.map((h: any) => (
                <tr key={h.id} className="border-b hover:bg-gray-50">
                  <td className="py-1">
                    <Link to={`/hosts/${h.id}`} className="text-blue-600 hover:underline font-mono">
                      {h.ip_address}
                    </Link>
                  </td>
                  <td className="font-mono">{h.mac_address ?? "—"}</td>
                  <td>{h.hostname ?? "—"}</td>
                  <td>
                    <button
                      onClick={() => detach.mutate(h.id)}
                      className="text-xs text-red-500 hover:underline"
                    >
                      Detach
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Register the route**

In the frontend router file (typically `frontend/src/App.tsx` or `frontend/src/router.tsx`), add:

```tsx
import DeviceDetailPage from "./routes/devices";
// Inside the routes:
<Route path="/devices/:id" element={<DeviceDetailPage />} />
```

- [ ] **Step 3: Build**

```bash
cd frontend && npm run build 2>&1 | tail -20
```

Expected: no TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/routes/devices.tsx frontend/src/App.tsx   # or router file
git commit -m "feat(frontend): add device detail page"
```

---

## Task 14: Host Detail Device Card

**Files:**
- Modify: `frontend/src/routes/host-detail.tsx` (or wherever host detail is rendered)

- [ ] **Step 1: Find the host detail component**

```bash
grep -r "device_id\|DeviceCard\|GetHost" frontend/src --include="*.tsx" -l
```

- [ ] **Step 2: Add device card**

In the host detail page, add a Device section after the existing cards:

```tsx
{/* Device card */}
<section className="border rounded p-4">
  <h3 className="font-semibold mb-2">Device</h3>
  {host.device_id ? (
    <div className="flex items-center justify-between">
      <Link to={`/devices/${host.device_id}`} className="text-blue-600 hover:underline">
        {host.device_name ?? host.device_id}
      </Link>
      <button
        onClick={() => detachFromHost.mutate({ deviceId: host.device_id!, hostId: host.id })}
        className="text-xs text-red-500 hover:underline"
      >
        Detach
      </button>
    </div>
  ) : (
    <p className="text-gray-400 text-sm">No device assigned.</p>
  )}
</section>
```

- [ ] **Step 3: Build**

```bash
cd frontend && npm run build 2>&1 | tail -10
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/routes/host-detail.tsx
git commit -m "feat(frontend): add Device card to host detail"
```

---

## Task 15: Full Backend Test Run and Push

- [ ] **Step 1: Run full backend suite**

```bash
go test -race ./internal/... 2>&1 | tail -30
```

Expected: all PASS, no race conditions.

- [ ] **Step 2: Run frontend tests**

```bash
cd frontend && npm test -- --run
```

Expected: all PASS.

- [ ] **Step 3: Check swagger drift**

```bash
make docs
git diff --exit-code docs/swagger/ frontend/src/api/types.ts
```

- [ ] **Step 4: Push**

```bash
git push origin feat/device-identity
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by task |
|-----------------|-----------------|
| `devices`, `device_known_macs`, `device_known_names`, `device_suggestions` tables | Task 1 |
| `hosts.device_id` and `hosts.mdns_name` columns | Task 1 |
| mDNS enricher (unicast PTR, miekg/dns, 2s timeout) | Task 6 |
| DNS name quality filter | Task 5 |
| DeviceMatcher with weighted scoring | Task 7 |
| Auto-attach on score ≥ 3, suggest on 1–2, tie → suggest both | Task 7 |
| Signal learning on auto-attach | Task 7 |
| Device CRUD API | Tasks 8–10 |
| Manual attach/detach API | Tasks 8–10 |
| Suggestion accept/dismiss API | Tasks 8–10 |
| Host list/detail gains device_id, device_name, mdns_name | Task 4 |
| Discovery diff gains suggestions array | Not yet covered — **see below** |
| Device detail page `/devices/:id` | Task 13 |
| Host detail Device card | Task 14 |
| Discovery diff suggestion cards | Not yet covered |

**Gap: Discovery diff suggestions**

The spec requires `GET /discovery/{id}/diff` to return a `suggestions` array. This needs:
1. `DiscoveryRepository.GetDiscoveryDiff` to also query `device_suggestions` for the hosts in the diff and attach them
2. `DiscoveryDiff` struct to gain a `Suggestions []DeviceSuggestion` field
3. Frontend discovery diff page to render suggestion cards with Accept/Dismiss buttons

This is scoped to Task 16 below.

---

## Task 16: Discovery Diff Suggestions

**Files:**
- Modify: `internal/db/models.go` (`DiscoveryDiff` struct)
- Modify: `internal/db/repository_discovery.go` (`GetDiscoveryDiff`)
- Modify: relevant frontend discovery diff component

- [ ] **Step 1: Add Suggestions to DiscoveryDiff**

In `internal/db/models.go`, find `DiscoveryDiff` and add:

```go
Suggestions []DeviceSuggestion `json:"suggestions"`
```

- [ ] **Step 2: Populate suggestions in GetDiscoveryDiff**

At the end of `GetDiscoveryDiff` in `repository_discovery.go`, after the diff is assembled, collect all host IDs from new/changed hosts and query suggestions:

```go
// Collect host IDs from new and changed hosts.
var hostIDs []uuid.UUID
for _, h := range diff.NewHosts {
    hostIDs = append(hostIDs, h.ID)
}
for _, h := range diff.ChangedHosts {
    hostIDs = append(hostIDs, h.Current.ID)
}

if len(hostIDs) > 0 {
    devRepo := NewDeviceRepository(r.db)
    suggs, err := devRepo.GetSuggestionsForDiscovery(ctx, hostIDs)
    if err != nil {
        // Non-fatal: log and return diff without suggestions.
        slog.Warn("discovery diff: failed to load suggestions", "error", err)
    } else {
        diff.Suggestions = suggs
    }
}
if diff.Suggestions == nil {
    diff.Suggestions = make([]DeviceSuggestion, 0)
}
```

- [ ] **Step 3: Build**

```bash
go build ./internal/...
```

- [ ] **Step 4: Find the frontend discovery diff component**

```bash
grep -r "GetDiscoveryDiff\|discovery.*diff\|DiscoveryDiff" frontend/src --include="*.tsx" -l
```

- [ ] **Step 5: Add suggestion cards to frontend discovery diff**

In the discovery diff page, below the new/gone/changed sections, render:

```tsx
{diff.suggestions?.length > 0 && (
  <section>
    <h3 className="font-semibold mt-6 mb-2">Device Suggestions</h3>
    <div className="space-y-2">
      {diff.suggestions.map((s: any) => (
        <div key={s.id} className="border rounded p-3 flex items-center justify-between">
          <div>
            <span className="font-mono text-sm">{s.confidence_reason}</span>
            <span className="ml-2 text-xs text-gray-500">Score: {s.confidence_score}</span>
          </div>
          <div className="flex gap-2">
            <button
              onClick={() => accept.mutate(s.id)}
              className="text-xs bg-green-500 text-white px-2 py-1 rounded"
            >
              Accept
            </button>
            <button
              onClick={() => dismiss.mutate(s.id)}
              className="text-xs bg-gray-200 px-2 py-1 rounded"
            >
              Dismiss
            </button>
          </div>
        </div>
      ))}
    </div>
  </section>
)}
```

Import `useAcceptSuggestion` and `useDismissSuggestion` from `../api/hooks/use-devices`.

- [ ] **Step 6: Build and test**

```bash
go test -race ./internal/db/... -run TestDiscovery
cd frontend && npm run build
```

- [ ] **Step 7: Commit**

```bash
git add internal/db/models.go internal/db/repository_discovery.go
git add frontend/src/  # discovery diff component
git commit -m "feat(api): include device suggestions in discovery diff response"
```
