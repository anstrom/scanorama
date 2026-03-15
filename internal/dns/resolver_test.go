// Package dns provides cached DNS resolution for scanorama.
package dns

import (
	"context"
	"errors"
	"fmt"
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

// sqlmock v1.5.2 strips whitespace then compiles the expected string as a
// regexp against the collapsed actual SQL.  Use .+ (not .*) between tokens so
// the greedy quantifier cannot swallow the next literal keyword.
const (
	selectPattern = "SELECT .+ FROM dns_cache WHERE direction"
	upsertPattern = "INSERT INTO dns_cache"
	stalePattern  = "FROM dns_cache WHERE resolved_at"
)

// ─── tests ────────────────────────────────────────────────────────────────────

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

	rows := sqlmock.NewRows(cacheQueryCols).AddRow(
		uuid.New().String(),
		string(DirectionReverse),
		"10.0.0.1",
		"", // empty → negative cache entry
		time.Now().Add(-time.Minute),
		300,
		nil,
	)
	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionReverse), "10.0.0.1").
		WillReturnRows(rows)

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

// TestLookupAddr_InvalidIP verifies that a non-IP string is rejected immediately.
func TestLookupAddr_InvalidIP(t *testing.T) {
	database, mock := newMockDB(t)
	r := newTestResolver(database, nil, nil)

	_, err := r.LookupAddr(context.Background(), "not-an-ip")
	require.Error(t, err)
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

// TestLookupHost_Singleflight verifies that concurrent callers for the same
// key share a single live lookup.
func TestLookupHost_Singleflight(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(selectPattern).
		WithArgs(string(DirectionForward), "slow.host").
		WillReturnRows(emptyRows())

	// Only one upsert should reach the DB regardless of goroutine count.
	mock.ExpectExec(upsertPattern).WillReturnResult(sqlmock.NewResult(1, 1))

	var liveCallCount atomic.Int32
	r := newTestResolver(database, func(_ context.Context, _ string) ([]string, error) {
		liveCallCount.Add(1)
		time.Sleep(20 * time.Millisecond) // simulate latency
		return []string{"5.5.5.5"}, nil
	}, nil)

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			addrs, err := r.LookupHost(context.Background(), "slow.host")
			errs[i] = err
			if len(addrs) > 0 {
				results[i] = addrs[0]
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, liveCallCount.Load(), "live resolver should only be called once")
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d should not error", i)
		assert.Equal(t, "5.5.5.5", results[i])
	}
}

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
