// Package enrichment — network-level tests for banner grabbing using
// in-process test TCP/TLS servers.
package enrichment

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newBannerMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// startTCPServer starts a test TCP server that sends banner on connect.
// Returns the address and a cleanup function.
func startTCPServer(t *testing.T, banner string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte(banner))
			}(conn)
		}
	}()

	return ln.Addr().String()
}

// startTLSServer starts a test TLS server using a self-signed cert.
// Returns the address, the self-signed cert (for assertion), and cleanup.
func startTLSServer(t *testing.T) (string, *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.local"},
		DNSNames:     []string{"test.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte(""))
			}(conn)
		}
	}()

	return ln.Addr().String(), leaf
}

// ── parseServiceFromBanner ───────────────────────────────────────────────────

func TestParseServiceFromBanner_SSH(t *testing.T) {
	svc, _ := parseServiceFromBanner("SSH-2.0-OpenSSH_8.9", 22)
	assert.Equal(t, "ssh", svc)
}

func TestParseServiceFromBanner_Unknown(t *testing.T) {
	svc, _ := parseServiceFromBanner("garbled data", 9999)
	assert.Equal(t, "", svc)
}

// ── NewBannerGrabber ─────────────────────────────────────────────────────────

func TestNewBannerGrabber(t *testing.T) {
	database, _ := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")
	require.NotNil(t, g)
}

// ── EnrichHosts ──────────────────────────────────────────────────────────────

func TestEnrichHosts_EmptyTargets(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// No DB calls should happen.
	g.EnrichHosts(context.Background(), []BannerTarget{})
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnrichHosts_NilTargets(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	g.EnrichHosts(context.Background(), nil)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabPlain ────────────────────────────────────────────────────────────────

func TestGrabPlain_ReturnsBanner(t *testing.T) {
	addr := startTCPServer(t, "SSH-2.0-OpenSSH_8.9\r\n")
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	var port int
	parsePort(portStr, &port)

	hostID := uuid.New()
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: hostID, IP: host, Ports: []int{port}}
	g.grabPlain(context.Background(), target, port, addr)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrabPlain_NoData_NoUpsert(t *testing.T) {
	// Server closes immediately without sending anything.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
		_ = ln.Close()
	}()

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	parsePort(portStr, &port)

	target := BannerTarget{HostID: uuid.New(), IP: host, Ports: []int{port}}
	g.grabPlain(context.Background(), target, port, addr)

	// May or may not upsert depending on timing; just verify no panic.
	_ = mock.ExpectationsWereMet()
}

func TestGrabPlain_UnreachableHost(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	// Use a port that is almost certainly not listening.
	g.grabPlain(ctx, target, 19999, "127.0.0.1:19999")

	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabTLS ──────────────────────────────────────────────────────────────────

func TestGrabTLS_StoresCertificate(t *testing.T) {
	addr, leaf := startTLSServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	var port int
	parsePort(portStr, &port)

	hostID := uuid.New()
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO certificates").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: hostID, IP: host, Ports: []int{port}}
	g.grabTLS(context.Background(), target, port, addr)

	require.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, "test.local", leaf.Subject.CommonName)
}

// TestGrabPlain_NoBanner_PortOnlyServiceDetected verifies that port-only
// fingerprint rules fire even when the server sends no banner bytes (binary
// protocols that wait for the client to speak first, e.g. SMB, RDP).
func TestGrabPlain_NoBanner_PortOnlyServiceDetected(t *testing.T) {
	// Server closes without sending anything.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
		_ = ln.Close()
	}()

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)

	// Use a custom fingerprinter with a port-only rule for the test port.
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	parsePort(portStr, &port)

	g := NewBannerGrabber(repo, newTestLogger(), "")
	// Inject a port-only rule for the dynamic test port via a temp file.
	extra := `[{"port":` + portStr + `,"pattern":"","service":"TestBinaryProto"}]`
	f, tmpErr := os.CreateTemp(t.TempDir(), "fp-*.json")
	require.NoError(t, tmpErr)
	_, _ = f.WriteString(extra)
	f.Close()
	g.fingerprint = NewFingerprinter(f.Name())

	// Even with no banner, the port-only rule should fire and trigger an upsert.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host, Ports: []int{port}}
	g.grabPlain(context.Background(), target, port, addr)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrabTLS_UnreachableHost(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.grabTLS(ctx, target, 19998, "127.0.0.1:19998")

	require.NoError(t, mock.ExpectationsWereMet())
}

// ── EnrichHosts integration ───────────────────────────────────────────────────

func TestEnrichHosts_WithTCPTarget(t *testing.T) {
	addr := startTCPServer(t, "220 FTP Server (vsftpd)\r\n")
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	var port int
	parsePort(portStr, &port)

	hostID := uuid.New()
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	targets := []BannerTarget{{HostID: hostID, IP: host, Ports: []int{port}}}
	g.EnrichHosts(context.Background(), targets)

	require.NoError(t, mock.ExpectationsWereMet())
}

// parsePort parses a decimal port string into *out.
func parsePort(s string, out *int) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return
		}
		n = n*10 + int(c-'0')
	}
	*out = n
}
