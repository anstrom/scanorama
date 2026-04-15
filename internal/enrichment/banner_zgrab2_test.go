//go:build !race

// Package enrichment — zgrab2 integration tests.
// These tests exercise grabZGrabHTTP, grabZGrabHTTPS, and grabZGrabSSH against
// in-process test servers.  They are excluded from race-detector runs because
// zgrab2's TimeoutConnection has a known internal data race (concurrent read/write
// in SaturateTimeoutsToReadAndWriteTimeouts) that is upstream and not fixable here.
package enrichment

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/anstrom/scanorama/internal/db"
)

// startHTTPServer starts a test plain-HTTP server returning a minimal 200 OK.
func startHTTPServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "TestHTTPD/1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// startHTTPSServer starts a test TLS-HTTP server using a self-signed cert.
// Returns address and the leaf certificate for assertion.
func startHTTPSServer(t *testing.T) (string, *x509.Certificate) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "TestHTTPSD/1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	srv.StartTLS()
	t.Cleanup(srv.Close)
	leaf := srv.TLS.Certificates[0].Leaf
	if leaf == nil {
		// httptest populates Leaf lazily on first use; parse it.
		var err error
		leaf, err = x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
		require.NoError(t, err)
	}
	return srv.Listener.Addr().String(), leaf
}

// startSSHServer starts a minimal SSH server that completes the key exchange but
// does not accept any authentication, mirroring the DontAuthenticate behavior
// used by grabZGrabSSH.
func startSSHServer(t *testing.T) string {
	t.Helper()

	hostKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(hostKey)
	require.NoError(t, err)

	cfg := &gossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

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
				srvConn, _, _, _ := gossh.NewServerConn(c, cfg)
				if srvConn != nil {
					_ = srvConn.Close()
				}
			}(conn)
		}
	}()

	return ln.Addr().String()
}

// ── grabZGrabHTTP ─────────────────────────────────────────────────────────────

func TestGrabZGrabHTTP_StoresBanner(t *testing.T) {
	addr := startHTTPServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host, Ports: []PortInfo{{Number: port, Service: "http"}}}
	err = g.grabZGrabHTTP(context.Background(), target, port)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrabZGrabHTTP_UnreachableHost(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	err := g.grabZGrabHTTP(ctx, target, 19997)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabZGrabHTTPS ────────────────────────────────────────────────────────────

func TestGrabZGrabHTTPS_StoresCertAndBanner(t *testing.T) {
	addr, _ := startHTTPSServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO certificates").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host, Ports: []PortInfo{{Number: port}}}
	err = g.grabZGrabHTTPS(context.Background(), target, port)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrabZGrabHTTPS_UnreachableHost(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	err := g.grabZGrabHTTPS(ctx, target, 19996)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabZGrabSSH ──────────────────────────────────────────────────────────────

func TestGrabZGrabSSH_StoresBanner(t *testing.T) {
	addr := startSSHServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host, Ports: []PortInfo{{Number: port, Service: "ssh"}}}
	err = g.grabZGrabSSH(context.Background(), target, port, addr)
	// SSH grab may return an error from DontAuthenticate rejection; the banner
	// is still stored when the key exchange succeeds.
	_ = err
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrabZGrabSSH_UnreachableHost(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	err := g.grabZGrabSSH(ctx, target, 19995, "127.0.0.1:19995")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

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

	// HTTP grab stores banner.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// MarkExtendedProbeDone.
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

	// All probes fail; grabPlain finds nothing to store, but MarkExtendedProbeDone runs.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.probeUnknown(ctx, target, 19990)

	require.NoError(t, mock.ExpectationsWereMet())
}

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

	// IsExtendedProbeDone returns false (no row) → probeUnknown runs.
	mock.ExpectQuery("SELECT extended_probe_done FROM port_banners").
		WillReturnRows(sqlmock.NewRows([]string{"extended_probe_done"}))
	// HTTP grab succeeds → banner INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// MarkExtendedProbeDone → flag INSERT.
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

	// No DB query expected — cap exceeded, probeUnknown skipped entirely.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "", AllowExtendedProbe: false}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}
