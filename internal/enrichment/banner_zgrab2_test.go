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
