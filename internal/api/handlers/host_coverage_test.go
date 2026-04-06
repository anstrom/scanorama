// Package handlers provides HTTP request handlers for the Scanorama API.
package handlers

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/anstrom/scanorama/internal/db"
)

// coverageIntPtr returns a pointer to an int — local test helper for coverage tests.
func coverageIntPtr(i int) *int { return &i }

// coverageStrPtr returns a pointer to a string — local test helper for coverage tests.
func coverageStrPtr(s string) *string { return &s }

// makeTestHost returns a minimal *db.Host suitable for unit tests.
func makeTestHost() *db.Host {
	return &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.1")},
		Status:    "up",
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
}

// ── populateOSFields ──────────────────────────────────────────────────────────

func TestPopulateOSFields(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		host := makeTestHost()
		host.OSFamily = coverageStrPtr("Linux")
		host.OSName = coverageStrPtr("Ubuntu")
		host.OSVersion = coverageStrPtr("22.04")
		confidence := 90
		host.OSConfidence = &confidence

		r := &HostResponse{}
		populateOSFields(r, host)

		assert.Equal(t, "Linux", r.OSFamily)
		assert.Equal(t, "Linux", r.OS, "legacy OS field should mirror OSFamily")
		assert.Equal(t, "Ubuntu", r.OSName)
		assert.Equal(t, "22.04", r.OSVersion)
		assert.NotNil(t, r.OSConfidence)
		assert.Equal(t, 90, *r.OSConfidence)
		assert.Equal(t, "Ubuntu 22.04", r.OSVersionLegacy)
	})

	t.Run("legacy field: name+version", func(t *testing.T) {
		host := makeTestHost()
		host.OSName = coverageStrPtr("Windows Server")
		host.OSVersion = coverageStrPtr("2022")

		r := &HostResponse{}
		populateOSFields(r, host)

		assert.Equal(t, "Windows Server 2022", r.OSVersionLegacy)
		// OSFamily not set → OS and OSFamily stay empty
		assert.Empty(t, r.OS)
		assert.Empty(t, r.OSFamily)
	})

	t.Run("legacy field: name only", func(t *testing.T) {
		host := makeTestHost()
		host.OSName = coverageStrPtr("FreeBSD")

		r := &HostResponse{}
		populateOSFields(r, host)

		assert.Equal(t, "FreeBSD", r.OSVersionLegacy)
		assert.Empty(t, r.OSVersion)
	})

	t.Run("legacy field: version only", func(t *testing.T) {
		host := makeTestHost()
		host.OSVersion = coverageStrPtr("10.0")

		r := &HostResponse{}
		populateOSFields(r, host)

		assert.Equal(t, "10.0", r.OSVersionLegacy)
		assert.Empty(t, r.OSName)
	})

	t.Run("no OS data: nothing set", func(t *testing.T) {
		host := makeTestHost()
		// All OS fields remain nil.

		r := &HostResponse{}
		populateOSFields(r, host)

		assert.Empty(t, r.OSFamily)
		assert.Empty(t, r.OS)
		assert.Empty(t, r.OSName)
		assert.Empty(t, r.OSVersion)
		assert.Nil(t, r.OSConfidence)
		assert.Empty(t, r.OSVersionLegacy)
	})
}

// ── populateResponseTimeFields ────────────────────────────────────────────────

func TestPopulateResponseTimeFields(t *testing.T) {
	t.Run("all time fields populated", func(t *testing.T) {
		host := makeTestHost()
		host.ResponseTimeMS = coverageIntPtr(42)
		host.TimeoutCount = 3

		r := &HostResponse{}
		populateResponseTimeFields(r, host)

		assert.NotNil(t, r.ResponseTimeMS)
		assert.Equal(t, 42, *r.ResponseTimeMS)
		assert.Equal(t, 3, r.TimeoutCount)
	})

	t.Run("timeout count populated", func(t *testing.T) {
		host := makeTestHost()
		// ResponseTimeMS left nil; only TimeoutCount set.
		host.TimeoutCount = 5

		r := &HostResponse{}
		populateResponseTimeFields(r, host)

		assert.Nil(t, r.ResponseTimeMS)
		assert.Equal(t, 5, r.TimeoutCount)
	})

	t.Run("nil pointers: nothing set", func(t *testing.T) {
		host := makeTestHost()
		// ResponseTimeMS == nil, TimeoutCount == 0 (zero value).

		r := &HostResponse{}
		populateResponseTimeFields(r, host)

		assert.Nil(t, r.ResponseTimeMS)
		assert.Equal(t, 0, r.TimeoutCount)
	})
}
