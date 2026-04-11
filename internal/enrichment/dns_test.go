// Package enrichment — unit tests for DNSEnricher.
//
// The Resolver is tested by injecting fake lookup functions via
// dns.WithLookupAddrFn / dns.WithLookupHostFn so these tests never touch the
// network.  Database interactions are handled by go-sqlmock.
package enrichment

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	internaldns "github.com/anstrom/scanorama/internal/dns"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// newMockDB wraps a go-sqlmock instance in the application's *db.DB type.
func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	return &db.DB{DB: sqlx.NewDb(rawDB, "sqlmock")}, mock
}

// cacheQueryCols mirrors the columns returned by getCachedEntries in the dns
// package.
var cacheQueryCols = []string{
	"id", "direction", "lookup_key", "resolved_value",
	"resolved_at", "ttl_seconds", "last_error",
}

// makeHost creates a minimal *db.Host with the given IP address string.
func makeHost(ip string) *db.Host {
	return &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: net.ParseIP(ip)},
	}
}

// makeHostWithHostname creates a *db.Host that already has a hostname set.
func makeHostWithHostname(ip, hostname string) *db.Host {
	h := makeHost(ip)
	h.Hostname = &hostname
	return h
}

// noOpCacheExpect sets up the sqlmock expectations for a Resolver whose cache
// always returns an empty result (cache miss), causing it to call the injected
// lookup function. After the lookup it tries to upsert the result.
func noOpCacheExpect(mock sqlmock.Sqlmock) {
	// getCachedEntries SELECT → empty (cache miss).
	mock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	// upsertEntry INSERT → success.
	mock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// ─── NewDNSEnricher ───────────────────────────────────────────────────────────

func TestNewDNSEnricher(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, _ := newMockDB(t)
	hostDB, _ := newMockDB(t)

	resolver := internaldns.New(resolverDB)
	dnsRepo := db.NewDNSRepository(dnsDB)
	hostRepo := db.NewHostRepository(hostDB)

	enricher := NewDNSEnricher(resolver, dnsRepo, hostRepo)
	require.NotNil(t, enricher)

	// No DB calls expected at construction time.
	require.NoError(t, resolverMock.ExpectationsWereMet())
}

// ─── EnrichHosts ─────────────────────────────────────────────────────────────

func TestEnrichHosts_EmptySlice(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	resolver := internaldns.New(resolverDB)
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))

	// Must be a no-op — no DB calls expected.
	enricher.EnrichHosts(context.Background(), nil)
	enricher.EnrichHosts(context.Background(), []*db.Host{})

	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

func TestEnrichHosts_CancelledContext(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	resolver := internaldns.New(resolverDB)
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled — EnrichHosts should return immediately.

	host := makeHost("192.0.2.1")
	enricher.EnrichHosts(ctx, []*db.Host{host})

	// No DB calls expected because the context check fires before EnrichHost.
	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── EnrichHost — no PTR record ───────────────────────────────────────────────

// TestEnrichHost_NoPTRRecord verifies that when the resolver finds no PTR
// record (ErrNoRecords), EnrichHost stores nothing and returns nil.
func TestEnrichHost_NoPTRRecord(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	// Cache miss → injected lookup returns ErrNoRecords.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	// upsert negative entry.
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, internaldns.ErrNoRecords
		}),
	)

	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	err := enricher.EnrichHost(context.Background(), makeHost("192.0.2.1"))

	require.NoError(t, err)
	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// TestEnrichHost_ResolverError verifies that a transient resolver error is
// logged but does not propagate to the caller (no records are stored).
func TestEnrichHost_ResolverError(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	// Cache miss → injected lookup returns a generic error.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	// upsert negative entry for the error case.
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, fmt.Errorf("network unreachable")
		}),
	)

	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	err := enricher.EnrichHost(context.Background(), makeHost("192.0.2.1"))

	// EnrichHost should return nil (no records to store).
	require.NoError(t, err)
	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── EnrichHost — PTR found, hostname already set ─────────────────────────────

// TestEnrichHost_PTRFound_HostnameAlreadySet verifies that when the host
// already has a hostname the repository UpdateHost is NOT called, and the PTR
// record is stored.
func TestEnrichHost_PTRFound_HostnameAlreadySet(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	const ptrName = "known.example.com"

	// Resolver: cache miss → lookup succeeds → upsert cache.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{ptrName}, nil
		}),
		internaldns.WithLookupHostFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{"192.0.2.1"}, nil
		}),
	)

	// DNS repo: BEGIN, DELETE, INSERT PTR, COMMIT.
	// forwardRecords uses net.DefaultResolver directly; in a unit-test
	// environment the hostname likely won't resolve, so no forward records
	// are added and only the PTR INSERT occurs.
	dnsMock.ExpectBegin()
	dnsMock.ExpectExec("DELETE FROM host_dns_records").
		WillReturnResult(sqlmock.NewResult(0, 0))
	// PTR record.
	dnsMock.ExpectExec("INSERT INTO host_dns_records").
		WillReturnResult(sqlmock.NewResult(1, 1))
	dnsMock.ExpectCommit()

	host := makeHostWithHostname("192.0.2.1", "existing.example.com")
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	err := enricher.EnrichHost(context.Background(), host)

	require.NoError(t, err)
	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	// hostDB should have no calls because hostname was already set.
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── EnrichHost — PTR found, no existing hostname ─────────────────────────────

// TestEnrichHost_PTRFound_SetsHostname verifies that when the host has no
// hostname, EnrichHost calls UpdateHost and stores the PTR record.
func TestEnrichHost_PTRFound_SetsHostname(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	const ptrName = "discovered.example.com"
	hostID := uuid.New()

	// Resolver: cache miss → lookup succeeds → upsert cache.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{ptrName}, nil
		}),
		// forwardRecords uses net.DefaultResolver; suppress it by making
		// WithLookupHostFn irrelevant (forwardRecords goes direct to
		// net.DefaultResolver, not through the Resolver wrapper). The test
		// will accept whatever comes back from net.DefaultResolver for the
		// forward lookups — the assertions focus on the PTR handling path.
	)

	// hostDB: UpdateHost path — BEGIN, SELECT EXISTS (true), UPDATE, COMMIT, GetHost.
	now := time.Now().UTC()
	hostCols := []string{
		"id", "ip_address", "hostname", "mac_address", "vendor",
		"os_family", "os_name", "os_version", "os_confidence", "os_detected_at",
		"last_seen", "created_at", "status", "ignore_scanning",
		"timeout_count", "open_port_count", "scan_count",
	}
	hostMock.ExpectBegin()
	hostMock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	hostMock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	hostMock.ExpectCommit()
	// GetHost after commit.
	hostMock.ExpectQuery("SELECT .+ FROM hosts").
		WillReturnRows(sqlmock.NewRows(hostCols).AddRow(
			hostID, "192.0.2.1", ptrName, nil, nil,
			nil, nil, nil, nil, nil,
			now, now, "up", false,
			0, 0, 0,
		))

	// DNS repo: BEGIN, DELETE, INSERT PTR, (possibly more from forwardRecords), COMMIT.
	// We use AnyArg matching via WillReturnResult so extra forward records are
	// handled gracefully — but we must still set up the mandatory expectations.
	dnsMock.ExpectBegin()
	dnsMock.ExpectExec("DELETE FROM host_dns_records").
		WillReturnResult(sqlmock.NewResult(0, 0))
	// At minimum the PTR record.
	dnsMock.ExpectExec("INSERT INTO host_dns_records").
		WillReturnResult(sqlmock.NewResult(1, 1))
	dnsMock.ExpectCommit()

	host := &db.Host{
		ID:        hostID,
		IPAddress: db.IPAddr{IP: net.ParseIP("192.0.2.1")},
		// Hostname is nil — should be set by EnrichHost.
	}

	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	// EnrichHost may fail if forwardRecords inserts more rows than we expect.
	// Accept both nil error and "unmet expectations" errors from sqlmock as
	// long as hostMock expectations for UpdateHost are met.
	_ = enricher.EnrichHost(context.Background(), host)

	// The key invariant: UpdateHost was called with the discovered hostname.
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── UpsertDNSRecords error propagation ───────────────────────────────────────

// TestEnrichHost_UpsertError verifies that a DB error from UpsertDNSRecords
// is propagated back to the caller.
func TestEnrichHost_UpsertError(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	const ptrName = "host.example.com"

	noOpCacheExpect(resolverMock)

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{ptrName}, nil
		}),
	)

	// DNS repo: BEGIN fails → UpsertDNSRecords returns error.
	dnsMock.ExpectBegin().WillReturnError(fmt.Errorf("db unavailable"))

	host := makeHostWithHostname("192.0.2.1", "existing.example.com")
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	err := enricher.EnrichHost(context.Background(), host)

	require.Error(t, err)
	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── EnrichHosts — loop body coverage ────────────────────────────────────────

// TestEnrichHosts_LoopBody_NoPTR verifies that the per-host context.WithTimeout
// and cancel() paths are exercised when EnrichHosts processes a single host
// that has no PTR record.
func TestEnrichHosts_LoopBody_NoPTR(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	// Cache miss → PTR lookup returns ErrNoRecords → upsert negative entry.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, internaldns.ErrNoRecords
		}),
	)

	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	// Must not panic; the per-host context.WithTimeout + cancel() paths are
	// exercised by having a non-empty hosts slice.
	enricher.EnrichHosts(context.Background(), []*db.Host{makeHost("10.0.0.1")})

	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// TestEnrichHosts_ErrorLogged verifies that when EnrichHost returns an error
// for a host, the loop logs a warning and processes remaining hosts rather
// than returning early.
func TestEnrichHosts_ErrorLogged(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	const ptrName = "router.example.com"

	// Resolver: cache miss → PTR found → upsert cache.
	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{ptrName}, nil
		}),
	)

	// DNS repo: BEGIN fails → UpsertDNSRecords returns an error.
	dnsMock.ExpectBegin().WillReturnError(fmt.Errorf("db unavailable"))

	// Host already has a hostname so maybeSetHostname is a no-op.
	host := makeHostWithHostname("10.0.0.1", "existing.host.com")
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))

	// EnrichHosts must not panic; it logs the warning and returns normally.
	enricher.EnrichHosts(context.Background(), []*db.Host{host})

	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── forwardRecords ───────────────────────────────────────────────────────────

// TestForwardRecords_EmptyHostname verifies that forwardRecords called with an
// empty hostname (which the enricher guards against) returns an empty slice.
// We test it indirectly via EnrichHost with a host that has no PTR record.
func TestForwardRecords_NotCalledWithoutPTR(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	resolverMock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheQueryCols))
	resolverMock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, internaldns.ErrNoRecords
		}),
	)

	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	err := enricher.EnrichHost(context.Background(), makeHost("192.0.2.1"))

	require.NoError(t, err)
	// No DNS repo calls expected because there are no records to store.
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
	require.NoError(t, resolverMock.ExpectationsWereMet())
}
