// Package db — unit tests for SNMPCredentialsRepository using sqlmock.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcrypto "github.com/anstrom/scanorama/internal/crypto"
	"github.com/anstrom/scanorama/internal/errors"
)

var snmpCredCols = []string{
	"id", "network_id", "version", "community", "username",
	"auth_proto", "auth_pass", "priv_proto", "priv_pass",
	"created_at", "updated_at",
}

// encryptOrPanic is a test helper that encrypts a string or panics.
func encryptOrPanic(s string) *string {
	if s == "" {
		return nil
	}
	enc, err := appcrypto.Encrypt(s)
	if err != nil {
		panic(err)
	}
	return &enc
}

// ── ListSNMPCredentials ───────────────────────────────────────────────────────

func TestSNMPCredentialsRepository_List_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredCols))

	creds, err := NewSNMPCredentialsRepository(db).ListSNMPCredentials(context.Background())

	require.NoError(t, err)
	assert.Empty(t, creds)
	assert.NotNil(t, creds, "empty result must be [] not nil")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_List_RedactsSecrets(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	encCommunity := encryptOrPanic("secret-community")
	rows := sqlmock.NewRows(snmpCredCols).
		AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").WillReturnRows(rows)

	creds, err := NewSNMPCredentialsRepository(db).ListSNMPCredentials(context.Background())

	require.NoError(t, err)
	require.Len(t, creds, 1)
	assert.Equal(t, "***", creds[0].Community, "community must be redacted")
	assert.Empty(t, creds[0].AuthPass)
	assert.Empty(t, creds[0].PrivPass)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_List_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").
		WillReturnError(fmt.Errorf("connection reset"))

	_, err := NewSNMPCredentialsRepository(db).ListSNMPCredentials(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetEffectiveCommunity ─────────────────────────────────────────────────────

func TestSNMPCredentialsRepository_GetEffectiveCommunity_NetworkHit(t *testing.T) {
	db, mock := newMockDB(t)
	netID := uuid.New()
	id := uuid.New()
	now := time.Now().UTC()

	encCommunity := encryptOrPanic("net-community")
	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id").
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(snmpCredCols).
			AddRow(id, &netID, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	community, err := NewSNMPCredentialsRepository(db).GetEffectiveCommunity(context.Background(), &netID)

	require.NoError(t, err)
	assert.Equal(t, "net-community", community)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_GetEffectiveCommunity_NetworkMiss_GlobalHit(t *testing.T) {
	db, mock := newMockDB(t)
	netID := uuid.New()
	id := uuid.New()
	now := time.Now().UTC()

	// Network-specific query returns no row.
	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id").
		WithArgs(netID).
		WillReturnError(sql.ErrNoRows)

	// Global fallback query returns a row.
	encCommunity := encryptOrPanic("global-community")
	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id IS NULL").
		WillReturnRows(sqlmock.NewRows(snmpCredCols).
			AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	community, err := NewSNMPCredentialsRepository(db).GetEffectiveCommunity(context.Background(), &netID)

	require.NoError(t, err)
	assert.Equal(t, "global-community", community)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_GetEffectiveCommunity_BothAbsent(t *testing.T) {
	db, mock := newMockDB(t)
	netID := uuid.New()

	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id").
		WithArgs(netID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id IS NULL").
		WillReturnError(sql.ErrNoRows)

	community, err := NewSNMPCredentialsRepository(db).GetEffectiveCommunity(context.Background(), &netID)

	require.NoError(t, err)
	assert.Equal(t, "public", community, "must fall back to built-in default")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_GetEffectiveCommunity_NilNetworkID(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	// Only the global query should be issued — no network-scoped query.
	encCommunity := encryptOrPanic("global-only")
	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id IS NULL").
		WillReturnRows(sqlmock.NewRows(snmpCredCols).
			AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	community, err := NewSNMPCredentialsRepository(db).GetEffectiveCommunity(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, "global-only", community)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_GetEffectiveCommunity_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	netID := uuid.New()

	mock.ExpectQuery("SELECT .* FROM snmp_credentials WHERE network_id").
		WithArgs(netID).
		WillReturnError(fmt.Errorf("db unavailable"))

	_, err := NewSNMPCredentialsRepository(db).GetEffectiveCommunity(context.Background(), &netID)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertSNMPCredential ──────────────────────────────────────────────────────

func TestSNMPCredentialsRepository_Upsert_V2c(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()
	encCommunity := encryptOrPanic("my-community")

	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredCols).
			AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	cred := &SNMPCredential{
		Version:   SNMPVersionV2c,
		Community: "my-community",
	}
	saved, err := NewSNMPCredentialsRepository(db).UpsertSNMPCredential(context.Background(), cred)

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "my-community", saved.Community, "returned credential must be decrypted")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_Upsert_V3(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()
	encAuthPass := encryptOrPanic("auth-secret")
	encPrivPass := encryptOrPanic("priv-secret")

	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredCols).
			AddRow(id, nil, "v3", nil,
				strPtr("admin"), strPtr("SHA"), encAuthPass,
				strPtr("AES"), encPrivPass, now, now))

	cred := &SNMPCredential{
		Version:   SNMPVersionV3,
		Username:  "admin",
		AuthProto: "SHA",
		AuthPass:  "auth-secret",
		PrivProto: "AES",
		PrivPass:  "priv-secret",
	}
	saved, err := NewSNMPCredentialsRepository(db).UpsertSNMPCredential(context.Background(), cred)

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "admin", saved.Username)
	assert.Equal(t, "auth-secret", saved.AuthPass)
	assert.Equal(t, "priv-secret", saved.PrivPass)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_Upsert_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnError(fmt.Errorf("constraint violation"))

	_, err := NewSNMPCredentialsRepository(db).UpsertSNMPCredential(context.Background(), &SNMPCredential{
		Version:   SNMPVersionV2c,
		Community: "x",
	})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteSNMPCredential ──────────────────────────────────────────────────────

func TestSNMPCredentialsRepository_Delete_OK(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := NewSNMPCredentialsRepository(db).DeleteSNMPCredential(context.Background(), id)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_Delete_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewSNMPCredentialsRepository(db).DeleteSNMPCredential(context.Background(), id)

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsRepository_Delete_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnError(fmt.Errorf("connection lost"))

	err := NewSNMPCredentialsRepository(db).DeleteSNMPCredential(context.Background(), id)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── Redact ────────────────────────────────────────────────────────────────────

func TestSNMPCredential_Redact_AllSecretsSet(t *testing.T) {
	c := &SNMPCredential{
		ID:        uuid.New(),
		Version:   SNMPVersionV3,
		Community: "comm",
		Username:  "admin",
		AuthProto: "SHA",
		AuthPass:  "auth-pass",
		PrivProto: "AES",
		PrivPass:  "priv-pass",
	}
	safe := c.Redact()

	assert.Equal(t, "***", safe.Community)
	assert.Equal(t, "***", safe.AuthPass)
	assert.Equal(t, "***", safe.PrivPass)
	// Non-secret fields must pass through unchanged.
	assert.Equal(t, "admin", safe.Username)
	assert.Equal(t, "SHA", safe.AuthProto)
	assert.Equal(t, "AES", safe.PrivProto)
}

func TestSNMPCredential_Redact_NoSecrets(t *testing.T) {
	c := &SNMPCredential{
		ID:      uuid.New(),
		Version: SNMPVersionV2c,
	}
	safe := c.Redact()

	assert.Empty(t, safe.Community)
	assert.Empty(t, safe.AuthPass)
	assert.Empty(t, safe.PrivPass)
}
