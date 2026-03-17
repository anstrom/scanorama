package db

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectTestDB opens a connection to the test database, running migrations.
// It skips the test (rather than failing) when no database is reachable, so
// the test suite stays green in environments without a local Postgres instance
// (e.g. the unit-test CI job).
func connectTestDB(t *testing.T) *DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	host := dbEnvOrDefault("TEST_DB_HOST", "localhost")
	port := dbEnvIntOrDefault("TEST_DB_PORT", 5433)
	name := dbEnvOrDefault("TEST_DB_NAME", "scanorama_test")
	user := dbEnvOrDefault("TEST_DB_USER", "test_user")
	pass := dbEnvOrDefault("TEST_DB_PASSWORD", "test_password")

	cfg := &Config{
		Host:            host,
		Port:            port,
		Database:        name,
		Username:        user,
		Password:        pass,
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}

	// Try ConnectAndMigrate first (fresh DB). If migrations fail because the
	// schema was already applied by another package's test run in the same
	// `go test -p 1 ./...` invocation, fall back to a plain Connect.
	db, err := ConnectAndMigrate(context.Background(), cfg)
	if err != nil {
		db, err = Connect(context.Background(), cfg)
		if err != nil {
			t.Skipf("skipping — could not connect to test database (%s:%d/%s): %v",
				host, port, name, err)
			return nil
		}
	}
	return db
}

func dbEnvOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func dbEnvIntOrDefault(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		fmt.Printf("warning: could not parse %s=%q as int, using default %d\n", key, v, def)
	}
	return def
}

// strPtr / intPtr — local test helpers.
func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// TestHostRepository_CreateOrUpdate_BasicInsert verifies that a minimal host
// record can be inserted and then retrieved by IP address.
func TestHostRepository_CreateOrUpdate_BasicInsert(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewHostRepository(db)
	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.1")}
	// Clean up any leftover row from a previous run.
	_, _ = db.ExecContext(ctx, `DELETE FROM hosts WHERE ip_address = $1::inet`, "203.0.113.1")

	host := &Host{
		ID:        uuid.New(),
		IPAddress: ip,
		Status:    HostStatusUp,
	}

	require.NoError(t, repo.CreateOrUpdate(ctx, host))

	got, err := repo.GetByIP(ctx, ip)
	require.NoError(t, err)
	assert.Equal(t, HostStatusUp, got.Status)
	assert.Equal(t, ip.String(), got.IPAddress.String())
}

// TestHostRepository_CreateOrUpdate_OSFieldsPersisted verifies that all four
// OS columns (os_name, os_family, os_version, os_confidence) are written by
// CreateOrUpdate and read back correctly by GetByIP.
func TestHostRepository_CreateOrUpdate_OSFieldsPersisted(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewHostRepository(db)
	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.2")}
	_, _ = db.ExecContext(ctx, `DELETE FROM hosts WHERE ip_address = $1::inet`, "203.0.113.2")

	host := &Host{
		ID:           uuid.New(),
		IPAddress:    ip,
		Status:       HostStatusUp,
		OSName:       strPtr("Linux 5.15"),
		OSFamily:     strPtr("Linux"),
		OSVersion:    strPtr("5.15"),
		OSConfidence: intPtr(97),
	}

	require.NoError(t, repo.CreateOrUpdate(ctx, host))

	got, err := repo.GetByIP(ctx, ip)
	require.NoError(t, err)

	require.NotNil(t, got.OSName)
	assert.Equal(t, "Linux 5.15", *got.OSName)
	require.NotNil(t, got.OSFamily)
	assert.Equal(t, "Linux", *got.OSFamily)
	require.NotNil(t, got.OSVersion)
	assert.Equal(t, "5.15", *got.OSVersion)
	require.NotNil(t, got.OSConfidence)
	assert.Equal(t, 97, *got.OSConfidence)
}

// TestHostRepository_CreateOrUpdate_OnConflictUpdatesOS is the regression test
// for the bug where os_name and os_confidence were missing from the ON CONFLICT
// DO UPDATE clause: insert a host with NULL OS data, then upsert with real OS
// data and confirm the columns are updated (COALESCE picks EXCLUDED over NULL).
func TestHostRepository_CreateOrUpdate_OnConflictUpdatesOS(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewHostRepository(db)
	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.3")}
	_, _ = db.ExecContext(ctx, `DELETE FROM hosts WHERE ip_address = $1::inet`, "203.0.113.3")

	// First insert: no OS data.
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:        uuid.New(),
		IPAddress: ip,
		Status:    HostStatusUp,
	}))

	after1, err := repo.GetByIP(ctx, ip)
	require.NoError(t, err)
	assert.Nil(t, after1.OSName, "OSName should be nil before OS data is supplied")
	assert.Nil(t, after1.OSConfidence, "OSConfidence should be nil before OS data is supplied")

	// Second upsert: supply OS data for the same IP.
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:           uuid.New(),
		IPAddress:    ip,
		Status:       HostStatusUp,
		OSName:       strPtr("Windows 10"),
		OSFamily:     strPtr("Windows"),
		OSVersion:    strPtr("10"),
		OSConfidence: intPtr(92),
	}))

	got, err := repo.GetByIP(ctx, ip)
	require.NoError(t, err)

	require.NotNil(t, got.OSName, "OSName must be updated when the existing value was NULL")
	assert.Equal(t, "Windows 10", *got.OSName)
	require.NotNil(t, got.OSConfidence, "OSConfidence must be updated when the existing value was NULL")
	assert.Equal(t, 92, *got.OSConfidence)
	require.NotNil(t, got.OSFamily)
	assert.Equal(t, "Windows", *got.OSFamily)
	require.NotNil(t, got.OSVersion)
	assert.Equal(t, "10", *got.OSVersion)
}

// TestHostRepository_CreateOrUpdate_CoalescePreservesExisting verifies the
// other direction of the COALESCE: when a host already has OS data and a
// subsequent upsert supplies nil OS fields, the original values are preserved.
func TestHostRepository_CreateOrUpdate_CoalescePreservesExisting(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewHostRepository(db)
	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.4")}
	_, _ = db.ExecContext(ctx, `DELETE FROM hosts WHERE ip_address = $1::inet`, "203.0.113.4")

	// First insert: with full OS data.
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:           uuid.New(),
		IPAddress:    ip,
		Status:       HostStatusUp,
		OSName:       strPtr("macOS 14"),
		OSFamily:     strPtr("macOS"),
		OSVersion:    strPtr("14"),
		OSConfidence: intPtr(88),
	}))

	// Second upsert: OS fields nil (ping-only re-discovery, no OS detection).
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:        uuid.New(),
		IPAddress: ip,
		Status:    HostStatusUp,
	}))

	got, err := repo.GetByIP(ctx, ip)
	require.NoError(t, err)

	require.NotNil(t, got.OSName, "OSName should be preserved by COALESCE when incoming value is NULL")
	assert.Equal(t, "macOS 14", *got.OSName)
	require.NotNil(t, got.OSFamily)
	assert.Equal(t, "macOS", *got.OSFamily)
	require.NotNil(t, got.OSVersion)
	assert.Equal(t, "14", *got.OSVersion)
	require.NotNil(t, got.OSConfidence)
	assert.Equal(t, 88, *got.OSConfidence)
}
