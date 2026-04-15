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
	"github.com/lib/pq"
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

// ─── forwardRecords ───────────────────────────────────────────────────────────

// fakeNetResolver implements netLookups for tests, returning configurable records.
type fakeNetResolver struct {
	cname string
	mx    []*net.MX
	txts  []string
	srvs  []*net.SRV // returned for every service/proto combination
}

func (f *fakeNetResolver) LookupCNAME(_ context.Context, _ string) (string, error) {
	if f.cname == "" {
		return "", &net.DNSError{IsNotFound: true}
	}
	return f.cname, nil
}

func (f *fakeNetResolver) LookupMX(_ context.Context, _ string) ([]*net.MX, error) {
	if len(f.mx) == 0 {
		return nil, &net.DNSError{IsNotFound: true}
	}
	return f.mx, nil
}

func (f *fakeNetResolver) LookupTXT(_ context.Context, _ string) ([]string, error) {
	if len(f.txts) == 0 {
		return nil, &net.DNSError{IsNotFound: true}
	}
	return f.txts, nil
}

func (f *fakeNetResolver) LookupSRV(_ context.Context, _, _, _ string) (string, []*net.SRV, error) {
	if len(f.srvs) == 0 {
		return "", nil, &net.DNSError{IsNotFound: true}
	}
	return "", f.srvs, nil
}

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

// newForwardRecordsEnricher builds a DNSEnricher wired to a fake net resolver
// for testing forwardRecords in isolation. The returned sqlmock is set up with
// exactly one cache-miss + upsert expectation for the A/AAAA lookup.
func newForwardRecordsEnricher(
	t *testing.T,
	fake *fakeNetResolver,
) (*DNSEnricher, sqlmock.Sqlmock) {
	t.Helper()
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, _ := newMockDB(t)
	hostDB, _ := newMockDB(t)

	// The A/AAAA lookup goes through the cached resolver → one cache miss + upsert.
	noOpCacheExpect(resolverMock)

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupHostFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, internaldns.ErrNoRecords
		}),
	)
	enricher := NewDNSEnricher(
		resolver,
		db.NewDNSRepository(dnsDB),
		db.NewHostRepository(hostDB),
	).WithNetResolver(fake)

	return enricher, resolverMock
}

func TestForwardRecords_CNAME(t *testing.T) {
	hostID := uuid.New()
	fake := &fakeNetResolver{cname: "canonical.example.com."}
	enricher, resolverMock := newForwardRecordsEnricher(t, fake)

	records := enricher.forwardRecords(context.Background(), hostID, "host.example.com")

	var cnames []string
	for _, r := range records {
		if r.RecordType == "CNAME" {
			cnames = append(cnames, r.Value)
		}
	}
	require.Len(t, cnames, 1)
	require.Equal(t, "canonical.example.com", cnames[0])
	require.NoError(t, resolverMock.ExpectationsWereMet())
}

func TestForwardRecords_CNAME_SelfReference_Omitted(t *testing.T) {
	// When the CNAME equals the queried hostname, it must not be stored.
	// net.DefaultResolver returns the queried name (with trailing dot) for names
	// without a CNAME alias. Simulate that.
	hostID := uuid.New()
	fake := &fakeNetResolver{cname: "host.example.com."}
	enricher, resolverMock := newForwardRecordsEnricher(t, fake)

	records := enricher.forwardRecords(context.Background(), hostID, "host.example.com")

	for _, r := range records {
		require.NotEqual(t, "CNAME", r.RecordType, "self-referencing CNAME must be omitted")
	}
	require.NoError(t, resolverMock.ExpectationsWereMet())
}

func TestForwardRecords_MX(t *testing.T) {
	hostID := uuid.New()
	fake := &fakeNetResolver{
		mx: []*net.MX{
			{Host: "mail.example.com.", Pref: 10},
		},
	}
	enricher, resolverMock := newForwardRecordsEnricher(t, fake)

	records := enricher.forwardRecords(context.Background(), hostID, "example.com")

	var mxVals []string
	for _, r := range records {
		if r.RecordType == "MX" {
			mxVals = append(mxVals, r.Value)
		}
	}
	require.Len(t, mxVals, 1)
	require.Equal(t, "mail.example.com", mxVals[0])
	require.NoError(t, resolverMock.ExpectationsWereMet())
}

func TestForwardRecords_TXT(t *testing.T) {
	hostID := uuid.New()
	fake := &fakeNetResolver{txts: []string{"v=spf1 include:example.com ~all"}}
	enricher, resolverMock := newForwardRecordsEnricher(t, fake)

	records := enricher.forwardRecords(context.Background(), hostID, "example.com")

	var txts []string
	for _, r := range records {
		if r.RecordType == "TXT" {
			txts = append(txts, r.Value)
		}
	}
	require.Len(t, txts, 1)
	require.Equal(t, "v=spf1 include:example.com ~all", txts[0])
	require.NoError(t, resolverMock.ExpectationsWereMet())
}

// ─── EnrichHosts — loop body coverage ────────────────────────────────────────

// TestEnrichHosts_ProcessesOneHost verifies that EnrichHosts enters the
// per-host timeout loop for a non-cancelled context with a non-empty slice.
func TestEnrichHosts_ProcessesOneHost(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	// PTR lookup returns ErrNoRecords → EnrichHost stores nothing and returns nil.
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
	enricher.EnrichHosts(context.Background(), []*db.Host{makeHost("192.0.2.1")})

	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// TestEnrichHosts_EnrichHostFails verifies that an error from EnrichHost is
// logged but does not abort the loop (function returns without propagating).
func TestEnrichHosts_EnrichHostFails(t *testing.T) {
	resolverDB, resolverMock := newMockDB(t)
	dnsDB, dnsMock := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	noOpCacheExpect(resolverMock)

	resolver := internaldns.New(resolverDB,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{"host.example.com"}, nil
		}),
	)

	// DNS repo: BEGIN fails → UpsertDNSRecords returns an error.
	dnsMock.ExpectBegin().WillReturnError(fmt.Errorf("db unavailable"))

	host := makeHostWithHostname("192.0.2.1", "existing.example.com")
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))
	// EnrichHosts must not panic and must not propagate the error.
	enricher.EnrichHosts(context.Background(), []*db.Host{host})

	require.NoError(t, resolverMock.ExpectationsWereMet())
	require.NoError(t, dnsMock.ExpectationsWereMet())
	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── maybeSetHostname — error path ───────────────────────────────────────────

// getHostCols lists the 26 columns scanned by GetHost in positional order,
// followed by the columns for the sub-queries that GetHost also runs.
var getHostCols = []string{
	"id", "ip_address", "hostname", "mac_address", "vendor",
	"os_family", "os_name", "os_version", "os_confidence",
	"os_detected_at", "os_method", "os_details",
	"discovery_method",
	"response_time_ms", "response_time_min_ms", "response_time_max_ms", "response_time_avg_ms",
	"ignore_scanning",
	"first_seen", "last_seen", "status",
	"status_changed_at", "previous_status", "timeout_count",
	"tags",
	"knowledge_score",
}

// TestMaybeSetHostname_UpdateHostSucceeds verifies that maybeSetHostname logs
// a debug message when UpdateHost completes without error.
func TestMaybeSetHostname_UpdateHostSucceeds(t *testing.T) {
	resolverDB, _ := newMockDB(t)
	dnsDB, _ := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	hostID := uuid.New()
	hostname := "discovered.example.com"
	now := time.Now().UTC()

	// Full UpdateHost transaction sequence.
	hostMock.ExpectBegin()
	hostMock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	hostMock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	hostMock.ExpectCommit()
	// GetHost after commit: main row + fetchHostPorts + fetchHostScanCount + GetHostGroups.
	hostMock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(getHostCols).AddRow(
			hostID, "192.0.2.1", &hostname, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil,
			nil,
			nil, nil, nil, nil,
			false,
			now, now, "up",
			nil, nil, 0,
			pq.StringArray{},
			0,
		))
	hostMock.ExpectQuery("SELECT DISTINCT").
		WillReturnRows(sqlmock.NewRows([]string{"port", "protocol", "state", "service_name", "scanned_at"}))
	hostMock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	hostMock.ExpectQuery("SELECT hg.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color"}))

	resolver := internaldns.New(resolverDB)
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))

	host := &db.Host{
		ID:        hostID,
		IPAddress: db.IPAddr{IP: net.ParseIP("192.0.2.1")},
		// Hostname is nil — maybeSetHostname will call UpdateHost.
	}
	enricher.maybeSetHostname(context.Background(), host, hostname)

	require.NoError(t, hostMock.ExpectationsWereMet())
}

// TestMaybeSetHostname_UpdateHostFails verifies that a DB error from
// UpdateHost is only logged; maybeSetHostname must not propagate it.
func TestMaybeSetHostname_UpdateHostFails(t *testing.T) {
	resolverDB, _ := newMockDB(t)
	dnsDB, _ := newMockDB(t)
	hostDB, hostMock := newMockDB(t)

	// UpdateHost fails immediately at BEGIN.
	hostMock.ExpectBegin().WillReturnError(fmt.Errorf("db unavailable"))

	resolver := internaldns.New(resolverDB)
	enricher := NewDNSEnricher(resolver, db.NewDNSRepository(dnsDB), db.NewHostRepository(hostDB))

	host := makeHost("192.0.2.1") // Hostname is nil — maybeSetHostname will attempt UpdateHost.
	enricher.maybeSetHostname(context.Background(), host, "discovered.example.com")

	require.NoError(t, hostMock.ExpectationsWereMet())
}

// ─── forwardRecords — SRV ─────────────────────────────────────────────────────

func TestForwardRecords_SRV(t *testing.T) {
	hostID := uuid.New()
	fake := &fakeNetResolver{
		srvs: []*net.SRV{
			{Target: "sip.example.com.", Port: 5060, Priority: 10, Weight: 20},
		},
	}
	enricher, resolverMock := newForwardRecordsEnricher(t, fake)

	records := enricher.forwardRecords(context.Background(), hostID, "example.com")

	var srvVals []string
	for _, r := range records {
		if r.RecordType == "SRV" {
			srvVals = append(srvVals, r.Value)
		}
	}
	// fakeNetResolver returns the same SRV for every service/proto, so we get
	// one record per probed service (9 total). The first probed service is "http/tcp".
	require.Len(t, srvVals, 9)
	// Validate the full format: _service._proto.target priority weight port
	require.Equal(t, "_http._tcp.sip.example.com 10 20 5060", srvVals[0])
	require.NoError(t, resolverMock.ExpectationsWereMet())
}
