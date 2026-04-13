// Package scanning — unit tests for NSE script output parsing.
package scanning

import (
	"testing"
	"time"

	nmap "github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── parsePortScripts ──────────────────────────────────────────────────────────

func TestParsePortScripts_Empty(t *testing.T) {
	assert.Nil(t, parsePortScripts(nil))
	assert.Nil(t, parsePortScripts([]nmap.Script{}))
}

func TestParsePortScripts_Banner(t *testing.T) {
	scripts := []nmap.Script{{ID: "banner", Output: "  SSH-2.0-OpenSSH_8.9  "}}
	got := parsePortScripts(scripts)
	require.NotNil(t, got)
	assert.Equal(t, "SSH-2.0-OpenSSH_8.9", got.Banner)
	assert.Empty(t, got.HTTPTitle)
	assert.Empty(t, got.SSHKeyFingerprint)
	assert.Nil(t, got.SSLCert)
}

func TestParsePortScripts_HTTPTitle_Normal(t *testing.T) {
	scripts := []nmap.Script{{ID: "http-title", Output: "Welcome to nginx"}}
	got := parsePortScripts(scripts)
	require.NotNil(t, got)
	assert.Equal(t, "Welcome to nginx", got.HTTPTitle)
}

func TestParsePortScripts_HTTPTitle_ErrorFiltered(t *testing.T) {
	scripts := []nmap.Script{{ID: "http-title", Output: "ERROR: No response from server"}}
	got := parsePortScripts(scripts)
	// ERROR prefix is filtered — the only field that would have been set is
	// HTTPTitle; with it empty the result should be nil (isEmpty check).
	assert.Nil(t, got)
}

func TestParsePortScripts_HTTPTitle_RedirectFiltered(t *testing.T) {
	scripts := []nmap.Script{{ID: "http-title", Output: "Did not follow redirect to https://example.com/"}}
	assert.Nil(t, parsePortScripts(scripts))
}

func TestParsePortScripts_SSHHostkey(t *testing.T) {
	output := "2048 SHA256:abc123def456 (RSA)\n4096 SHA256:xyz789 (RSA)"
	scripts := []nmap.Script{{ID: "ssh-hostkey", Output: output}}
	got := parsePortScripts(scripts)
	require.NotNil(t, got)
	// Only the first line should be captured.
	assert.Equal(t, "2048 SHA256:abc123def456 (RSA)", got.SSHKeyFingerprint)
}

func TestParsePortScripts_UnrecognisedScript_ReturnsNil(t *testing.T) {
	// A script whose ID we don't handle should leave all fields empty,
	// triggering isEmpty() and returning nil rather than a zero-value struct.
	scripts := []nmap.Script{{ID: "ftp-bounce", Output: "some output"}}
	assert.Nil(t, parsePortScripts(scripts))
}

func TestParsePortScripts_AllFieldsReturnedTogether(t *testing.T) {
	scripts := []nmap.Script{
		{ID: "banner", Output: "HTTP/1.1 200 OK"},
		{ID: "http-title", Output: "My Page"},
	}
	got := parsePortScripts(scripts)
	require.NotNil(t, got)
	assert.Equal(t, "HTTP/1.1 200 OK", got.Banner)
	assert.Equal(t, "My Page", got.HTTPTitle)
}

// ── parseSSLCert ──────────────────────────────────────────────────────────────

func TestParseSSLCert_MissingSubjectTable(t *testing.T) {
	s := &nmap.Script{ID: "ssl-cert", Output: ""}
	assert.Nil(t, parseSSLCert(s))
}

func TestParseSSLCert_BasicFields(t *testing.T) {
	s := &nmap.Script{
		ID: "ssl-cert",
		Tables: []nmap.Table{
			{
				Key:      "subject",
				Elements: []nmap.Element{{Key: "commonName", Value: "example.com"}},
			},
			{
				Key:      "issuer",
				Elements: []nmap.Element{{Key: "commonName", Value: "Let's Encrypt Authority X3"}},
			},
			{
				Key: "validity",
				Elements: []nmap.Element{
					{Key: "notBefore", Value: "2024-01-01T00:00:00"},
					{Key: "notAfter", Value: "2024-04-01T00:00:00"},
				},
			},
		},
		Elements: []nmap.Element{{Key: "sig_algo", Value: "sha256WithRSAEncryption"}},
	}

	got := parseSSLCert(s)
	require.NotNil(t, got)
	assert.Equal(t, "example.com", got.SubjectCN)
	assert.Equal(t, "Let's Encrypt Authority X3", got.Issuer)
	assert.Equal(t, "sha256WithRSAEncryption", got.KeyType)

	wantBefore, _ := time.Parse("2006-01-02T15:04:05", "2024-01-01T00:00:00")
	wantAfter, _ := time.Parse("2006-01-02T15:04:05", "2024-04-01T00:00:00")
	assert.Equal(t, wantBefore.UTC(), got.NotBefore)
	assert.Equal(t, wantAfter.UTC(), got.NotAfter)
}

func TestParseSSLCert_SANs(t *testing.T) {
	s := &nmap.Script{
		ID: "ssl-cert",
		Tables: []nmap.Table{
			{
				Key:      "subject",
				Elements: []nmap.Element{{Key: "commonName", Value: "example.com"}},
			},
			{
				Key: "extensions",
				Tables: []nmap.Table{
					{
						Key: "Subject Alternative Name",
						Elements: []nmap.Element{
							{Value: "example.com"},
							{Value: "www.example.com"},
						},
					},
				},
			},
		},
	}

	got := parseSSLCert(s)
	require.NotNil(t, got)
	assert.Equal(t, []string{"example.com", "www.example.com"}, got.SANs)
}

// ── parseHostScripts ──────────────────────────────────────────────────────────

func TestParseHostScripts_SMBOSDiscovery_AppliedWhenNoNmapOS(t *testing.T) {
	host := &Host{}
	scripts := []nmap.Script{
		{
			ID: "smb-os-discovery",
			Elements: []nmap.Element{
				{Key: "os", Value: "Windows Server 2019"},
				{Key: "domain", Value: "CORP"},
				{Key: "FQDN", Value: "srv01.corp.local"},
			},
		},
	}

	parseHostScripts(scripts, host)

	assert.Equal(t, "Windows Server 2019", host.OSName)
	assert.Equal(t, 60, host.OSAccuracy)
	assert.Equal(t, "Windows", host.OSFamily)
	assert.Equal(t, "CORP", host.SMBDomain)
	assert.Equal(t, "srv01.corp.local", host.SMBHostname)
}

func TestParseHostScripts_SMBOSDiscovery_DoesNotOverrideNmapOS(t *testing.T) {
	host := &Host{
		OSName:     "Linux 5.4",
		OSAccuracy: 95,
		OSFamily:   "Linux",
	}
	scripts := []nmap.Script{
		{
			ID:       "smb-os-discovery",
			Elements: []nmap.Element{{Key: "os", Value: "Windows 10"}},
		},
	}

	parseHostScripts(scripts, host)

	// nmap -O result must not be overridden.
	assert.Equal(t, "Linux 5.4", host.OSName)
	assert.Equal(t, 95, host.OSAccuracy)
	assert.Equal(t, "Linux", host.OSFamily)
}

func TestParseHostScripts_NBStat_SetsHostname(t *testing.T) {
	host := &Host{}
	scripts := []nmap.Script{
		{
			ID: "nbstat",
			Elements: []nmap.Element{
				{Key: "NetBIOS_Computer_Name", Value: "WORKSTATION01"},
			},
		},
	}

	parseHostScripts(scripts, host)
	assert.Equal(t, "WORKSTATION01", host.SMBHostname)
}

func TestParseHostScripts_NBStat_DoesNotOverrideSMBHostname(t *testing.T) {
	host := &Host{SMBHostname: "from-smb-discovery"}
	scripts := []nmap.Script{
		{
			ID:       "nbstat",
			Elements: []nmap.Element{{Key: "NetBIOS_Computer_Name", Value: "from-nbstat"}},
		},
	}

	parseHostScripts(scripts, host)
	// smb-os-discovery hostname wins.
	assert.Equal(t, "from-smb-discovery", host.SMBHostname)
}

func TestParseHostScripts_SMBOSDiscovery_LowercaseFQDN(t *testing.T) {
	host := &Host{}
	scripts := []nmap.Script{
		{
			ID:       "smb-os-discovery",
			Elements: []nmap.Element{{Key: "fqdn", Value: "srv.corp.local"}},
		},
	}
	parseHostScripts(scripts, host)
	assert.Equal(t, "srv.corp.local", host.SMBHostname)
}

// ── parseNmapTime ─────────────────────────────────────────────────────────────

func TestParseNmapTime_RFC3339(t *testing.T) {
	got, err := parseNmapTime("2024-01-15T10:30:00")
	require.NoError(t, err)
	assert.Equal(t, 2024, got.Year())
	assert.Equal(t, time.January, got.Month())
	assert.Equal(t, 15, got.Day())
}

func TestParseNmapTime_SpaceSeparated(t *testing.T) {
	got, err := parseNmapTime("2024-06-01 12:00:00")
	require.NoError(t, err)
	assert.Equal(t, 2024, got.Year())
	assert.Equal(t, time.June, got.Month())
}

func TestParseNmapTime_RFC3339WithTimezone(t *testing.T) {
	// Verify the time.RFC3339 fallback handles a Z-suffixed timestamp.
	got, err := parseNmapTime("2024-01-15T10:30:00Z")
	require.NoError(t, err)
	assert.Equal(t, 2024, got.Year())
	assert.Equal(t, time.January, got.Month())
}

func TestParseNmapTime_Invalid(t *testing.T) {
	_, err := parseNmapTime("not-a-date")
	require.Error(t, err)
}

// ── nseToDBCert ───────────────────────────────────────────────────────────────

func TestNSEToDBCert_NilsForEmptyFields(t *testing.T) {
	cert := nseToDBCert(uuid.New(), 443, &NSECertData{})
	assert.Nil(t, cert.SubjectCN)
	assert.Nil(t, cert.Issuer)
	assert.Nil(t, cert.KeyType)
	assert.Nil(t, cert.NotBefore)
	assert.Nil(t, cert.NotAfter)
}

func TestNSEToDBCert_PopulatesAllFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	later := now.Add(90 * 24 * time.Hour)
	id := uuid.New()

	c := &NSECertData{
		SubjectCN: "example.com",
		Issuer:    "My CA",
		KeyType:   "sha256WithRSAEncryption",
		SANs:      []string{"example.com", "www.example.com"},
		NotBefore: now,
		NotAfter:  later,
	}

	got := nseToDBCert(id, 443, c)
	require.NotNil(t, got.SubjectCN)
	assert.Equal(t, "example.com", *got.SubjectCN)
	require.NotNil(t, got.Issuer)
	assert.Equal(t, "My CA", *got.Issuer)
	require.NotNil(t, got.KeyType)
	assert.Equal(t, "sha256WithRSAEncryption", *got.KeyType)
	assert.Equal(t, []string{"example.com", "www.example.com"}, got.SANs)
	require.NotNil(t, got.NotBefore)
	assert.Equal(t, now, *got.NotBefore)
	require.NotNil(t, got.NotAfter)
	assert.Equal(t, later, *got.NotAfter)
}
