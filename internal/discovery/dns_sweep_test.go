package discovery

import (
	"context"
	"net"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	internaldns "github.com/anstrom/scanorama/internal/dns"
)

// newMockResolver creates a Resolver backed by an empty sqlmock DB and applies
// the given options (typically WithLookupAddrFn / WithLookupHostFn).
func newMockResolver(t *testing.T, opts ...internaldns.Option) (*internaldns.Resolver, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(rawDB, "sqlmock")}
	return internaldns.New(database, opts...), mock
}

// cacheColumns matches the columns returned by the dns_cache SELECT.
var cacheColumns = []string{
	"id", "direction", "lookup_key", "resolved_value",
	"resolved_at", "ttl_seconds", "last_error",
}

// expectCacheMissAndUpsert sets up sqlmock expectations for one Resolver
// lookup that misses the cache and writes the result back.
func expectCacheMissAndUpsert(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .+ FROM dns_cache WHERE direction").
		WillReturnRows(sqlmock.NewRows(cacheColumns))
	mock.ExpectExec("INSERT INTO dns_cache").
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// ─── enumerateIPs ─────────────────────────────────────────────────────────────

func TestEnumerateIPs_SmallSubnet(t *testing.T) {
	// /30 → 2 usable host IPs (.1 and .2)
	ipnet := mustParseCIDR("192.168.1.0/30")
	ips := enumerateIPs(ipnet, 0)
	require.Len(t, ips, 2)
	assert.Equal(t, "192.168.1.1", ips[0].String())
	assert.Equal(t, "192.168.1.2", ips[1].String())
}

func TestEnumerateIPs_Slash29(t *testing.T) {
	// /29 → 6 usable host IPs
	ipnet := mustParseCIDR("10.0.0.0/29")
	ips := enumerateIPs(ipnet, 0)
	require.Len(t, ips, 6)
	assert.Equal(t, "10.0.0.1", ips[0].String())
	assert.Equal(t, "10.0.0.6", ips[len(ips)-1].String())
}

func TestEnumerateIPs_MaxHosts(t *testing.T) {
	// /24 has 254 usable IPs; cap at 10.
	ipnet := mustParseCIDR("10.0.0.0/24")
	ips := enumerateIPs(ipnet, 10)
	assert.Len(t, ips, 10)
	assert.Equal(t, "10.0.0.1", ips[0].String())
	assert.Equal(t, "10.0.0.10", ips[9].String())
}

func TestEnumerateIPs_Slash32(t *testing.T) {
	// /32 has no usable host IPs (network == host == broadcast).
	ipnet := mustParseCIDR("192.0.2.1/32")
	ips := enumerateIPs(ipnet, 0)
	assert.Empty(t, ips, "a /32 should yield no usable host IPs")
}

func TestEnumerateIPs_Slash31(t *testing.T) {
	// RFC 3021 /31 — both addresses are host addresses, no broadcast.
	// Our implementation skips the first address (network) and stops before
	// the last (treated as broadcast), so returns 0 IPs.
	ipnet := mustParseCIDR("10.0.0.0/31")
	ips := enumerateIPs(ipnet, 0)
	// Both addresses are consumed by the skip-network / skip-broadcast logic.
	assert.Empty(t, ips)
}

// ─── dnsSweep ─────────────────────────────────────────────────────────────────

func TestDNSSweep_AllIPsResolved(t *testing.T) {
	// /30 → 2 usable IPs; both resolve to PTR names.
	ipnet := mustParseCIDR("192.168.1.0/30")

	resolver, mock := newMockResolver(t,
		internaldns.WithLookupAddrFn(func(_ context.Context, ip string) ([]string, error) {
			return []string{ip + ".example.com"}, nil
		}),
	)
	// Two IPs → two cache-miss+upsert cycles.
	expectCacheMissAndUpsert(mock)
	expectCacheMissAndUpsert(mock)

	results := dnsSweep(context.Background(), ipnet, resolver, 0)

	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, "up", r.Status)
		assert.Equal(t, "dns", r.Method)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSSweep_NoIPsResolved(t *testing.T) {
	// /30 → 2 usable IPs; none have PTR records.
	ipnet := mustParseCIDR("192.168.1.0/30")

	resolver, mock := newMockResolver(t,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return nil, internaldns.ErrNoRecords
		}),
	)
	// Two cache misses — negative results are also cached.
	expectCacheMissAndUpsert(mock)
	expectCacheMissAndUpsert(mock)

	results := dnsSweep(context.Background(), ipnet, resolver, 0)

	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSSweep_PartialResolution(t *testing.T) {
	// /30 → 2 IPs; only .1 resolves.
	ipnet := mustParseCIDR("192.168.1.0/30")

	callCount := 0
	resolver, mock := newMockResolver(t,
		internaldns.WithLookupAddrFn(func(_ context.Context, ip string) ([]string, error) {
			callCount++
			if ip == "192.168.1.1" {
				return []string{"host1.example.com"}, nil
			}
			return nil, internaldns.ErrNoRecords
		}),
	)
	expectCacheMissAndUpsert(mock)
	expectCacheMissAndUpsert(mock)

	results := dnsSweep(context.Background(), ipnet, resolver, 0)

	require.Len(t, results, 1)
	assert.Equal(t, net.ParseIP("192.168.1.1").String(), results[0].IPAddress.String())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSSweep_ContextCancelled(t *testing.T) {
	// A cancelled context should cause the sweep to return immediately.
	ipnet := mustParseCIDR("10.0.0.0/24")

	resolver, _ := newMockResolver(t,
		internaldns.WithLookupAddrFn(func(_ context.Context, _ string) ([]string, error) {
			return []string{"host.example.com"}, nil
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	results := dnsSweep(ctx, ipnet, resolver, 0)
	// With a pre-cancelled context the loop exits on the first iteration check.
	assert.Empty(t, results)
}

func TestDNSSweep_EmptyNetwork(t *testing.T) {
	// /32 has no usable IPs → dnsSweep should return nil immediately.
	ipnet := mustParseCIDR("192.0.2.1/32")
	resolver, mock := newMockResolver(t)

	results := dnsSweep(context.Background(), ipnet, resolver, 0)

	assert.Nil(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}
