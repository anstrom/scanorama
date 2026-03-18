package db

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestMigrator returns a Migrator with a nil DB, sufficient for testing
// methods that don't touch the database.
func newTestMigrator() *Migrator {
	return &Migrator{db: nil}
}

// ----------------------------------------------------------------------------
// NewMigrator
// ----------------------------------------------------------------------------

func TestNewMigrator(t *testing.T) {
	t.Run("returns non-nil migrator", func(t *testing.T) {
		m := newTestMigrator()
		assert.NotNil(t, m)
	})

	t.Run("stores db reference", func(t *testing.T) {
		m := NewMigrator(nil)
		require.NotNil(t, m)
		assert.Nil(t, m.db)
	})
}

// ----------------------------------------------------------------------------
// calculateChecksum
// ----------------------------------------------------------------------------

func TestCalculateChecksum(t *testing.T) {
	m := newTestMigrator()

	t.Run("empty string produces valid hex", func(t *testing.T) {
		result := m.calculateChecksum("")
		assert.Len(t, result, 64, "SHA-256 hex should be 64 characters")
		expected := sha256hex("")
		assert.Equal(t, expected, result)
	})

	t.Run("known content produces correct checksum", func(t *testing.T) {
		content := "CREATE TABLE foo (id SERIAL PRIMARY KEY);"
		result := m.calculateChecksum(content)
		assert.Len(t, result, 64)
		assert.Equal(t, sha256hex(content), result)
	})

	t.Run("different content produces different checksum", func(t *testing.T) {
		a := m.calculateChecksum("SELECT 1")
		b := m.calculateChecksum("SELECT 2")
		assert.NotEqual(t, a, b)
	})

	t.Run("identical content produces identical checksum", func(t *testing.T) {
		content := "DROP TABLE bar;"
		assert.Equal(t, m.calculateChecksum(content), m.calculateChecksum(content))
	})

	t.Run("result is lowercase hex", func(t *testing.T) {
		result := m.calculateChecksum("some sql")
		assert.Equal(t, strings.ToLower(result), result, "checksum should be lowercase hex")
	})

	t.Run("whitespace differences change checksum", func(t *testing.T) {
		a := m.calculateChecksum("SELECT 1")
		b := m.calculateChecksum("SELECT 1 ")
		assert.NotEqual(t, a, b, "trailing whitespace should produce a different checksum")
	})
}

// ----------------------------------------------------------------------------
// getMigrationFiles
// ----------------------------------------------------------------------------

func TestGetMigrationFiles(t *testing.T) {
	m := newTestMigrator()

	files, err := m.getMigrationFiles()
	require.NoError(t, err, "getMigrationFiles should not return an error")

	t.Run("returns at least one file", func(t *testing.T) {
		assert.NotEmpty(t, files, "expected at least one embedded SQL migration file")
	})

	t.Run("all files have .sql extension", func(t *testing.T) {
		for _, f := range files {
			assert.Equal(t, ".sql", filepath.Ext(f), "file %q should have .sql extension", f)
		}
	})

	t.Run("files are sorted lexicographically", func(t *testing.T) {
		for i := 1; i < len(files); i++ {
			assert.LessOrEqual(t, files[i-1], files[i],
				"files should be sorted: %q should come before %q", files[i-1], files[i])
		}
	})

	t.Run("all files are readable from embed", func(t *testing.T) {
		for _, f := range files {
			content, err := migrationFiles.ReadFile(f)
			require.NoErrorf(t, err, "should be able to read embedded file %q", f)
			assert.NotEmpty(t, content, "embedded file %q should not be empty", f)
		}
	})
}

// ----------------------------------------------------------------------------
// Migration file naming convention
// ----------------------------------------------------------------------------

func TestMigrationFileNamingConvention(t *testing.T) {
	m := newTestMigrator()

	files, err := m.getMigrationFiles()
	require.NoError(t, err)
	require.NotEmpty(t, files)

	t.Run("files follow NNN_description format", func(t *testing.T) {
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".sql")
			parts := strings.SplitN(name, "_", 2)
			require.Lenf(t, parts, 2, "migration %q should contain at least one underscore", name)
			assert.NotEmpty(t, parts[0], "numeric prefix should not be empty in %q", name)
			assert.NotEmpty(t, parts[1], "description should not be empty in %q", name)
		}
	})

	t.Run("numeric prefixes are zero-padded to three digits", func(t *testing.T) {
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".sql")
			prefix := strings.SplitN(name, "_", 2)[0]
			assert.Lenf(t, prefix, 3, "numeric prefix of %q should be exactly 3 digits", name)
			for _, ch := range prefix {
				assert.True(t, ch >= '0' && ch <= '9',
					"prefix character %q in %q should be a digit", string(ch), name)
			}
		}
	})

	t.Run("files are sequentially numbered starting from 001", func(t *testing.T) {
		for i, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".sql")
			expectedPrefix := strings.ToLower(formatMigrationPrefix(i + 1))
			actualPrefix := strings.SplitN(name, "_", 2)[0]
			assert.Equalf(t, expectedPrefix, actualPrefix,
				"migration at index %d should have prefix %q, got %q (file: %q)",
				i, expectedPrefix, actualPrefix, f)
		}
	})

	t.Run("no duplicate numeric prefixes", func(t *testing.T) {
		seen := make(map[string]string)
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".sql")
			prefix := strings.SplitN(name, "_", 2)[0]
			if prev, exists := seen[prefix]; exists {
				t.Errorf("duplicate prefix %q: files %q and %q", prefix, prev, f)
			}
			seen[prefix] = f
		}
	})
}

// ----------------------------------------------------------------------------
// Migration content sanity checks
// ----------------------------------------------------------------------------

func TestMigrationContentSanity(t *testing.T) {
	m := newTestMigrator()

	files, err := m.getMigrationFiles()
	require.NoError(t, err)
	require.NotEmpty(t, files)

	t.Run("each file contains SQL keywords", func(t *testing.T) {
		sqlKeywords := []string{"CREATE", "ALTER", "INSERT", "DROP", "UPDATE", "--"}
		for _, f := range files {
			content, err := migrationFiles.ReadFile(f)
			require.NoErrorf(t, err, "could not read %q", f)

			upper := strings.ToUpper(string(content))
			found := false
			for _, kw := range sqlKeywords {
				if strings.Contains(upper, kw) {
					found = true
					break
				}
			}
			assert.Truef(t, found,
				"migration %q does not appear to contain any SQL keywords", f)
		}
	})

	t.Run("checksums are stable across calls", func(t *testing.T) {
		for _, f := range files {
			content, err := migrationFiles.ReadFile(f)
			require.NoError(t, err)

			s := string(content)
			first := m.calculateChecksum(s)
			second := m.calculateChecksum(s)
			assert.Equalf(t, first, second,
				"checksum for %q should be deterministic", f)
		}
	})

	t.Run("each file has a unique checksum", func(t *testing.T) {
		seen := make(map[string]string)
		for _, f := range files {
			content, err := migrationFiles.ReadFile(f)
			require.NoError(t, err)

			checksum := m.calculateChecksum(string(content))
			if prev, exists := seen[checksum]; exists {
				t.Errorf("files %q and %q have identical checksums — possible duplicate migration", prev, f)
			}
			seen[checksum] = f
		}
	})
}

// ----------------------------------------------------------------------------
// Migration struct
// ----------------------------------------------------------------------------

func TestMigrationStruct(t *testing.T) {
	t.Run("zero value is valid", func(t *testing.T) {
		var m Migration
		assert.Equal(t, 0, m.ID)
		assert.Empty(t, m.Name)
		assert.Empty(t, m.Checksum)
		assert.True(t, m.AppliedAt.IsZero())
	})

	t.Run("fields are assignable", func(t *testing.T) {
		m := Migration{
			ID:       1,
			Name:     "001_initial_schema",
			Checksum: "abc123",
		}
		assert.Equal(t, 1, m.ID)
		assert.Equal(t, "001_initial_schema", m.Name)
		assert.Equal(t, "abc123", m.Checksum)
	})
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// sha256hex returns the lowercase hex-encoded SHA-256 hash of s.
func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// formatMigrationPrefix returns the zero-padded three-digit prefix for the
// given 1-based sequence number, matching the convention used in migration
// file names (e.g. 1 → "001", 12 → "012").
func formatMigrationPrefix(n int) string {
	return strings.ToLower(string([]byte{
		byte('0' + n/100),
		byte('0' + (n/10)%10),
		byte('0' + n%10),
	}))
}
