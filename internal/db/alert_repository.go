package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// AlertRepository handles all alert-rule-related DB operations.
type AlertRepository struct {
	db *DB
}

// NewAlertRepository creates a new AlertRepository.
func NewAlertRepository(db *DB) *AlertRepository {
	return &AlertRepository{db: db}
}

const alertRuleSelectCols = `id, host_id, group_id, tag, trigger, channel_type, channel_url,
	enabled, created_at, updated_at`

// scanAlertRule scans one row into an AlertRule.
func scanAlertRule(row interface{ Scan(...interface{}) error }) (*AlertRule, error) {
	r := &AlertRule{}
	err := row.Scan(
		&r.ID, &r.HostID, &r.GroupID, &r.Tag, &r.Trigger,
		&r.ChannelType, &r.ChannelURL, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan alert rule: %w", err)
	}
	return r, nil
}

// ListAlertRules returns all alert rules ordered by creation time.
func (r *AlertRepository) ListAlertRules(ctx context.Context) ([]*AlertRule, error) {
	q := `SELECT ` + alertRuleSelectCols + `
	      FROM alert_rules ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, sanitizeDBError("list alert rules", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*AlertRule, 0)
	for rows.Next() {
		ar, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, ar)
	}
	return result, rows.Err()
}

// ListAlertRulesForHost returns enabled alert rules that target a specific host directly.
func (r *AlertRepository) ListAlertRulesForHost(
	ctx context.Context, hostID uuid.UUID,
) ([]*AlertRule, error) {
	q := `SELECT ` + alertRuleSelectCols + `
	      FROM alert_rules
	      WHERE host_id = $1
	      ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, hostID)
	if err != nil {
		return nil, sanitizeDBError("list alert rules for host", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*AlertRule, 0)
	for rows.Next() {
		ar, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, ar)
	}
	return result, rows.Err()
}

// CreateAlertRule inserts a new alert rule.
func (r *AlertRepository) CreateAlertRule(
	ctx context.Context, input CreateAlertRuleInput,
) (*AlertRule, error) {
	q := `INSERT INTO alert_rules (host_id, group_id, tag, trigger, channel_url)
	      VALUES ($1, $2, $3, $4, $5)
	      RETURNING ` + alertRuleSelectCols

	row := r.db.QueryRowContext(ctx, q,
		input.HostID, input.GroupID, input.Tag, input.Trigger, input.ChannelURL,
	)
	ar, err := scanAlertRule(row)
	if err != nil {
		return nil, sanitizeDBError("create alert rule", err)
	}
	return ar, nil
}

// GetAlertRule retrieves a single alert rule by ID.
func (r *AlertRepository) GetAlertRule(ctx context.Context, id uuid.UUID) (*AlertRule, error) {
	q := `SELECT ` + alertRuleSelectCols + `
	      FROM alert_rules WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)
	ar, err := scanAlertRule(row)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "alert rule not found")
	}
	if err != nil {
		return nil, sanitizeDBError("get alert rule", err)
	}
	return ar, nil
}

// UpdateAlertRule applies partial updates to an alert rule.
func (r *AlertRepository) UpdateAlertRule(
	ctx context.Context, id uuid.UUID, input UpdateAlertRuleInput,
) (*AlertRule, error) {
	q := `UPDATE alert_rules SET
	          trigger     = COALESCE($2, trigger),
	          channel_url = COALESCE($3, channel_url),
	          enabled     = COALESCE($4, enabled),
	          updated_at  = NOW()
	      WHERE id = $1
	      RETURNING ` + alertRuleSelectCols

	row := r.db.QueryRowContext(ctx, q, id, input.Trigger, input.ChannelURL, input.Enabled)
	ar, err := scanAlertRule(row)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "alert rule not found")
	}
	if err != nil {
		return nil, sanitizeDBError("update alert rule", err)
	}
	return ar, nil
}

// DeleteAlertRule removes an alert rule by ID.
func (r *AlertRepository) DeleteAlertRule(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return sanitizeDBError("delete alert rule", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "alert rule not found")
	}
	return nil
}

// GetStatusTransitionsSince returns host status transitions since the given time,
// enriched with the host's current tags for alert rule matching.
func (r *AlertRepository) GetStatusTransitionsSince(
	ctx context.Context, since time.Time,
) ([]StatusTransitionRow, error) {
	q := `SELECT se.host_id, se.from_status, se.to_status, se.changed_at, h.tags
	      FROM host_status_events se
	      JOIN hosts h ON h.id = se.host_id
	      WHERE se.changed_at >= $1
	      ORDER BY se.changed_at DESC`

	rows, err := r.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, sanitizeDBError("get status transitions since", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]StatusTransitionRow, 0)
	for rows.Next() {
		var row StatusTransitionRow
		var tags pq.StringArray
		if err := rows.Scan(
			&row.HostID, &row.FromStatus, &row.ToStatus, &row.ChangedAt, &tags,
		); err != nil {
			return nil, fmt.Errorf("scan status transition row: %w", err)
		}
		row.Tags = []string(tags)
		result = append(result, row)
	}
	return result, rows.Err()
}
