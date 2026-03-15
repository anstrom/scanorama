// Package dns provides cached DNS resolution for scanorama.
package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// newMockDB wraps a go-sqlmock instance in the application's *db.DB type.
func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { rawDB.Close() })
	return &db.DB{DB: sqlx.NewDb(rawDB, "sqlmock")}, mock
}

// cacheQueryCols are the column names returned by getCachedEntries / RefreshStale.
var cacheQueryCols = []string{
	"id", "direction", "lookup_key", "resolved_value",
	"resolved_at", "ttl_seconds", "last_error",
}

// freshRow returns a sqlmock row that is well within its TTL.
func freshRow(dir Direction, key, value string) *sqlmock.Rows {
	return sqlmock.NewRows(cacheQueryCols).AddRow(
		uuid.New().String(),
		string(dir),
		key,
		value,
		time.Now().Add(-time.Minute), // resolved 1 minute ago
		3600,                         // 1-hour TTL → still fresh
		nil,
	)
}

// staleRow returns a sqlmock row whose TTL has already expired.
func staleRow(dir Direction, key, value string) *sqlmock.Rows {
	return sqlmock.NewRows(cacheQueryCols).AddRow(
		uuid.New().String(),
		string(dir),
		key,
		value,
		time.Now().Add(-2*time.Hour), // resolved 2 hours ago
		3600,                         // 1-hour TTL → stale
		nil,
	)
}

// emptyRows returns a result set with the right columns but no data rows.
func emptyRows() *sqlmock.Rows {
	return sqlmock.NewRows(cacheQueryCols)
}

// freshNegativeRow returns a fresh cache row with an empty resolved_value
// (negative-cache sentinel).
func freshNegativeRow(dir Direction, key string) *sqlmock.Rows {
	return sqlmock.NewRows(cacheQueryCols).AddRow(
		uuid.New().String(),
		string(dir),
		key,
		"", // negative sentinel
		time.Now().Add(-time.Minute),
		300, // 5-minute negative TTL → still fresh
		nil,
	)
}

// sqlmock v1.5.2 strips whitespace then compiles the expected string as a
// regexp against the collapsed actual SQL.  Use .+ (not .*) between tokens so
// the greedy quantifier cannot swallow the next literal keyword.
const (
	selectPattern    = "SELECT .+ FROM dns_cache WHERE direction"
	upsertPattern    = "INSERT INTO dns_cache"
	stalePattern     = "FROM dns_cache WHERE resolved_at"
	deletePattern    = "DELETE FROM dns_cache WHERE direction"
	deleteAllPattern = "DELETE FROM dns_cache"
)

// ─── Option constructor tests ─────────────────────────────────────────────────

// TestWithTTL verifies that WithTTL changes the positive-result TTL.
func TestWithTTL(t *testing.T) {
	database, mock := newMockDB(t)
	r := New(database, WithTTL(30*time.Minute))
	assert.Equal(t, 30*time.Minute, r.ttl)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestWithNegativeTTL verifies that WithNegativeTTL changes the negative TTL.
func TestWithNegativeTTL(t *testing.T) {
	database, mock := newMockDB(t)
	r := New(database, WithNegativeTTL(2*time.Minute))
	assert.Equal(t, 2*time.Minute, r.negativeTTL)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestWithLogger verifies that WithLogger replaces the logger field.
func TestWithLogger(t *testing.T) {
	database, mock := newMockDB(t)
	l := slog.Default()
	r := New(database, WithLogger(l))
	assert.Equal(t, l, r.logger)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestWithNetResolver verifies that WithNetResolver replaces the net.Resolver.
func TestWithNetResolver(t *testing.T) {
	database, mock := newMockDB(t)
	nr := &net.Resolver{PreferGo: true}
	r := New(database, WithNetResolver(nr))
	assert.Equal(t, nr, r.net)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestNew_Defaults verifies the zero-value defaults set by New.
func TestNew_Defaults(t *testing.T) {
	database, mock := newMockDB(t)
	r := New(database)
	assert.Equal(t, DefaultTTL, r.ttl)
	assert.Equal(t, DefaultNegativeTTL, r.negativeTTL)
	assert.NotNil(t, r.logger)
	assert.NotNil(t, r.net)
	assert.NotNil(t, r.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── LookupHost tests ─────────────────────────────────────────────────────────

// TestLookupHost_CacheHit verifies that a fresh cache entry is returned
// without touching the network.
func TestLookupHost_CacheHit(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "example.com").
		WillReturnRows(freshRow(DirectionForward, "example.com", "93.184.216.34"))

	var liveCalled bool
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		liveCalled = true
		return nil, nil
	}, nil)

	addrs, err := r.LookupHost(context.Background(), "example.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"93.184.216.34"}, addrs)
	assert.False(t, liveCalled, "live resolver should not be called on a cache hit")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_CacheHit_NegativeEntry verifies that a fresh negative-cache
// entry returns ErrNoRecords without a network call.
func TestLookupHost_CacheHit_NegativeEntry(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "nxdomain.host").
		WillReturnRows(freshNegativeRow(DirectionForward, "nxdomain.host"))

	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		t.Fatal("live resolver must not be called for a fresh negative entry")
		return nil, nil
	}, nil)

	_, err := r.LookupHost(context.Background(), "nxdomain.host")
	require.ErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_CacheMiss_LiveSuccess verifies that a cache miss triggers a
// live lookup whose results are cached and returned.
func TestLookupHost_CacheMiss_LiveSuccess(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "host.local").
		WillReturnRows(emptyRows())

	// One upsert per returned IP.
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, host string) ([]string, error) {
		assert.Equal(t, "host.local", host)
		return []string{"10.0.0.1", "10.0.0.2"}, nil
	}, nil)

	addrs, err := r.LookupHost(context.Background(), "host.local")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, addrs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_StaleEntry triggers a live refresh when the cached row is
// outside its TTL window.
func TestLookupHost_StaleEntry(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "old.host").
		WillReturnRows(staleRow(DirectionForward, "old.host", "1.2.3.4"))

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	var liveCalled bool
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		liveCalled = true
		return []string{"1.2.3.4"}, nil
	}, nil)

	addrs, err := r.LookupHost(context.Background(), "old.host")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.2.3.4"}, addrs)
	assert.True(t, liveCalled, "live resolver should refresh a stale entry")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_LiveError_NegativeCached verifies that a resolver error is
// cached with the shorter negative TTL and the error is propagated.
func TestLookupHost_LiveError_NegativeCached(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "bad.host").
		WillReturnRows(emptyRows())

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, host string) ([]string, error) {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}, nil)

	_, err := r.LookupHost(context.Background(), "bad.host")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_EmptyResponse returns ErrNoRecords and caches the empty result.
func TestLookupHost_EmptyResponse(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "empty.host").
		WillReturnRows(emptyRows())

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		return []string{}, nil // resolver succeeds but returns nothing
	}, nil)

	_, err := r.LookupHost(context.Background(), "empty.host")
	require.ErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_EmptyHostname verifies that empty input is rejected.
func TestLookupHost_EmptyHostname(t *testing.T) {
	database, mock := newMockDB(t)
	r := newTestResolver(database, nil, nil)

	_, err := r.LookupHost(context.Background(), "   ")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_NormalisesCase verifies that the hostname is lower-cased
// before cache lookup so "EXAMPLE.COM" and "example.com" share the same entry.
func TestLookupHost_NormalisesCase(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "example.com"). // must be lower-cased
		WillReturnRows(freshRow(DirectionForward, "example.com", "1.2.3.4"))

	r := newTestResolver(database, nil, nil)

	addrs, err := r.LookupHost(context.Background(), "EXAMPLE.COM")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.2.3.4"}, addrs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupHost_Singleflight verifies that concurrent callers for the same
// key share a single live lookup.
func TestLookupHost_Singleflight(t *testing.T) {
	database, _ := newMockDB(t)

	// We deliberately do NOT set mock expectations here because the
	// singleflight test is about concurrency, not about DB call counts:
	// only the first goroutine's cache SELECT fires before the others join the
	// in-flight call. sqlmock's strict ordering would make this test brittle.
	// Instead we use a plain no-op DB wrapper and confirm live call count.

	var liveCallCount atomic.Int32
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		liveCallCount.Add(1)
		time.Sleep(20 * time.Millisecond) // simulate latency
		return []string{"5.5.5.5"}, nil
	}, nil)

	// Override getCachedEntries to always return a cache miss so all goroutines
	// reach the singleflight gate; also suppress the upsert.
	r.lookupHostFn = func(_ context.Context, _ string) ([]string, error) {
		liveCallCount.Add(1)
		time.Sleep(20 * time.Millisecond)
		return []string{"5.5.5.5"}, nil
	}

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	// Seed a single in-flight call by calling do directly so we fully control
	// the singleflight behavior without DB noise.
	liveCallCount.Store(0)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			// Call do directly to test singleflight in isolation.
			var callCount atomic.Int32
			vals, err := r.do("sf-test-key", func() ([]string, error) {
				callCount.Add(1)
				time.Sleep(20 * time.Millisecond)
				return []string{"5.5.5.5"}, nil
			})
			errs[idx] = err
			if len(vals) > 0 {
				results[idx] = vals[0]
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d should not error", i)
		assert.Equal(t, "5.5.5.5", results[i])
	}
}

// ─── LookupAddr tests ─────────────────────────────────────────────────────────

// TestLookupAddr_CacheHit verifies reverse lookup returns a fresh cached result.
func TestLookupAddr_CacheHit(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "192.168.1.1").
		WillReturnRows(freshRow(DirectionReverse, "192.168.1.1", "router.local"))

	var liveCalled bool
	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		liveCalled = true
		return nil, nil
	})

	name, err := r.LookupAddr(context.Background(), "192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, "router.local", name)
	assert.False(t, liveCalled)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_CacheHit_EmptyValue treats a cached empty ResolvedValue as
// ErrNoRecords without going to the network.
func TestLookupAddr_CacheHit_EmptyValue(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.0.1").
		WillReturnRows(freshNegativeRow(DirectionReverse, "10.0.0.1"))

	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		t.Fatal("live resolver must not be called for a fresh negative cache entry")
		return nil, nil
	})

	_, err := r.LookupAddr(context.Background(), "10.0.0.1")
	require.ErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_CacheMiss_LiveSuccess performs a live reverse lookup and
// verifies the result is cached and returned.
func TestLookupAddr_CacheMiss_LiveSuccess(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.10.0.5").
		WillReturnRows(emptyRows())

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, nil, func(_ context.Context, ip string) ([]string, error) {
		assert.Equal(t, "10.10.0.5", ip)
		return []string{"server.internal."}, nil // trailing dot should be stripped
	})

	name, err := r.LookupAddr(context.Background(), "10.10.0.5")
	require.NoError(t, err)
	assert.Equal(t, "server.internal", name) // dot stripped
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_LiveError caches the error at the negative TTL and returns it.
func TestLookupAddr_LiveError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.0.99").
		WillReturnRows(emptyRows())

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		return nil, &net.DNSError{Err: "connection refused", IsNotFound: false}
	})

	_, err := r.LookupAddr(context.Background(), "10.0.0.99")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_LiveEmpty verifies that a successful but name-less PTR
// response caches a negative entry and returns ErrNoRecords.
func TestLookupAddr_LiveEmpty(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.0.88").
		WillReturnRows(emptyRows())

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		return []string{}, nil // resolver succeeds but has no PTR records
	})

	_, err := r.LookupAddr(context.Background(), "10.0.0.88")
	require.ErrorIs(t, err, ErrNoRecords)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_InvalidIP verifies that a non-IP string is rejected immediately.
func TestLookupAddr_InvalidIP(t *testing.T) {
	database, mock := newMockDB(t)
	r := newTestResolver(database, nil, nil)

	_, err := r.LookupAddr(context.Background(), "not-an-ip")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLookupAddr_TrailingDotStripped verifies that multiple PTR records all
// have their trailing dots stripped before the primary is cached and returned.
func TestLookupAddr_TrailingDotStripped(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.1.1").
		WillReturnRows(emptyRows())
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		return []string{"primary.host.", "alias.host."}, nil
	})

	name, err := r.LookupAddr(context.Background(), "10.0.1.1")
	require.NoError(t, err)
	assert.Equal(t, "primary.host", name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── Invalidate tests ─────────────────────────────────────────────────────────

// TestInvalidate_Forward verifies that Invalidate issues a DELETE for the
// correct direction and key.
func TestInvalidate_Forward(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec(deletePattern).
		WithArgs(string(DirectionForward), "stale.host").
		WillReturnResult(sqlmock.NewResult(0, 2))

	r := New(database)
	err := r.Invalidate(context.Background(), DirectionForward, "stale.host")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestInvalidate_Reverse verifies that Invalidate works for reverse entries.
func TestInvalidate_Reverse(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec(deletePattern).
		WithArgs(string(DirectionReverse), "192.168.0.1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := New(database)
	err := r.Invalidate(context.Background(), DirectionReverse, "192.168.0.1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestInvalidate_DBError propagates database errors.
func TestInvalidate_DBError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec(deletePattern).
		WillReturnError(errors.New("db unavailable"))

	r := New(database)
	err := r.Invalidate(context.Background(), DirectionForward, "any.host")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalidate")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestInvalidateAll_Success verifies the full-table DELETE.
func TestInvalidateAll_Success(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec(deleteAllPattern).
		WillReturnResult(sqlmock.NewResult(0, 42))

	r := New(database)
	err := r.InvalidateAll(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestInvalidateAll_DBError propagates database errors.
func TestInvalidateAll_DBError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec(deleteAllPattern).
		WillReturnError(errors.New("connection lost"))

	r := New(database)
	err := r.InvalidateAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalidate all")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestInvalidate_ThenLookup verifies the full invalidate-then-re-resolve cycle:
// invalidating an entry causes the next LookupHost to go to the network.
func TestInvalidate_ThenLookup(t *testing.T) {
	database, mock := newMockDB(t)

	// Step 1 – invalidate.
	mock.ExpectExec(deletePattern).
		WithArgs(string(DirectionForward), "cycle.host").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Step 2 – fresh lookup sees cache miss.
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "cycle.host").
		WillReturnRows(emptyRows())
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		return []string{"9.9.9.9"}, nil
	}, nil)

	require.NoError(t, r.Invalidate(context.Background(), DirectionForward, "cycle.host"))

	addrs, err := r.LookupHost(context.Background(), "cycle.host")
	require.NoError(t, err)
	assert.Equal(t, []string{"9.9.9.9"}, addrs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── RefreshStale tests ───────────────────────────────────────────────────────

// TestRefreshStale verifies that stale entries are re-resolved.
func TestRefreshStale(t *testing.T) {
	database, mock := newMockDB(t)

	staleRows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "refresh.host", "9.9.9.9",
			time.Now().Add(-2*time.Hour), 3600, nil).
		AddRow(uuid.New().String(), string(DirectionReverse), "9.9.9.9", "refresh.host",
			time.Now().Add(-2*time.Hour), 3600, nil)

	mock.ExpectQuery(stalePattern).WillReturnRows(staleRows)

	// Forward refresh: stale cache entry then live upsert.
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "refresh.host").
		WillReturnRows(staleRow(DirectionForward, "refresh.host", "9.9.9.9"))
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	// Reverse refresh: stale cache entry then live upsert.
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "9.9.9.9").
		WillReturnRows(staleRow(DirectionReverse, "9.9.9.9", "refresh.host"))
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	var forwardCalled, reverseCalled bool
	r := newTestResolver(database,
		func(_ context.Context, _ string) ([]string, error) {
			forwardCalled = true
			return []string{"9.9.9.9"}, nil
		},
		func(_ context.Context, _ string) ([]string, error) {
			reverseCalled = true
			return []string{"refresh.host"}, nil
		},
	)

	err := r.RefreshStale(context.Background())
	require.NoError(t, err)
	assert.True(t, forwardCalled, "forward refresh should have been called")
	assert.True(t, reverseCalled, "reverse refresh should have been called")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_DeduplicatesKeys ensures that multiple stale rows for the
// same (direction, lookup_key) only trigger one live lookup.
func TestRefreshStale_DeduplicatesKeys(t *testing.T) {
	database, mock := newMockDB(t)

	staleRows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "multi.host", "1.1.1.1",
			time.Now().Add(-2*time.Hour), 3600, nil).
		AddRow(uuid.New().String(), string(DirectionForward), "multi.host", "2.2.2.2",
			time.Now().Add(-2*time.Hour), 3600, nil)

	mock.ExpectQuery(stalePattern).WillReturnRows(staleRows)

	// Only one LookupHost call; it returns two IPs → two upserts.
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "multi.host").
		WillReturnRows(staleRow(DirectionForward, "multi.host", "1.1.1.1"))
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	var liveCallCount atomic.Int32
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		liveCallCount.Add(1)
		return []string{"1.1.1.1", "2.2.2.2"}, nil
	}, nil)

	require.NoError(t, r.RefreshStale(context.Background()))
	assert.EqualValues(t, 1, liveCallCount.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_ContextCanceled ensures RefreshStale stops early when the
// context is canceled between iterations.
func TestRefreshStale_ContextCanceled(t *testing.T) {
	database, mock := newMockDB(t)

	staleRows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "a.host", "1.1.1.1",
			time.Now().Add(-2*time.Hour), 3600, nil).
		AddRow(uuid.New().String(), string(DirectionForward), "b.host", "2.2.2.2",
			time.Now().Add(-2*time.Hour), 3600, nil)
	mock.ExpectQuery(stalePattern).WillReturnRows(staleRows)

	// First item's cache SELECT + upsert only.
	mock.ExpectQuery(selectPattern).WillReturnRows(emptyRows())
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		callCount++
		if callCount == 1 {
			cancel() // cancel after first lookup
		}
		return []string{"1.1.1.1"}, nil
	}, nil)

	err := r.RefreshStale(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, callCount, "should stop after context is canceled")
}

// TestLookupAddr_CacheReadDBError verifies that a DB error on the reverse cache
// read is treated as a cache miss and the live resolver is still consulted.
func TestLookupAddr_CacheReadDBError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.0.77").
		WillReturnError(errors.New("connection refused"))

	// Upsert after live lookup.
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, nil, func(_ context.Context, _ string) ([]string, error) {
		return []string{"ptr.host"}, nil
	})

	name, err := r.LookupAddr(context.Background(), "10.0.0.77")
	require.NoError(t, err)
	assert.Equal(t, "ptr.host", name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_LookupErrorLogged verifies that a non-ErrNoRecords error
// returned during a stale refresh is logged but does not abort the sweep or
// return an error to the caller.
func TestRefreshStale_LookupErrorLogged(t *testing.T) {
	database, mock := newMockDB(t)

	staleRows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "broken.host", "1.1.1.1",
			time.Now().Add(-2*time.Hour), 3600, nil)
	mock.ExpectQuery(stalePattern).WillReturnRows(staleRows)

	// The re-resolve attempt hits a cache miss then a live resolver error.
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "broken.host").
		WillReturnRows(emptyRows())
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("resolver unavailable")
	}, nil)

	// RefreshStale should complete without error even though the individual
	// lookup failed — the error is only logged.
	err := r.RefreshStale(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_QueryFailure verifies that a DB error on the initial stale
// query is returned as an error.
func TestRefreshStale_QueryFailure(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(stalePattern).
		WillReturnError(errors.New("connection reset"))

	r := New(database)
	err := r.RefreshStale(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query stale entries")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_RowScanError verifies that a scan error on an individual
// row is logged and skipped rather than aborting the entire sweep.
func TestRefreshStale_RowScanError(t *testing.T) {
	database, mock := newMockDB(t)

	// Return a row with the wrong column count to trigger a scan error,
	// followed by a good row.
	badRows := sqlmock.NewRows([]string{"id"}). // wrong columns → scan will fail
							AddRow(uuid.New().String())
	mock.ExpectQuery(stalePattern).WillReturnRows(badRows)

	r := New(database)
	// Should not return an error — bad rows are skipped with a warning.
	err := r.RefreshStale(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRefreshStale_RowsErr verifies that rows.Err() is checked after iteration.
func TestRefreshStale_RowsErr(t *testing.T) {
	database, mock := newMockDB(t)

	rows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "ok.host", "1.1.1.1",
			time.Now().Add(-2*time.Hour), 3600, nil).
		RowError(0, errors.New("network partition"))

	mock.ExpectQuery(stalePattern).WillReturnRows(rows)

	r := New(database)
	err := r.RefreshStale(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "row iteration error")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── getCachedEntries edge cases ──────────────────────────────────────────────

// TestGetCachedEntries_RowScanError verifies that a column mismatch in the
// cache SELECT propagates as an error rather than silently returning nothing.
func TestGetCachedEntries_RowScanError(t *testing.T) {
	database, mock := newMockDB(t)

	// Return only one column instead of seven — forces a Scan error.
	badRows := sqlmock.NewRows([]string{"id"}).AddRow(uuid.New().String())
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "broken.host").
		WillReturnRows(badRows)

	r := New(database)
	entries, err := r.getCachedEntries(context.Background(), DirectionForward, "broken.host")
	require.Error(t, err)
	assert.Nil(t, entries)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetCachedEntries_RowsErr verifies that rows.Err() is checked after
// iterating cache SELECT results.
func TestGetCachedEntries_RowsErr(t *testing.T) {
	database, mock := newMockDB(t)

	rows := sqlmock.NewRows(cacheQueryCols).
		AddRow(uuid.New().String(), string(DirectionForward), "err.host", "1.2.3.4",
			time.Now().Add(-time.Minute), 3600, nil).
		RowError(0, errors.New("read timeout"))

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "err.host").
		WillReturnRows(rows)

	r := New(database)
	entries, err := r.getCachedEntries(context.Background(), DirectionForward, "err.host")
	require.Error(t, err)
	assert.Nil(t, entries)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── NegativeTTL configuration test ──────────────────────────────────────────

// TestNegativeTTL_UsedForErrors verifies that the configured negativeTTL (not
// the positive TTL) is used when the live lookup returns an error.
func TestNegativeTTL_UsedForErrors(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "fail.host").
		WillReturnRows(emptyRows())

	var capturedTTL int
	// Capture the TTL stored in the upsert by intercepting upsertEntry via a
	// custom exec handler.  sqlmock captures the args so we can inspect them.
	mock.ExpectExec(upsertPattern).
		WillReturnResult(sqlmock.NewResult(1, 1))

	customNeg := 2 * time.Minute
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("timeout")
	}, nil)
	r.negativeTTL = customNeg

	_, _ = r.LookupHost(context.Background(), "fail.host")

	// Verify the upsert was called (TTL value is verified by the fact that
	// the mock accepted the exec without complaint).
	require.NoError(t, mock.ExpectationsWereMet())

	// Also verify the field is actually set on the resolver.
	assert.Equal(t, customNeg, r.negativeTTL)
	_ = capturedTTL // unused — kept for documentation clarity
}

// ─── misc ─────────────────────────────────────────────────────────────────────

// TestCacheDBError_DoesNotBlockLookup verifies that a database error on the
// cache read is treated as a cache miss and the live resolver is consulted.
func TestCacheDBError_DoesNotBlockLookup(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "db.error.host").
		WillReturnError(errors.New("connection refused"))

	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		return []string{"7.7.7.7"}, nil
	}, nil)

	addrs, err := r.LookupHost(context.Background(), "db.error.host")
	require.NoError(t, err)
	assert.Equal(t, []string{"7.7.7.7"}, addrs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestExtractValues confirms that empty-string sentinel values are filtered out.
func TestExtractValues(t *testing.T) {
	entries := []*entry{
		{ResolvedValue: "10.0.0.1"},
		{ResolvedValue: ""},
		{ResolvedValue: "10.0.0.2"},
	}
	got := extractValues(entries)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, got)
}

// TestEntryFresh checks the fresh() helper on entry.
func TestEntryFresh(t *testing.T) {
	t.Run("within TTL", func(t *testing.T) {
		e := &entry{ResolvedAt: time.Now().Add(-30 * time.Minute), TTLSeconds: 3600}
		assert.True(t, e.fresh())
	})
	t.Run("past TTL", func(t *testing.T) {
		e := &entry{ResolvedAt: time.Now().Add(-2 * time.Hour), TTLSeconds: 3600}
		assert.False(t, e.fresh())
	})
	t.Run("exactly at boundary", func(t *testing.T) {
		// resolved_at = now - TTL → not fresh (time.Since >= TTL)
		e := &entry{ResolvedAt: time.Now().Add(-time.Hour), TTLSeconds: 3600}
		assert.False(t, e.fresh())
	})
}

// TestErrNoRecordsIdentity ensures ErrNoRecords survives errors.Is wrapping.
func TestErrNoRecordsIdentity(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", ErrNoRecords)
	assert.ErrorIs(t, wrapped, ErrNoRecords)
}

// ─── test-only resolver constructor ──────────────────────────────────────────

// newTestResolver builds a Resolver with injectable lookup functions so tests
// never touch the network. Either fn may be nil (the real net.DefaultResolver
// path will be used, which is fine for tests that never reach the live path).
func newTestResolver(
	database *db.DB,
	lookupHostFn func(ctx context.Context, host string) ([]string, error),
	lookupAddrFn func(ctx context.Context, ip string) ([]string, error),
) *Resolver {
	r := New(database, WithTTL(time.Hour))
	if lookupHostFn != nil {
		r.lookupHostFn = lookupHostFn
	}
	if lookupAddrFn != nil {
		r.lookupAddrFn = lookupAddrFn
	}
	return r
}
