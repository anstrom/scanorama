package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

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

// ListDevices returns a summary list with per-device MAC and host counts.
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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Printf("repository_device: close rows: %v\n", closeErr)
		}
	}()

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

// UpdateDevice updates name and/or notes (nil fields are left unchanged).
func (r *DeviceRepository) UpdateDevice(ctx context.Context, id uuid.UUID, input UpdateDeviceInput) (*Device, error) {
	q := `
		UPDATE devices SET
		    name       = COALESCE($2, name),
		    notes      = COALESCE($3, notes),
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

// GetDeviceDetail returns the full device view (device + known MACs + known names + attached hosts).
func (r *DeviceRepository) GetDeviceDetail(ctx context.Context, id uuid.UUID) (*DeviceDetail, error) {
	dev, err := r.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &DeviceDetail{Device: *dev}

	macs, err := r.listKnownMACs(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.KnownMACs = macs

	names, err := r.listKnownNames(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.KnownNames = names

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

// listAttachedHosts returns all hosts with device_id = deviceID.
func (r *DeviceRepository) listAttachedHosts(ctx context.Context, deviceID uuid.UUID) ([]AttachedHostSummary, error) {
	q := `SELECT id, ip_address, mac_address, hostname, status, os_family,
	             vendor, first_seen, last_seen
	      FROM hosts WHERE device_id = $1 ORDER BY ip_address`
	rows, err := r.db.QueryContext(ctx, q, deviceID)
	if err != nil {
		return nil, sanitizeDBError("list attached hosts", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]AttachedHostSummary, 0)
	for rows.Next() {
		var h AttachedHostSummary
		if err := rows.Scan(
			&h.ID, &h.IPAddress, &h.MACAddress, &h.Hostname, &h.Status,
			&h.OSFamily, &h.Vendor, &h.FirstSeen, &h.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("scan attached host: %w", err)
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// AttachHost sets hosts.device_id = deviceID for the given host.
func (r *DeviceRepository) AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET device_id = $1 WHERE id = $2`, deviceID, hostID)
	if err != nil {
		return sanitizeDBError("attach host", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "host not found")
	}
	return nil
}

// DetachHost sets hosts.device_id = NULL for a host currently attached to deviceID.
func (r *DeviceRepository) DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET device_id = NULL WHERE id = $1 AND device_id = $2`,
		hostID, deviceID)
	if err != nil {
		return sanitizeDBError("detach host", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "host not found or not attached to this device")
	}
	return nil
}

// UpsertKnownMAC inserts or touches a known MAC record for a device.
func (r *DeviceRepository) UpsertKnownMAC(ctx context.Context, deviceID uuid.UUID, mac string) error {
	q := `
		INSERT INTO device_known_macs (device_id, mac_address, first_seen, last_seen)
		VALUES ($1, $2::macaddr, NOW(), NOW())
		ON CONFLICT (mac_address) DO UPDATE
		    SET last_seen = NOW()`
	_, err := r.db.ExecContext(ctx, q, deviceID, mac)
	return sanitizeDBError("upsert known mac", err)
}

// UpsertKnownName inserts or touches a known name record for a device.
func (r *DeviceRepository) UpsertKnownName(ctx context.Context, deviceID uuid.UUID, name, source string) error {
	q := `
		INSERT INTO device_known_names (device_id, name, source, first_seen, last_seen)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (name, source) DO UPDATE
		    SET last_seen = NOW()`
	_, err := r.db.ExecContext(ctx, q, deviceID, name, source)
	return sanitizeDBError("upsert known name", err)
}

// UpsertSuggestion inserts or refreshes a device suggestion (resets dismissed = FALSE).
func (r *DeviceRepository) UpsertSuggestion(
	ctx context.Context, hostID, deviceID uuid.UUID, score int, reason string,
) error {
	q := `
		INSERT INTO device_suggestions (host_id, device_id, confidence_score, confidence_reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_id, device_id) DO UPDATE
		    SET confidence_score  = $3,
		        confidence_reason = $4,
		        dismissed         = FALSE`
	_, err := r.db.ExecContext(ctx, q, hostID, deviceID, score, reason)
	return sanitizeDBError("upsert suggestion", err)
}

// AcceptSuggestion attaches the host to the device and deletes the suggestion.
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
// Filters on dismissed = FALSE so an already-dismissed suggestion returns CodeNotFound,
// consistent with AcceptSuggestion behavior.
func (r *DeviceRepository) DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_suggestions SET dismissed = TRUE WHERE id = $1 AND dismissed = FALSE`, suggestionID)
	if err != nil {
		return sanitizeDBError("dismiss suggestion", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "suggestion not found")
	}
	return nil
}

// FindDeviceByMAC returns the device that owns the given MAC address, or nil when none match.
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

// FindDevicesByName returns all devices that have the given (name, source) pair.
func (r *DeviceRepository) FindDevicesByName(ctx context.Context, name, source string) ([]*Device, error) {
	q := `SELECT d.id, d.name, d.notes, d.created_at, d.updated_at
	      FROM devices d
	      JOIN device_known_names n ON n.device_id = d.id
	      WHERE n.name = $1 AND n.source = $2`
	rows, err := r.db.QueryContext(ctx, q, name, source)
	if err != nil {
		return nil, sanitizeDBError("find devices by name", err)
	}
	defer func() { _ = rows.Close() }()

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

// AllDevicesWithSignals returns every device with its known MACs and names.
// Used by DeviceMatcher to avoid N+1 lookups.
//
// Two separate queries are used — one for MACs and one for names — to avoid a
// cartesian-product multiplication when a device has M MACs and N names.
func (r *DeviceRepository) AllDevicesWithSignals(ctx context.Context) ([]DeviceSignals, error) {
	// ── pass 1: devices + MACs ────────────────────────────────────────────
	macRows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.name, d.notes, d.created_at, d.updated_at,
		       m.mac_address
		FROM devices d
		LEFT JOIN device_known_macs m ON m.device_id = d.id
		ORDER BY d.id`)
	if err != nil {
		return nil, sanitizeDBError("all devices with signals (macs)", err)
	}
	defer func() { _ = macRows.Close() }()

	byID := map[uuid.UUID]*DeviceSignals{}
	order := make([]uuid.UUID, 0)

	for macRows.Next() {
		var (
			d   Device
			mac *string
		)
		if err := macRows.Scan(&d.ID, &d.Name, &d.Notes, &d.CreatedAt, &d.UpdatedAt, &mac); err != nil {
			return nil, fmt.Errorf("scan device signals (mac): %w", err)
		}
		if _, ok := byID[d.ID]; !ok {
			byID[d.ID] = &DeviceSignals{
				Device:     d,
				KnownMACs:  make([]string, 0),
				KnownNames: make([]DeviceKnownNameSignal, 0),
			}
			order = append(order, d.ID)
		}
		if mac != nil {
			byID[d.ID].KnownMACs = append(byID[d.ID].KnownMACs, *mac)
		}
	}
	if err := macRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device signals (mac): %w", err)
	}

	if len(byID) == 0 {
		return make([]DeviceSignals, 0), nil
	}

	// ── pass 2: names ─────────────────────────────────────────────────────
	nameRows, err := r.db.QueryContext(ctx, `
		SELECT device_id, name, source
		FROM device_known_names
		ORDER BY device_id`)
	if err != nil {
		return nil, sanitizeDBError("all devices with signals (names)", err)
	}
	defer func() { _ = nameRows.Close() }()

	for nameRows.Next() {
		var (
			deviceID uuid.UUID
			name     string
			source   string
		)
		if err := nameRows.Scan(&deviceID, &name, &source); err != nil {
			return nil, fmt.Errorf("scan device signals (name): %w", err)
		}
		if sig, ok := byID[deviceID]; ok {
			sig.KnownNames = append(sig.KnownNames, DeviceKnownNameSignal{Name: name, Source: source})
		}
	}
	if err := nameRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device signals (name): %w", err)
	}

	result := make([]DeviceSignals, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

// GetSuggestionsForDiscovery returns non-dismissed suggestions for a set of host IDs.
func (r *DeviceRepository) GetSuggestionsForDiscovery(
	ctx context.Context, hostIDs []uuid.UUID,
) ([]DeviceSuggestion, error) {
	if len(hostIDs) == 0 {
		return make([]DeviceSuggestion, 0), nil
	}

	strs := make([]string, len(hostIDs))
	for i, id := range hostIDs {
		strs[i] = id.String()
	}

	q := `
		SELECT id, host_id, device_id, confidence_score, confidence_reason,
		       dismissed, created_at
		FROM device_suggestions
		WHERE host_id = ANY($1::uuid[]) AND dismissed = FALSE`

	rows, err := r.db.QueryContext(ctx, q, pq.Array(strs))
	if err != nil {
		return nil, sanitizeDBError("get suggestions for discovery", err)
	}
	defer func() { _ = rows.Close() }()

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

// UpdateMDNSName writes the most-recently resolved mDNS name to hosts.mdns_name.
func (r *DeviceRepository) UpdateMDNSName(ctx context.Context, hostID uuid.UUID, name string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET mdns_name = $2 WHERE id = $1`, hostID, name)
	return sanitizeDBError("update mdns name", err)
}
