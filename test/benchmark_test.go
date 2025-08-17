package test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/test/helpers"
	"github.com/google/uuid"
)

// BenchmarkSuite holds benchmark test environment.
type BenchmarkSuite struct {
	database *db.DB
	testEnv  *helpers.TestEnvironment
	ctx      context.Context
}

// setupBenchmarkSuite sets up the benchmark test environment.
func setupBenchmarkSuite(b *testing.B) *BenchmarkSuite {
	// Set up test environment
	testEnv := helpers.DefaultTestEnvironment()

	// Set up test database
	cfg := db.Config{
		Host:            getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            getEnvIntOrDefault("TEST_DB_PORT", 5432),
		Database:        getEnvOrDefault("TEST_DB_NAME", "scanorama_dev"),
		Username:        getEnvOrDefault("TEST_DB_USER", "scanorama_dev"),
		Password:        getEnvOrDefault("TEST_DB_PASSWORD", "dev_password"),
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}

	database, err := db.Connect(context.Background(), &cfg)
	if err != nil {
		b.Fatalf("Failed to connect to test database: %v", err)
	}

	return &BenchmarkSuite{
		database: database,
		testEnv:  testEnv,
		ctx:      context.Background(),
	}
}

// No cleanup functions - data accumulates across benchmark runs

// BenchmarkScanWithDatabaseStorage benchmarks scanning with database storage.
func BenchmarkScanWithDatabaseStorage(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	// Use standard port for testing (no Docker services needed)
	testPort := "22"

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{"localhost"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  5,
		Concurrency: 1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result, err := scanning.RunScanWithContext(suite.ctx, scanConfig, suite.database)
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
		if len(result.Hosts) == 0 {
			b.Fatalf("No hosts found in scan result")
		}
	}
}

// BenchmarkScanWithoutDatabase benchmarks scanning without database storage.
func BenchmarkScanWithoutDatabase(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	// Use standard port for testing (no Docker services needed)
	testPort := "22"

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{"localhost"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  5,
		Concurrency: 1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result, err := scanning.RunScanWithContext(suite.ctx, scanConfig, nil) // No database
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
		if len(result.Hosts) == 0 {
			b.Fatalf("No hosts found in scan result")
		}
	}
}

// BenchmarkDiscoveryWithDatabase benchmarks discovery with database storage.
func BenchmarkDiscoveryWithDatabase(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.1/32",
		Method:      "ping",
		DetectOS:    false,
		Timeout:     3 * time.Second,
		Concurrency: 1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		job, err := discoveryEngine.Discover(suite.ctx, &discoveryConfig)
		if err != nil {
			b.Fatalf("Discovery failed: %v", err)
		}

		// Wait for discovery to complete
		maxWait := 10 * time.Second
		startTime := time.Now()

		for {
			if time.Since(startTime) > maxWait {
				b.Fatalf("Discovery timed out")
			}

			var status string
			query := `SELECT status FROM discovery_jobs WHERE id = $1`
			err := suite.database.QueryRowContext(suite.ctx, query, job.ID).Scan(&status)
			if err != nil {
				b.Fatalf("Failed to check discovery status: %v", err)
			}

			if status == "completed" || status == "failed" {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BenchmarkDatabaseQueries benchmarks various database queries.
func BenchmarkDatabaseQueries(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	// Set up test data first
	setupBenchmarkTestData(b, suite)

	b.Run("QueryActiveHosts", func(b *testing.B) {
		hostRepo := db.NewHostRepository(suite.database)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			hosts, err := hostRepo.GetActiveHosts(suite.ctx)
			if err != nil {
				b.Fatalf("Failed to query active hosts: %v", err)
			}
			if len(hosts) == 0 {
				b.Fatalf("No active hosts found")
			}
		}
	})

	b.Run("QueryPortScansByHost", func(b *testing.B) {
		// Get a host ID
		var hostID uuid.UUID
		query := `SELECT id FROM hosts WHERE status = 'up' LIMIT 1`
		err := suite.database.QueryRowContext(suite.ctx, query).Scan(&hostID)
		if err != nil {
			b.Fatalf("Failed to get host ID: %v", err)
		}

		portRepo := db.NewPortScanRepository(suite.database)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			portScans, err := portRepo.GetByHost(suite.ctx, hostID)
			if err != nil {
				b.Fatalf("Failed to query port scans: %v", err)
			}
			_ = portScans
		}
	})

	b.Run("QueryNetworkSummary", func(b *testing.B) {
		summaryRepo := db.NewNetworkSummaryRepository(suite.database)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			summaries, err := summaryRepo.GetAll(suite.ctx)
			if err != nil {
				b.Fatalf("Failed to query network summary: %v", err)
			}
			_ = summaries
		}
	})

	b.Run("ComplexJoinQuery", func(b *testing.B) {
		query := `
			SELECT
				h.ip_address,
				h.status,
				h.last_seen,
				COUNT(ps.id) as total_ports,
				COUNT(ps.id) FILTER (WHERE ps.state = 'open') as open_ports
			FROM hosts h
			LEFT JOIN port_scans ps ON h.id = ps.host_id
			WHERE h.status = 'up'
			GROUP BY h.id, h.ip_address, h.status, h.last_seen
			ORDER BY h.last_seen DESC
			LIMIT 10
		`

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			rows, err := suite.database.QueryContext(suite.ctx, query)
			if err != nil {
				b.Fatalf("Failed to execute complex query: %v", err)
			}

			for rows.Next() {
				var ipAddr, status string
				var lastSeen time.Time
				var totalPorts, openPorts int
				err := rows.Scan(&ipAddr, &status, &lastSeen, &totalPorts, &openPorts)
				if err != nil {
					b.Fatalf("Failed to scan query result: %v", err)
				}
			}
			if err := rows.Close(); err != nil {
				b.Fatalf("Failed to close rows: %v", err)
			}
		}
	})
}

// BenchmarkBulkInsertPortScans benchmarks bulk insertion of port scan results.
func BenchmarkBulkInsertPortScans(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	// Create a test host and scan job
	hostID := uuid.New()
	jobID := uuid.New()
	targetID := uuid.New()

	// Insert test host
	hostRepo := db.NewHostRepository(suite.database)
	testHost := &db.Host{
		ID:        hostID,
		IPAddress: db.IPAddr{IP: []byte{127, 0, 0, 1}},
		Status:    "up",
	}
	err := hostRepo.CreateOrUpdate(suite.ctx, testHost)
	if err != nil {
		b.Fatalf("Failed to create test host: %v", err)
	}

	// Insert test scan target and job
	targetRepo := db.NewScanTargetRepository(suite.database)
	// Create a proper network for the target
	_, ipnet, _ := net.ParseCIDR("127.0.0.1/32")
	testTarget := &db.ScanTarget{
		ID:                  targetID,
		Name:                "Benchmark Target",
		Network:             db.NetworkAddr{IPNet: *ipnet},
		ScanIntervalSeconds: 3600,
		ScanPorts:           "80,443,22",
		ScanType:            "connect",
		Enabled:             true,
	}
	err = targetRepo.Create(suite.ctx, testTarget)
	if err != nil {
		b.Fatalf("Failed to create test target: %v", err)
	}

	jobRepo := db.NewScanJobRepository(suite.database)
	testJob := &db.ScanJob{
		ID:       jobID,
		TargetID: targetID,
		Status:   "completed",
	}
	err = jobRepo.Create(suite.ctx, testJob)
	if err != nil {
		b.Fatalf("Failed to create test job: %v", err)
	}

	// Generate port scan data
	portScans := make([]*db.PortScan, 100)
	for i := 0; i < 100; i++ {
		portScans[i] = &db.PortScan{
			ID:       uuid.New(),
			JobID:    jobID,
			HostID:   hostID,
			Port:     i + 1,
			Protocol: "tcp",
			State:    "open",
		}
	}

	portRepo := db.NewPortScanRepository(suite.database)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Generate unique port scans for each iteration to avoid conflicts
		uniquePortScans := make([]*db.PortScan, 100)
		for j := 0; j < 100; j++ {
			uniquePortScans[j] = &db.PortScan{
				ID:       uuid.New(),
				JobID:    jobID,
				HostID:   hostID,
				Port:     (i*100 + j + 1), // Unique port numbers
				Protocol: "tcp",
				State:    "open",
			}
		}

		// Benchmark bulk insert
		err := portRepo.CreateBatch(suite.ctx, uniquePortScans)
		if err != nil {
			b.Fatalf("Failed to bulk insert port scans: %v", err)
		}
	}
}

// BenchmarkConcurrentScans benchmarks concurrent scanning operations.
func BenchmarkConcurrentScans(b *testing.B) {
	suite := setupBenchmarkSuite(b)

	// Use standard port for testing (no Docker services needed)
	testPort := "22"

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{"localhost"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  5,
		Concurrency: 5, // Test with higher concurrency
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := scanning.RunScanWithContext(suite.ctx, scanConfig, suite.database)
			if err != nil {
				b.Fatalf("Concurrent scan failed: %v", err)
			}
			if len(result.Hosts) == 0 {
				b.Fatalf("No hosts found in concurrent scan result")
			}
		}
	})
}

// setupBenchmarkTestData creates test data for benchmarking.
func setupBenchmarkTestData(b *testing.B, suite *BenchmarkSuite) {
	// Use standard port for testing (no Docker services needed)
	testPort := "22"

	// Run a scan to populate some test data
	scanConfig := &scanning.ScanConfig{
		Targets:     []string{"localhost"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  5,
		Concurrency: 1,
	}

	_, err := scanning.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	if err != nil {
		b.Fatalf("Failed to set up benchmark test data: %v", err)
	}
}
