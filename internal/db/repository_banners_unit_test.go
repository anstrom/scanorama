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
	"raw_banner", "service", "version",
	"http_title", "ssh_key_fingerprint",
	"http_status_code", "http_redirect", "http_response_headers",
	"scanned_at",
}

// certCols is the column list returned by certificates SELECT queries.
var certCols = []string{
	"id", "host_id", "port", "subject_cn", "sans", "issuer",
	"not_before", "not_after", "key_type", "tls_version", "cipher_suite", "raw_banner", "scanned_at",
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

// ── UpsertNSEPortData ───────────────────────────────────────────────────────

func TestBannerRepository_UpsertNSEPortData_OK(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	title := "Admin Console"
	fingerprint := "2048 SHA256:abc123 (RSA)"

	b := &PortBanner{
		HostID:            hostID,
		Port:              443,
		Protocol:          ProtocolTCP,
		HTTPTitle:         &title,
		SSHKeyFingerprint: &fingerprint,
	}

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(sqlmock.AnyArg(), hostID, 443, ProtocolTCP,
			b.RawBanner, &title, &fingerprint, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.UpsertNSEPortData(context.Background(), b))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertNSEPortData_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnError(fmt.Errorf("deadlock"))

	err := repo.UpsertNSEPortData(context.Background(), &PortBanner{
		HostID:   uuid.New(),
		Port:     80,
		Protocol: ProtocolTCP,
	})
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
		AddRow(uuid.New(), hostID, 22, ProtocolTCP, &raw1, &svc1, nil, nil, nil, nil, nil, nil, now).
		AddRow(uuid.New(), hostID, 21, ProtocolTCP, &raw2, &svc2, nil, nil, nil, nil, nil, nil, now)

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
			c.KeyType, c.TLSVersion, c.CipherSuite, c.RawBanner,
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
			c.KeyType, c.TLSVersion, c.CipherSuite, c.RawBanner,
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
			&keyType, &tlsVer, nil, &raw, now,
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
			&keyType, &tlsVer, nil, &raw, now,
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

// ── ListExpiringCertificatesWithHosts ────────────────────────────────────────

var expiringWithHostsCols = []string{
	"host_id", "ip_address", "hostname", "port", "subject_cn", "not_after",
}

func TestBannerRepository_ListExpiringCertificatesWithHosts_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	notAfter := time.Now().UTC().Add(14 * 24 * time.Hour)
	hostID := uuid.New().String()

	rows := sqlmock.NewRows(expiringWithHostsCols).
		AddRow(hostID, "192.168.1.100", "server01.local", 443, "server01.local", notAfter)

	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnRows(rows)

	result, err := repo.ListExpiringCertificatesWithHosts(context.Background(), 30)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, hostID, result[0].HostID)
	assert.Equal(t, "192.168.1.100", result[0].HostIP)
	assert.Equal(t, "server01.local", result[0].Hostname)
	assert.Equal(t, 443, result[0].Port)
	assert.Equal(t, "server01.local", result[0].SubjectCN)
	assert.Equal(t, "tcp", result[0].Protocol)
	// DaysLeft = int(hours / 24); allow ±1 for execution time
	assert.InDelta(t, 14, result[0].DaysLeft, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListExpiringCertificatesWithHosts_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows(expiringWithHostsCols))

	result, err := repo.ListExpiringCertificatesWithHosts(context.Background(), 30)
	require.NoError(t, err)
	assert.NotNil(t, result, "should return [] not nil")
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_ListExpiringCertificatesWithHosts_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewBannerRepository(db)

	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnError(fmt.Errorf("join failed"))

	_, err := repo.ListExpiringCertificatesWithHosts(context.Background(), 30)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertHTTPPortData ───────────────────────────────────────────────────────

func TestBannerRepository_UpsertHTTPPortData_OK(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	raw := "HTTP 200 https"
	svc := "https"
	var sc int16 = 200
	redirect := "https://example.com/"
	b := &PortBanner{
		ID:             uuid.New(),
		HostID:         hostID,
		Port:           443,
		Protocol:       ProtocolTCP,
		RawBanner:      &raw,
		Service:        &svc,
		HTTPStatusCode: &sc,
		HTTPRedirect:   &redirect,
	}

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			b.ID, b.HostID, b.Port, b.Protocol,
			b.RawBanner, b.Service, b.Version,
			b.HTTPStatusCode, b.HTTPRedirect, b.HTTPResponseHeaders,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertHTTPPortData(context.Background(), b)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertHTTPPortData_AssignsID(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	b := &PortBanner{
		HostID:   uuid.New(),
		Port:     80,
		Protocol: ProtocolTCP,
	}
	assert.Equal(t, uuid.Nil, b.ID)

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			sqlmock.AnyArg(), b.HostID, b.Port, b.Protocol,
			b.RawBanner, b.Service, b.Version,
			b.HTTPStatusCode, b.HTTPRedirect, b.HTTPResponseHeaders,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertHTTPPortData(context.Background(), b)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, b.ID, "UpsertHTTPPortData should assign a UUID")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertHTTPPortData_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnError(fmt.Errorf("deadlock"))

	err := repo.UpsertHTTPPortData(context.Background(), &PortBanner{
		ID:       uuid.New(),
		HostID:   uuid.New(),
		Port:     80,
		Protocol: ProtocolTCP,
	})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertSSHPortData ────────────────────────────────────────────────────────

func TestBannerRepository_UpsertSSHPortData_OK(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	hostID := uuid.New()
	raw := "SSH-2.0-OpenSSH_9.0"
	svc := "ssh"
	ver := "OpenSSH_9.0"
	fp := "SHA256:abc123xyz"
	b := &PortBanner{
		ID:                uuid.New(),
		HostID:            hostID,
		Port:              22,
		Protocol:          ProtocolTCP,
		RawBanner:         &raw,
		Service:           &svc,
		Version:           &ver,
		SSHKeyFingerprint: &fp,
	}

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			b.ID, b.HostID, b.Port, b.Protocol,
			b.RawBanner, b.Service, b.Version, b.SSHKeyFingerprint,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertSSHPortData(context.Background(), b)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertSSHPortData_AssignsID(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	b := &PortBanner{
		HostID:   uuid.New(),
		Port:     22,
		Protocol: ProtocolTCP,
	}
	assert.Equal(t, uuid.Nil, b.ID)

	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			sqlmock.AnyArg(), b.HostID, b.Port, b.Protocol,
			b.RawBanner, b.Service, b.Version, b.SSHKeyFingerprint,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertSSHPortData(context.Background(), b)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, b.ID, "UpsertSSHPortData should assign a UUID")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBannerRepository_UpsertSSHPortData_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewBannerRepository(database)

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnError(fmt.Errorf("connection reset"))

	err := repo.UpsertSSHPortData(context.Background(), &PortBanner{
		ID:       uuid.New(),
		HostID:   uuid.New(),
		Port:     22,
		Protocol: ProtocolTCP,
	})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

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
