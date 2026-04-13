// Package db provides typed repository for SNMP credential operations.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	appcrypto "github.com/anstrom/scanorama/internal/crypto"
	"github.com/anstrom/scanorama/internal/errors"
)

// SNMPVersion identifies the SNMP protocol version for a credential set.
type SNMPVersion string

const (
	SNMPVersionV2c SNMPVersion = "v2c"
	SNMPVersionV3  SNMPVersion = "v3"

	// redactedSecret is the placeholder used in API responses for secret fields.
	redactedSecret = "***"
)

// SNMPCredential holds decrypted SNMP credentials for a scope.
// NetworkID nil means global default.
type SNMPCredential struct {
	ID        uuid.UUID   `db:"id"         json:"id"`
	NetworkID *uuid.UUID  `db:"network_id" json:"network_id,omitempty"`
	Version   SNMPVersion `db:"version"    json:"version"`
	// v2c
	Community string `json:"community,omitempty"` // plain text in memory, encrypted in DB
	// v3
	Username  string `json:"username,omitempty"`
	AuthProto string `json:"auth_proto,omitempty"`
	AuthPass  string `json:"auth_pass,omitempty"` // plain text in memory, encrypted in DB
	PrivProto string `json:"priv_proto,omitempty"`
	PrivPass  string `json:"priv_pass,omitempty"` // plain text in memory, encrypted in DB
	// Metadata
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// SNMPCredentialSafe is like SNMPCredential but with secrets redacted for API responses.
type SNMPCredentialSafe struct {
	ID        uuid.UUID   `json:"id"`
	NetworkID *uuid.UUID  `json:"network_id,omitempty"`
	Version   SNMPVersion `json:"version"`
	Community string      `json:"community,omitempty"` // redactedSecret if set
	Username  string      `json:"username,omitempty"`
	AuthProto string      `json:"auth_proto,omitempty"`
	AuthPass  string      `json:"auth_pass,omitempty"` // redactedSecret if set
	PrivProto string      `json:"priv_proto,omitempty"`
	PrivPass  string      `json:"priv_pass,omitempty"` // redactedSecret if set
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// Redact returns a copy with secret fields replaced by redactedSecret.
func (c *SNMPCredential) Redact() SNMPCredentialSafe {
	safe := SNMPCredentialSafe{
		ID:        c.ID,
		NetworkID: c.NetworkID,
		Version:   c.Version,
		Username:  c.Username,
		AuthProto: c.AuthProto,
		PrivProto: c.PrivProto,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if c.Community != "" {
		safe.Community = redactedSecret
	}
	if c.AuthPass != "" {
		safe.AuthPass = redactedSecret
	}
	if c.PrivPass != "" {
		safe.PrivPass = redactedSecret
	}
	return safe
}

// SNMPCredentialsRepository handles SNMP credential persistence.
type SNMPCredentialsRepository struct {
	db *DB
}

// NewSNMPCredentialsRepository creates a new SNMPCredentialsRepository.
func NewSNMPCredentialsRepository(db *DB) *SNMPCredentialsRepository {
	return &SNMPCredentialsRepository{db: db}
}

// row holds raw encrypted DB values between scan and decrypt.
type snmpCredRow struct {
	ID        uuid.UUID   `db:"id"`
	NetworkID *uuid.UUID  `db:"network_id"`
	Version   SNMPVersion `db:"version"`
	Community *string     `db:"community"`
	Username  *string     `db:"username"`
	AuthProto *string     `db:"auth_proto"`
	AuthPass  *string     `db:"auth_pass"`
	PrivProto *string     `db:"priv_proto"`
	PrivPass  *string     `db:"priv_pass"`
	CreatedAt time.Time   `db:"created_at"`
	UpdatedAt time.Time   `db:"updated_at"`
}

func (row *snmpCredRow) decrypt() (*SNMPCredential, error) {
	c := &SNMPCredential{
		ID:        row.ID,
		NetworkID: row.NetworkID,
		Version:   row.Version,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	var err error
	if row.Community != nil {
		if c.Community, err = appcrypto.Decrypt(*row.Community); err != nil {
			return nil, fmt.Errorf("decrypt community: %w", err)
		}
	}
	if row.AuthPass != nil {
		if c.AuthPass, err = appcrypto.Decrypt(*row.AuthPass); err != nil {
			return nil, fmt.Errorf("decrypt auth_pass: %w", err)
		}
	}
	if row.PrivPass != nil {
		if c.PrivPass, err = appcrypto.Decrypt(*row.PrivPass); err != nil {
			return nil, fmt.Errorf("decrypt priv_pass: %w", err)
		}
	}
	if row.Username != nil {
		c.Username = *row.Username
	}
	if row.AuthProto != nil {
		c.AuthProto = *row.AuthProto
	}
	if row.PrivProto != nil {
		c.PrivProto = *row.PrivProto
	}
	return c, nil
}

const selectSNMPCredsColumns = `id, network_id, version, community, username,
	auth_proto, auth_pass, priv_proto, priv_pass, created_at, updated_at`

// ListSNMPCredentials returns all credential rows as safe (redacted) values.
// Secrets are never decrypted; presence is inferred from column nullability.
func (r *SNMPCredentialsRepository) ListSNMPCredentials(ctx context.Context) ([]SNMPCredentialSafe, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectSNMPCredsColumns+` FROM snmp_credentials ORDER BY network_id NULLS FIRST, version`)
	if err != nil {
		return nil, sanitizeDBError("list snmp credentials", err)
	}
	defer rows.Close() //nolint:errcheck

	var creds []SNMPCredentialSafe
	for rows.Next() {
		var row snmpCredRow
		if err := rows.Scan(&row.ID, &row.NetworkID, &row.Version, &row.Community,
			&row.Username, &row.AuthProto, &row.AuthPass, &row.PrivProto, &row.PrivPass,
			&row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, sanitizeDBError("scan snmp credential row", err)
		}
		creds = append(creds, row.redact())
	}
	if err := rows.Err(); err != nil {
		return nil, sanitizeDBError("iterate snmp credentials", err)
	}
	if creds == nil {
		creds = []SNMPCredentialSafe{}
	}
	return creds, nil
}

// redact builds a SNMPCredentialSafe from a raw DB row without decrypting secrets.
// A non-NULL column value is represented as redactedSecret in the safe view.
func (row *snmpCredRow) redact() SNMPCredentialSafe {
	safe := SNMPCredentialSafe{
		ID:        row.ID,
		NetworkID: row.NetworkID,
		Version:   row.Version,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.Username != nil {
		safe.Username = *row.Username
	}
	if row.AuthProto != nil {
		safe.AuthProto = *row.AuthProto
	}
	if row.PrivProto != nil {
		safe.PrivProto = *row.PrivProto
	}
	if row.Community != nil && *row.Community != "" {
		safe.Community = redactedSecret
	}
	if row.AuthPass != nil && *row.AuthPass != "" {
		safe.AuthPass = redactedSecret
	}
	if row.PrivPass != nil && *row.PrivPass != "" {
		safe.PrivPass = redactedSecret
	}
	return safe
}

// GetSNMPCredential returns a single credential by ID.
func (r *SNMPCredentialsRepository) GetSNMPCredential(ctx context.Context, id uuid.UUID) (*SNMPCredential, error) {
	var row snmpCredRow
	err := r.db.QueryRowContext(ctx,
		`SELECT `+selectSNMPCredsColumns+` FROM snmp_credentials WHERE id = $1`, id).
		Scan(&row.ID, &row.NetworkID, &row.Version, &row.Community,
			&row.Username, &row.AuthProto, &row.AuthPass, &row.PrivProto, &row.PrivPass,
			&row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFound("snmp_credential")
		}
		return nil, sanitizeDBError("get snmp credential", err)
	}
	return row.decrypt()
}

// GetEffectiveCommunity returns the decrypted SNMPv2c community string for the
// given network, falling back to the global default. Returns "public" if no
// credential is configured.
func (r *SNMPCredentialsRepository) GetEffectiveCommunity(ctx context.Context, networkID *uuid.UUID) (string, error) {
	// Try network-specific first, then global (network_id IS NULL).
	var candidates []*uuid.UUID
	if networkID != nil {
		candidates = append(candidates, networkID)
	}
	candidates = append(candidates, nil) // global fallback

	for _, nid := range candidates {
		var row snmpCredRow
		var err error
		if nid == nil {
			err = r.db.QueryRowContext(ctx,
				`SELECT `+selectSNMPCredsColumns+
					` FROM snmp_credentials WHERE network_id IS NULL AND version = 'v2c'`).
				Scan(&row.ID, &row.NetworkID, &row.Version, &row.Community,
					&row.Username, &row.AuthProto, &row.AuthPass, &row.PrivProto, &row.PrivPass,
					&row.CreatedAt, &row.UpdatedAt)
		} else {
			err = r.db.QueryRowContext(ctx,
				`SELECT `+selectSNMPCredsColumns+
					` FROM snmp_credentials WHERE network_id = $1 AND version = 'v2c'`, nid).
				Scan(&row.ID, &row.NetworkID, &row.Version, &row.Community,
					&row.Username, &row.AuthProto, &row.AuthPass, &row.PrivProto, &row.PrivPass,
					&row.CreatedAt, &row.UpdatedAt)
		}
		if stderrors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", sanitizeDBError("get effective snmp community", err)
		}
		c, err := row.decrypt()
		if err != nil {
			return "", err
		}
		if c.Community != "" {
			return c.Community, nil
		}
	}
	return "public", nil // built-in default
}

// UpsertSNMPCredential inserts or replaces a credential set. Secrets in the
// input are expected to be plain text; this method encrypts them before storage.
func (r *SNMPCredentialsRepository) UpsertSNMPCredential(
	ctx context.Context, c *SNMPCredential,
) (*SNMPCredential, error) {
	encCommunity, err := appcrypto.Encrypt(c.Community)
	if err != nil {
		return nil, fmt.Errorf("encrypt community: %w", err)
	}
	encAuthPass, err := appcrypto.Encrypt(c.AuthPass)
	if err != nil {
		return nil, fmt.Errorf("encrypt auth_pass: %w", err)
	}
	encPrivPass, err := appcrypto.Encrypt(c.PrivPass)
	if err != nil {
		return nil, fmt.Errorf("encrypt priv_pass: %w", err)
	}

	nullStr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	var row snmpCredRow
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO snmp_credentials
			(network_id, version, community, username, auth_proto, auth_pass, priv_proto, priv_pass, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (network_id, version) DO UPDATE SET
			community  = EXCLUDED.community,
			username   = EXCLUDED.username,
			auth_proto = EXCLUDED.auth_proto,
			auth_pass  = EXCLUDED.auth_pass,
			priv_proto = EXCLUDED.priv_proto,
			priv_pass  = EXCLUDED.priv_pass,
			updated_at = NOW()
		RETURNING `+selectSNMPCredsColumns,
		c.NetworkID,
		string(c.Version),
		nullStr(encCommunity),
		nullStr(c.Username),
		nullStr(c.AuthProto),
		nullStr(encAuthPass),
		nullStr(c.PrivProto),
		nullStr(encPrivPass),
	).Scan(&row.ID, &row.NetworkID, &row.Version, &row.Community,
		&row.Username, &row.AuthProto, &row.AuthPass, &row.PrivProto, &row.PrivPass,
		&row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, sanitizeDBError("upsert snmp credential", err)
	}
	return row.decrypt()
}

// DeleteSNMPCredential removes a credential by ID.
func (r *SNMPCredentialsRepository) DeleteSNMPCredential(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM snmp_credentials WHERE id = $1`, id)
	if err != nil {
		return sanitizeDBError("delete snmp credential", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return sanitizeDBError("delete snmp credential rows affected", err)
	}
	if n == 0 {
		return errors.ErrNotFound("snmp_credential")
	}
	return nil
}
