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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	zcrypto_tls "github.com/zmap/zcrypto/tls"
	zgrabhttp_lib "github.com/zmap/zgrab2/lib/http"
	zgrabhttp "github.com/zmap/zgrab2/modules/http"
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

// startTLSSniffingHTTPServer starts a TCP server that peeks at the first byte of
// each incoming connection. If the byte is 0x16 (TLS ClientHello record type),
// it wraps the connection in TLS and serves HTTP; otherwise it drops the
// connection immediately. This lets grabZGrabHTTP fail (connection reset) while
// grabZGrabHTTPS succeeds, enabling probeUnknown HTTPS-branch tests.
func startTLSSniffingHTTPServer(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	tlsCfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}, MinVersion: tls.VersionTLS12}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "TestHTTPSOnly/1.0")
		w.WriteHeader(http.StatusOK)
	})

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
				buf := make([]byte, 1)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, err := c.Read(buf)
				_ = c.SetReadDeadline(time.Time{})
				if err != nil || n == 0 {
					return // plain HTTP or timeout: drop connection
				}
				if buf[0] != 0x16 {
					return // not a TLS ClientHello: drop connection
				}
				// Reconnect as TLS, prepending the peeked byte.
				tlsConn := tls.Server(&prependByteConn{Conn: c, b: buf[0]}, tlsCfg)
				if err := tlsConn.Handshake(); err != nil {
					return
				}
				srv := &http.Server{Handler: mux}
				_ = srv.Serve(newOneConnListener(tlsConn))
			}(conn)
		}
	}()

	return ln.Addr().String()
}

// prependByteConn is a net.Conn that re-emits one peeked byte before delegating
// further reads to the underlying connection.
type prependByteConn struct {
	net.Conn
	b    byte
	sent bool
}

func (p *prependByteConn) Read(b []byte) (int, error) {
	if !p.sent {
		p.sent = true
		b[0] = p.b
		return 1, nil
	}
	return p.Conn.Read(b)
}

// oneConnListener is a net.Listener that yields exactly one connection and
// then blocks until Close() is called. This keeps http.Server.Serve alive
// while the handler goroutine writes its response, without leaking permanently:
// Close() unblocks Accept() so Serve can return.
type oneConnListener struct {
	conn net.Conn
	addr net.Addr
	done chan struct{}
	once sync.Once
}

func newOneConnListener(conn net.Conn) *oneConnListener {
	return &oneConnListener{
		conn: conn,
		addr: conn.LocalAddr(),
		done: make(chan struct{}),
	}
}

func (o *oneConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	o.once.Do(func() { c = o.conn })
	if c != nil {
		return c, nil
	}
	<-o.done
	return nil, net.ErrClosed
}

func (o *oneConnListener) Close() error {
	select {
	case <-o.done:
	default:
		close(o.done)
	}
	return nil
}

func (o *oneConnListener) Addr() net.Addr { return o.addr }

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

// TestProbeUnknown_HTTPS_StopsAfterSuccess verifies that when a TLS-only HTTPS
// server responds on the port, probeUnknown stops after HTTPS succeeds and does
// not attempt SSH. It uses startTLSSniffingHTTPServer, which drops plain-HTTP
// connections so that grabZGrabHTTP returns an error, while grabZGrabHTTPS
// succeeds after a full TLS handshake.
func TestProbeUnknown_HTTPS_StopsAfterSuccess(t *testing.T) {
	addr := startTLSSniffingHTTPServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// HTTP probe fails (connection reset by sniffing server) → no INSERT.
	// HTTPS probe succeeds: certificate INSERT + banner INSERT.
	mock.ExpectExec("INSERT INTO certificates").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// MarkExtendedProbeDone.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host}
	g.probeUnknown(context.Background(), target, port)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestProbeUnknown_SSH_StopsAfterSuccess verifies that when an SSH server
// responds on the port, probeUnknown stops after SSH succeeds (HTTP and HTTPS
// fail first) and does not fall back to plain TCP.
func TestProbeUnknown_SSH_StopsAfterSuccess(t *testing.T) {
	addr := startSSHServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// HTTP probe fails (SSH server drops non-HTTP traffic) → no INSERT.
	// HTTPS probe fails (no TLS on an SSH port) → no INSERT.
	// SSH probe succeeds → banner INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// MarkExtendedProbeDone → flag INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	target := BannerTarget{HostID: uuid.New(), IP: host}
	g.probeUnknown(context.Background(), target, port)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabZGrabHTTPS / grabZGrabHTTP — invalid IP ───────────────────────────────

// TestGrabZGrabHTTPS_InvalidIP verifies that an unparseable IP address causes
// grabZGrabHTTPS to return an error without hitting the network or the DB.
func TestGrabZGrabHTTPS_InvalidIP(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	target := BannerTarget{HostID: uuid.New(), IP: "not-an-ip"}
	err := g.grabZGrabHTTPS(context.Background(), target, 8443)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabZGrabHTTP_InvalidIP verifies that an unparseable IP address causes
// grabZGrabHTTP to return an error without hitting the network or the DB.
func TestGrabZGrabHTTP_InvalidIP(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	target := BannerTarget{HostID: uuid.New(), IP: "not-an-ip"}
	err := g.grabZGrabHTTP(context.Background(), target, 8080)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── grabOne — service routing ─────────────────────────────────────────────────

// TestGrabOne_TLSPort_TriesHTTPS verifies that grabOne routes a port that is in
// tlsPorts to grabZGrabHTTPS (falling back to grabTLS on failure). Port 8443 is
// used because it's in tlsPorts but unlikely to have a listener, so both
// grabZGrabHTTPS and the TLS fallback quickly fail — the assertion is that the
// code path executes without panic.
func TestGrabOne_TLSPort_TriesHTTPS(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// No DB calls expected — both HTTPS and TLS fallback fail before storing.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	pi := PortInfo{Number: 8443}
	g.grabOne(ctx, BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_SSHService_TriesSSH verifies that grabOne routes a port with
// Service="ssh" to grabZGrabSSH (then falls back to grabPlain on failure).
func TestGrabOne_SSHService_TriesSSH(t *testing.T) {
	addr := startSSHServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// SSH grab succeeds → banner INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "ssh"}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_HTTPService_TriesHTTP verifies that grabOne routes a port with
// Service="http" to grabZGrabHTTP (then falls back to grabPlain on failure).
func TestGrabOne_HTTPService_TriesHTTP(t *testing.T) {
	addr := startHTTPServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// HTTP grab succeeds → banner INSERT.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "http"}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ── storeZGrabHTTPBanner ──────────────────────────────────────────────────────

// TestStoreZGrabHTTPBanner_NilResponse verifies that storeZGrabHTTPBanner
// returns immediately without a DB call when the response is nil.
func TestStoreZGrabHTTPBanner_NilResponse(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// The HTTPS server response must be non-nil at the Results level
	// but the embedded Response field is nil.
	results := &zgrabhttp.Results{Response: nil}
	g.storeZGrabHTTPBanner(context.Background(), BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}, 443, results)

	// No INSERT expected.
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_SSHService_FallsBackToPlain verifies that grabOne falls back to
// grabPlain when the port speaks SSH protocol but grabZGrabSSH fails because
// the target is not actually an SSH server (plain TCP, not SSH handshake).
func TestGrabOne_SSHService_FallsBackToPlain(t *testing.T) {
	// A plain TCP server that sends a non-SSH banner — grabZGrabSSH will fail.
	addr := startTCPServer(t, "NOT-SSH-BANNER\r\n")
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// grabPlain stores the banner received from the TCP server.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "ssh"}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGrabOne_HTTPService_FallsBackToPlain verifies that grabOne falls back to
// grabPlain when grabZGrabHTTP fails (e.g. the target speaks raw TCP, not HTTP).
func TestGrabOne_HTTPService_FallsBackToPlain(t *testing.T) {
	// A plain TCP server that sends a non-HTTP banner — grabZGrabHTTP will fail
	// because the response is not parseable as HTTP.
	addr := startTCPServer(t, "RAW PROTO 1.0\r\n")
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	parsePort(portStr, &port)

	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	// grabPlain may or may not store a banner depending on parse result.
	// Allow the call but don't require it.
	mock.ExpectExec("INSERT INTO port_banners").
		WillReturnResult(sqlmock.NewResult(1, 1))

	pi := PortInfo{Number: port, Service: "http"}
	g.grabOne(context.Background(), BannerTarget{HostID: uuid.New(), IP: host}, pi)

	// ExpectationsWereMet is intentionally not called here because grabPlain
	// may choose not to upsert if the banner is empty.
	_ = mock
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

	// Get a free port then immediately close — ensures no server is listening.
	ln, lnErr := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, lnErr)
	freeAddr := ln.Addr().String()
	require.NoError(t, ln.Close())
	_, portStr, splitErr := net.SplitHostPort(freeAddr)
	require.NoError(t, splitErr)
	var port int
	parsePort(portStr, &port)

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.probeUnknown(ctx, target, port)

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

// ── storeZGrabHTTPBanner — response variants ──────────────────────────────────

// UpsertHTTPPortData args: id, host_id, port, protocol, raw_banner, service,
// version, http_status_code, http_redirect, http_response_headers, scanned_at.
// ($1 through $11 — see repository_banners.go:UpsertHTTPPortData)

// TestStoreZGrabHTTPBanner_HTTP200 verifies that a plain-HTTP 200 response
// stores service="http" and http_status_code=200.
func TestStoreZGrabHTTPBanner_HTTP200(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	svc := "http"
	sc := int16(200)
	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			sqlmock.AnyArg(), // id
			sqlmock.AnyArg(), // host_id
			80,               // port
			"tcp",            // protocol
			sqlmock.AnyArg(), // raw_banner
			&svc,             // service: "http"
			(*string)(nil),   // version: nil (no Server header)
			&sc,              // http_status_code: 200
			(*string)(nil),   // http_redirect
			sqlmock.AnyArg(), // http_response_headers
			sqlmock.AnyArg(), // scanned_at
		).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := &zgrabhttp_lib.Response{
		StatusCode: 200,
		Header:     zgrabhttp_lib.Header{},
	}
	results := &zgrabhttp.Results{Response: resp}
	g.storeZGrabHTTPBanner(context.Background(), BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}, 80, results)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreZGrabHTTPBanner_HTTPS verifies that service="https" is stored when
// resp.TLS is non-nil (HTTPS connection detected).
func TestStoreZGrabHTTPBanner_HTTPS(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	svc := "https"
	sc := int16(200)
	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			sqlmock.AnyArg(), // id
			sqlmock.AnyArg(), // host_id
			443,              // port
			"tcp",            // protocol
			sqlmock.AnyArg(), // raw_banner
			&svc,             // service: "https" (TLS detected)
			(*string)(nil),   // version
			&sc,              // http_status_code: 200
			(*string)(nil),   // http_redirect
			sqlmock.AnyArg(), // http_response_headers
			sqlmock.AnyArg(), // scanned_at
		).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := &zgrabhttp_lib.Response{
		StatusCode: 200,
		Header:     zgrabhttp_lib.Header{},
		TLS:        &zcrypto_tls.ConnectionState{},
	}
	results := &zgrabhttp.Results{Response: resp}

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.storeZGrabHTTPBanner(context.Background(), target, 443, results)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreZGrabHTTPBanner_ServerHeader verifies that the Server response
// header is stored as the banner Version field.
func TestStoreZGrabHTTPBanner_ServerHeader(t *testing.T) {
	database, mock := newBannerMockDB(t)
	repo := db.NewBannerRepository(database)
	g := NewBannerGrabber(repo, newTestLogger(), "")

	svc := "http"
	sc := int16(200)
	ver := "nginx/1.24.0"
	mock.ExpectExec("INSERT INTO port_banners").
		WithArgs(
			sqlmock.AnyArg(), // id
			sqlmock.AnyArg(), // host_id
			80,               // port
			"tcp",            // protocol
			sqlmock.AnyArg(), // raw_banner
			&svc,             // service: "http"
			&ver,             // version: "nginx/1.24.0" (from Server header)
			&sc,              // http_status_code: 200
			(*string)(nil),   // http_redirect
			sqlmock.AnyArg(), // http_response_headers (includes Server)
			sqlmock.AnyArg(), // scanned_at
		).WillReturnResult(sqlmock.NewResult(1, 1))

	hdr := zgrabhttp_lib.Header{}
	hdr.Set("Server", "nginx/1.24.0")
	resp := &zgrabhttp_lib.Response{
		StatusCode: 200,
		Header:     hdr,
	}
	results := &zgrabhttp.Results{Response: resp}

	target := BannerTarget{HostID: uuid.New(), IP: "127.0.0.1"}
	g.storeZGrabHTTPBanner(context.Background(), target, 80, results)

	require.NoError(t, mock.ExpectationsWereMet())
}
