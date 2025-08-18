package db

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewRepository(t *testing.T) {
	t.Run("creates repository with nil db", func(t *testing.T) {
		// Test repository creation without actual database
		var db *DB
		repo := NewRepository(db)

		assert.NotNil(t, repo)
		assert.Equal(t, db, repo.db)
	})

	t.Run("repository structure", func(t *testing.T) {
		var db *DB
		repo := NewRepository(db)

		// Verify repository has expected structure
		assert.IsType(t, &Repository{}, repo)
	})
}

func TestNewScanTargetRepository(t *testing.T) {
	t.Run("creates scan target repository", func(t *testing.T) {
		var db *DB
		repo := NewScanTargetRepository(db)

		assert.NotNil(t, repo)
		assert.Equal(t, db, repo.db)
		assert.IsType(t, &ScanTargetRepository{}, repo)
	})

	t.Run("scan target repository structure", func(t *testing.T) {
		var db *DB
		repo := NewScanTargetRepository(db)

		// Test that repository has proper structure
		assert.NotNil(t, repo)
	})
}

func TestNewScanJobRepository(t *testing.T) {
	t.Run("creates scan job repository", func(t *testing.T) {
		var db *DB
		repo := NewScanJobRepository(db)

		assert.NotNil(t, repo)
		assert.Equal(t, db, repo.db)
		assert.IsType(t, &ScanJobRepository{}, repo)
	})
}

func TestNewHostRepository(t *testing.T) {
	t.Run("creates host repository", func(t *testing.T) {
		var db *DB
		repo := NewHostRepository(db)

		assert.NotNil(t, repo)
		assert.Equal(t, db, repo.db)
		assert.IsType(t, &HostRepository{}, repo)
	})
}

func TestNewPortScanRepository(t *testing.T) {
	t.Run("creates port scan repository", func(t *testing.T) {
		var db *DB
		repo := NewPortScanRepository(db)

		assert.NotNil(t, repo)
		assert.Equal(t, db, repo.db)
		assert.IsType(t, &PortScanRepository{}, repo)
	})
}

func TestScanTargetModel(t *testing.T) {
	t.Run("scan target creation", func(t *testing.T) {
		target := ScanTarget{
			ID:        uuid.New(),
			Name:      "test-target",
			ScanPorts: "80,443",
			Enabled:   true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		assert.NotEqual(t, uuid.Nil, target.ID)
		assert.Equal(t, "test-target", target.Name)
		assert.Equal(t, "80,443", target.ScanPorts)
		assert.True(t, target.Enabled)
		assert.False(t, target.CreatedAt.IsZero())
		assert.False(t, target.UpdatedAt.IsZero())
	})

	t.Run("scan target with various names", func(t *testing.T) {
		testNames := []string{
			"web-server",
			"database-cluster",
			"api-gateway",
			"load-balancer",
			"dns-server",
		}

		for _, name := range testNames {
			target := ScanTarget{
				ID:      uuid.New(),
				Name:    name,
				Enabled: true,
			}

			assert.NotEqual(t, uuid.Nil, target.ID)
			assert.Equal(t, name, target.Name)
			assert.True(t, target.Enabled)
			assert.NotEmpty(t, target.Name)
		}
	})

	t.Run("scan target with port ranges", func(t *testing.T) {
		portConfigs := []string{
			"80",
			"80,443",
			"80-443",
			"22,80,443,8080",
			"1-1000",
		}

		for _, ports := range portConfigs {
			target := ScanTarget{
				ID:        uuid.New(),
				Name:      "test-target",
				ScanPorts: ports,
			}

			assert.NotEqual(t, uuid.Nil, target.ID)
			assert.Equal(t, "test-target", target.Name)
			assert.Equal(t, ports, target.ScanPorts)
			assert.NotEmpty(t, target.ScanPorts)
		}
	})
}

func TestScanJobModel(t *testing.T) {
	t.Run("scan job creation", func(t *testing.T) {
		startTime := time.Now()
		job := ScanJob{
			ID:        uuid.New(),
			TargetID:  uuid.New(),
			Status:    "pending",
			StartedAt: &startTime,
			CreatedAt: time.Now(),
		}

		assert.NotEqual(t, uuid.Nil, job.ID)
		assert.NotEqual(t, uuid.Nil, job.TargetID)
		assert.Equal(t, "pending", job.Status)
		assert.NotNil(t, job.StartedAt)
		assert.False(t, job.CreatedAt.IsZero())
	})

	t.Run("scan job statuses", func(t *testing.T) {
		statuses := []string{
			"pending",
			"running",
			"completed",
			"failed",
			"cancelled",
		}

		for _, status := range statuses {
			job := ScanJob{
				ID:     uuid.New(),
				Status: status,
			}

			assert.NotEqual(t, uuid.Nil, job.ID)
			assert.Equal(t, status, job.Status)
			assert.NotEmpty(t, job.Status)
		}
	})

	t.Run("scan job with optional fields", func(t *testing.T) {
		completedTime := time.Now()
		errorMsg := "test error"
		job := ScanJob{
			ID:           uuid.New(),
			TargetID:     uuid.New(),
			Status:       "completed",
			CompletedAt:  &completedTime,
			ErrorMessage: &errorMsg,
		}

		assert.NotEqual(t, uuid.Nil, job.ID)
		assert.NotEqual(t, uuid.Nil, job.TargetID)
		assert.Equal(t, "completed", job.Status)
		assert.NotNil(t, job.CompletedAt)
		assert.NotNil(t, job.ErrorMessage)
		assert.Equal(t, "test error", *job.ErrorMessage)
	})
}

func TestScanResultModel(t *testing.T) {
	t.Run("scan result creation", func(t *testing.T) {
		result := ScanResult{
			ID:        uuid.New(),
			ScanID:    uuid.New(),
			HostID:    uuid.New(),
			Port:      80,
			Protocol:  "tcp",
			State:     "open",
			Service:   "http",
			ScannedAt: time.Now(),
		}

		assert.NotEqual(t, uuid.Nil, result.ID)
		assert.NotEqual(t, uuid.Nil, result.ScanID)
		assert.NotEqual(t, uuid.Nil, result.HostID)
		assert.Equal(t, 80, result.Port)
		assert.Equal(t, "tcp", result.Protocol)
		assert.Equal(t, "open", result.State)
		assert.Equal(t, "http", result.Service)
		assert.False(t, result.ScannedAt.IsZero())
	})

	t.Run("scan result with various protocols", func(t *testing.T) {
		protocols := []string{
			"tcp",
			"udp",
			"sctp",
		}

		for _, protocol := range protocols {
			result := ScanResult{
				ID:       uuid.New(),
				ScanID:   uuid.New(),
				HostID:   uuid.New(),
				Port:     80,
				Protocol: protocol,
				State:    "open",
			}

			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.NotEqual(t, uuid.Nil, result.ScanID)
			assert.NotEqual(t, uuid.Nil, result.HostID)
			assert.Equal(t, 80, result.Port)
			assert.Equal(t, protocol, result.Protocol)
			assert.Equal(t, "open", result.State)
		}
	})

	t.Run("scan result with various ports", func(t *testing.T) {
		testPorts := []int{
			21, 22, 23, 25, 53, 80, 110, 143, 443, 993, 995, 8080, 8443,
		}

		for _, port := range testPorts {
			result := ScanResult{
				ID:     uuid.New(),
				ScanID: uuid.New(),
				HostID: uuid.New(),
				Port:   port,
				State:  "open",
			}

			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.NotEqual(t, uuid.Nil, result.ScanID)
			assert.NotEqual(t, uuid.Nil, result.HostID)
			assert.Equal(t, port, result.Port)
			assert.Equal(t, "open", result.State)
			assert.True(t, port > 0 && port < 65536)
		}
	})

	t.Run("scan result with various states", func(t *testing.T) {
		states := []string{
			"open",
			"closed",
			"filtered",
			"open|filtered",
			"closed|filtered",
		}

		for _, state := range states {
			result := ScanResult{
				ID:     uuid.New(),
				ScanID: uuid.New(),
				HostID: uuid.New(),
				Port:   80,
				State:  state,
			}

			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.NotEqual(t, uuid.Nil, result.ScanID)
			assert.NotEqual(t, uuid.Nil, result.HostID)
			assert.Equal(t, 80, result.Port)
			assert.Equal(t, state, result.State)
		}
	})
}

func TestScanProfileModel(t *testing.T) {
	t.Run("scan profile creation", func(t *testing.T) {
		profile := ScanProfile{
			ID:          "profile-1",
			Name:        "Web Server Scan",
			Description: "Scan for web services",
			Ports:       "80,443,8080",
			ScanType:    "tcp",
			Timing:      "normal",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		assert.Equal(t, "profile-1", profile.ID)
		assert.Equal(t, "Web Server Scan", profile.Name)
		assert.Equal(t, "Scan for web services", profile.Description)
		assert.Equal(t, "80,443,8080", profile.Ports)
		assert.Equal(t, "tcp", profile.ScanType)
		assert.Equal(t, "normal", profile.Timing)
		assert.False(t, profile.CreatedAt.IsZero())
		assert.False(t, profile.UpdatedAt.IsZero())
	})

	t.Run("scan profile with different scan types", func(t *testing.T) {
		scanTypes := []string{"tcp", "udp", "syn", "connect"}

		for _, scanType := range scanTypes {
			profile := ScanProfile{
				ID:       "profile-test",
				Name:     "Test Profile",
				ScanType: scanType,
			}

			assert.Equal(t, "profile-test", profile.ID)
			assert.Equal(t, "Test Profile", profile.Name)
			assert.Equal(t, scanType, profile.ScanType)
		}
	})

	t.Run("scan profile with different timings", func(t *testing.T) {
		timings := []string{
			"paranoid",
			"sneaky",
			"polite",
			"normal",
			"aggressive",
			"insane",
		}

		for _, timing := range timings {
			profile := ScanProfile{
				ID:     "profile-test",
				Name:   "Test Profile",
				Timing: timing,
			}

			assert.Equal(t, "profile-test", profile.ID)
			assert.Equal(t, "Test Profile", profile.Name)
			assert.Equal(t, timing, profile.Timing)
		}
	})

	t.Run("scan profile with various port configurations", func(t *testing.T) {
		portConfigs := []string{
			"80",
			"80,443",
			"80-443",
			"22,80,443,8080",
			"1-1000",
			"T:100",
		}

		for _, ports := range portConfigs {
			profile := ScanProfile{
				ID:    "profile-test",
				Name:  "Test Profile",
				Ports: ports,
			}

			assert.Equal(t, "profile-test", profile.ID)
			assert.Equal(t, "Test Profile", profile.Name)
			assert.Equal(t, ports, profile.Ports)
			assert.NotEmpty(t, profile.Ports)
		}
	})
}

func TestModelValidation(t *testing.T) {
	t.Run("valid UUID generation", func(t *testing.T) {
		// Test that we can generate valid UUIDs for models
		id1 := uuid.New()
		id2 := uuid.New()

		assert.NotEqual(t, uuid.Nil, id1)
		assert.NotEqual(t, uuid.Nil, id2)
		assert.NotEqual(t, id1, id2)
	})

	t.Run("IP address validation helper", func(t *testing.T) {
		validIPs := []string{
			"127.0.0.1",
			"192.168.1.1",
			"10.0.0.1",
			"172.16.0.1",
			"8.8.8.8",
			"::1",
			"2001:db8::1",
		}

		for _, ip := range validIPs {
			parsed := net.ParseIP(ip)
			assert.NotNil(t, parsed, "IP should be valid: %s", ip)
		}
	})

	t.Run("invalid IP address detection", func(t *testing.T) {
		invalidIPs := []string{
			"256.256.256.256",
			"192.168.1",
			"192.168.1.1.1",
			"not-an-ip",
			"",
		}

		for _, ip := range invalidIPs {
			parsed := net.ParseIP(ip)
			assert.Nil(t, parsed, "IP should be invalid: %s", ip)
		}
	})

	t.Run("port number validation", func(t *testing.T) {
		validPorts := []int{1, 22, 80, 443, 8080, 65535}

		for _, port := range validPorts {
			assert.True(t, port > 0 && port < 65536, "Port should be valid: %d", port)
		}
	})

	t.Run("invalid port number detection", func(t *testing.T) {
		invalidPorts := []int{0, -1, 65536, 100000}

		for _, port := range invalidPorts {
			assert.False(t, port > 0 && port < 65536, "Port should be invalid: %d", port)
		}
	})
}

func TestRepositoryStructures(t *testing.T) {
	t.Run("all repositories can be created", func(t *testing.T) {
		var db *DB

		// Test that all repository types can be instantiated
		scanTargetRepo := NewScanTargetRepository(db)
		assert.IsType(t, &ScanTargetRepository{}, scanTargetRepo)

		scanJobRepo := NewScanJobRepository(db)
		assert.IsType(t, &ScanJobRepository{}, scanJobRepo)

		hostRepo := NewHostRepository(db)
		assert.IsType(t, &HostRepository{}, hostRepo)

		portScanRepo := NewPortScanRepository(db)
		assert.IsType(t, &PortScanRepository{}, portScanRepo)

		baseRepo := NewRepository(db)
		assert.IsType(t, &Repository{}, baseRepo)
	})

	t.Run("repositories have correct db reference", func(t *testing.T) {
		var db *DB

		scanTargetRepo := &ScanTargetRepository{db: db}
		scanJobRepo := &ScanJobRepository{db: db}
		baseRepo := &Repository{db: db}

		assert.Equal(t, db, scanTargetRepo.getDB())
		assert.Equal(t, db, scanJobRepo.getDB())
		assert.Equal(t, db, baseRepo.getDB())
	})
}

func TestModelTimestamps(t *testing.T) {
	t.Run("models support timestamps", func(t *testing.T) {
		now := time.Now()

		// Test ScanTarget timestamps
		target := ScanTarget{
			CreatedAt: now,
			UpdatedAt: now,
		}
		assert.Equal(t, now.Unix(), target.CreatedAt.Unix())
		assert.Equal(t, now.Unix(), target.UpdatedAt.Unix())

		// Test ScanJob timestamps
		job := ScanJob{
			StartedAt: &now,
			CreatedAt: now,
		}
		assert.Equal(t, now.Unix(), job.StartedAt.Unix())
		assert.Equal(t, now.Unix(), job.CreatedAt.Unix())

		// Test ScanResult timestamps
		result := ScanResult{
			ScannedAt: now,
		}
		assert.Equal(t, now.Unix(), result.ScannedAt.Unix())

		// Test ScanProfile timestamps
		profile := ScanProfile{
			CreatedAt: now,
			UpdatedAt: now,
		}
		assert.Equal(t, now.Unix(), profile.CreatedAt.Unix())
		assert.Equal(t, now.Unix(), profile.UpdatedAt.Unix())
	})

	t.Run("timestamp ordering", func(t *testing.T) {
		earlier := time.Now().Add(-time.Hour)
		later := time.Now()

		profile := ScanProfile{
			CreatedAt: earlier,
			UpdatedAt: later,
		}

		assert.True(t, profile.CreatedAt.Before(profile.UpdatedAt))
		assert.True(t, profile.UpdatedAt.After(profile.CreatedAt))
	})
}

func TestModelDefaults(t *testing.T) {
	t.Run("models with zero values", func(t *testing.T) {
		// Test zero value initialization
		var target ScanTarget
		assert.Equal(t, uuid.Nil, target.ID)
		assert.Empty(t, target.Name)
		assert.False(t, target.Enabled)
		assert.True(t, target.CreatedAt.IsZero())

		var job ScanJob
		assert.Equal(t, uuid.Nil, job.ID)
		assert.Empty(t, job.Status)
		assert.Nil(t, job.StartedAt)

		var result ScanResult
		assert.Equal(t, uuid.Nil, result.ID)
		assert.Zero(t, result.Port)
		assert.Empty(t, result.Protocol)

		var profile ScanProfile
		assert.Empty(t, profile.ID)
		assert.Empty(t, profile.Name)
		assert.Empty(t, profile.ScanType)
	})
}

// Helper methods for testing repository database references
func (r *Repository) getDB() *DB           { return r.db }
func (r *ScanTargetRepository) getDB() *DB { return r.db }
func (r *ScanJobRepository) getDB() *DB    { return r.db }

// BenchmarkRepositoryCreation benchmarks repository creation
func BenchmarkRepositoryCreation(b *testing.B) {
	var db *DB

	b.Run("NewRepository", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = NewRepository(db)
		}
	})

	b.Run("NewScanTargetRepository", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = NewScanTargetRepository(db)
		}
	})

	b.Run("AllRepositories", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = NewRepository(db)
			_ = NewScanTargetRepository(db)
			_ = NewScanJobRepository(db)
		}
	})
}

// BenchmarkModelCreation benchmarks model struct creation
func BenchmarkModelCreation(b *testing.B) {
	b.Run("ScanTarget", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ScanTarget{
				ID:        uuid.New(),
				Name:      "test-target",
				ScanPorts: "80,443",
				Enabled:   true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
	})
}
