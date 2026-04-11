// Package db — unit tests for BannerRepository using sqlmock.
// These run without a live database.
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bannerCols is the column list returned by port_banners SELECT queries.
var bannerCols = []string{
	"id", "host_id", "port", "protocol",
	"raw_banner", "service", "version", "scanned_at",
}

// certCols is the column list returned by certificates SELECT queries.
var certCols = []string{
	"id", "host_id", "port", "subject_cn", "sans", "issuer",
	"not_before", "not_after", "key_type", "tls_version", "raw_banner", "scanned_at",
}

// ── NewBannerRepository ─────────────────────────────────────────────────────

func TestBannerRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewBannerRepository(db)
	require.NotNil(t, repo)
}

// ── UpsertPortBanner ────────────────────────────────────────────────────────

func TestBannerRepository_UpsertPortBanner_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	raw := "SSH-2.0-OpenSSH_8.9"
	svc := "ssh"
	b := &PortBanner{
		ID:        uuid.New(),
		HostID:    hostID,
		Port:      22,
		Protocol:  ProtocolTCP,
		RawBanner: &raw,
		Service:   &svc,
	}

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(b.ID, b.HostID, b.Port, b.Protocol, b.RawBanner, b.Service, b.Version, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertPortBanner(context.Background(), b)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertPortBanner_AssignsID(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	b := &PortBanner{
		HostID:   uuid.New(),
		Port:     80,
		Protocol: ProtocolTCP,
	}
	// ID is zero before the call.
	assert.Equal(t, uuid.Nil, b.ID)

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(sqlmock.AnyArg(), b.HostID, b.Port, b.Protocol,
			b.RawBanner, b.Service, b.Version, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertPortBanner(context.Background(), b)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, b.ID, "UpsertPortBanner should assign a UUID")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertPortBanner_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	b := &PortBanner{
		ID:       uuid.New(),
		HostID:   uuid.New(),
		Port:     22,
		Protocol: ProtocolTCP,
	}

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnError(fmt.Errorf("connection refused"))

	err := repo.UpsertPortBanner(context.Background(), b)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListPortBanners ─────────────────────────────────────────────────────────

func TestBannerRepository_ListPortBanners_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	now := time.Now().UTC()
	raw1, svc1 := "SSH-2.0-OpenSSH_8.9", "ssh"
	raw2, svc2 := "220 FTP ready", "ftp"

	rows := sqlmock.NewRows(bannerCols).
		AddRow(uuid.New(), hostID, 22, ProtocolTCP, &raw1, &svc1, nil, now).
		AddRow(uuid.New(), hostID, 21, ProtocolTCP, &raw2, &svc2, nil, now)

	mock.ExpectQuery("SELECT .* FROM port_banners").
		WithArgs(hostID).
		WillReturnRows(rows)

	banners, err := repo.ListPortBanners(context.Background(), hostID)
	require.NoError(t, err)
	assert.Len(t, banners, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListPortBanners_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT .* FROM port_banners").
		WithArgs(hostID).
		WillReturnRows(sqlmock.NewRows(bannerCols))

	banners, err := repo.ListPortBanners(context.Background(), hostID)
	require.NoError(t, err)
	assert.Empty(t, banners)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListPortBanners_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT .* FROM port_banners").
		WithArgs(hostID).
		WillReturnError(fmt.Errorf("query failed"))

	_, err := repo.ListPortBanners(context.Background(), hostID)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertCertificate ───────────────────────────────────────────────────────

func TestBannerRepository_UpsertCertificate_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	cn := "example.com"
	issuer := "Let's Encrypt"
	keyType := "RSA"
	tlsVer := "TLS 1.3"
	raw := "TLS TLS 1.3 CN=example.com"
	notBefore := time.Now().UTC().Add(-24 * time.Hour)
	notAfter := time.Now().UTC().Add(365 * 24 * time.Hour)

	c := &Certificate{
		ID:         uuid.New(),
		HostID:     hostID,
		Port:       443,
		SubjectCN:  &cn,
		SANs:       []string{"www.example.com"},
		Issuer:     &issuer,
		NotBefore:  &notBefore,
		NotAfter:   &notAfter,
		KeyType:    &keyType,
		TLSVersion: &tlsVer,
		RawBanner:  &raw,
	}

	mock.ExpectExec("INSERT INTO certificates").
		WithArgs(
			c.ID, c.HostID, c.Port, c.SubjectCN,
			sqlmock.AnyArg(),
			c.Issuer, c.NotBefore, c.NotAfter,
			c.KeyType, c.TLSVersion, c.RawBanner,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertCertificate(context.Background(), c)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertCertificate_AssignsID(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	c := &Certificate{
		HostID: uuid.New(),
		Port:   443,
		SANs:   []string{},
	}
	assert.Equal(t, uuid.Nil, c.ID)

	mock.ExpectExec("INSERT INTO certificates").
		WithArgs(
			sqlmock.AnyArg(), c.HostID, c.Port,
			c.SubjectCN,
			sqlmock.AnyArg(),
			c.Issuer, c.NotBefore, c.NotAfter,
			c.KeyType, c.TLSVersion, c.RawBanner,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertCertificate(context.Background(), c)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, c.ID, "UpsertCertificate should assign a UUID")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertCertificate_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	c := &Certificate{
		ID:     uuid.New(),
		HostID: uuid.New(),
		Port:   443,
	}

	mock.ExpectExec("INSERT INTO certificates").
		WillReturnError(fmt.Errorf("disk full"))

	err := repo.UpsertCertificate(context.Background(), c)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListCertificates ────────────────────────────────────────────────────────

func TestBannerRepository_ListCertificates_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	now := time.Now().UTC()
	notBefore := now.Add(-24 * time.Hour)
	notAfter := now.Add(365 * 24 * time.Hour)
	cn := "example.com"
	issuer := "Let's Encrypt"
	keyType := "RSA"
	tlsVer := "TLS 1.3"
	raw := "TLS TLS 1.3 CN=example.com"

	rows := sqlmock.NewRows(certCols).
		AddRow(
			uuid.New(), hostID, 443, &cn,
			pq.Array([]string{"a.example.com"}),
			&issuer, &notBefore, &notAfter,
			&keyType, &tlsVer, &raw, now,
		)

	mock.ExpectQuery("SELECT .* FROM certificates").
		WithArgs(hostID).
		WillReturnRows(rows)

	certs, err := repo.ListCertificates(context.Background(), hostID)
	require.NoError(t, err)
	require.Len(t, certs, 1)
	assert.Equal(t, "example.com", *certs[0].SubjectCN)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListCertificates_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT .* FROM certificates").
		WithArgs(hostID).
		WillReturnRows(sqlmock.NewRows(certCols))

	certs, err := repo.ListCertificates(context.Background(), hostID)
	require.NoError(t, err)
	assert.Empty(t, certs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListCertificates_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	hostID := uuid.New()
	mock.ExpectQuery("SELECT .* FROM certificates").
		WithArgs(hostID).
		WillReturnError(fmt.Errorf("query error"))

	_, err := repo.ListCertificates(context.Background(), hostID)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListExpiringCertificates ────────────────────────────────────────────────

func TestBannerRepository_ListExpiringCertificates_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	now := time.Now().UTC()
	notBefore := now.Add(-30 * 24 * time.Hour)
	notAfter := now.Add(7 * 24 * time.Hour)
	cn := "expiring.example.com"
	issuer := "Test CA"
	keyType := "ECDSA"
	tlsVer := "TLS 1.3"
	raw := "TLS TLS 1.3 CN=expiring.example.com"

	rows := sqlmock.NewRows(certCols).
		AddRow(
			uuid.New(), uuid.New(), 443, &cn,
			pq.Array([]string{"expiring.example.com"}),
			&issuer, &notBefore, &notAfter,
			&keyType, &tlsVer, &raw, now,
		)

	mock.ExpectQuery("SELECT .* FROM certificates").
		WithArgs(30).
		WillReturnRows(rows)

	certs, err := repo.ListExpiringCertificates(context.Background(), 30)
	require.NoError(t, err)
	require.Len(t, certs, 1)
	assert.Equal(t, "expiring.example.com", *certs[0].SubjectCN)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListExpiringCertificates_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	mock.ExpectQuery("SELECT .* FROM certificates").
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows(certCols))

	certs, err := repo.ListExpiringCertificates(context.Background(), 7)
	require.NoError(t, err)
	assert.Empty(t, certs)
	require.NoError(t, mock.ExpectationsWereMet())
}
