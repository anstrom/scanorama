// Package enrichment — unit tests for the service fingerprint library.
package enrichment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── NewFingerprinter ──────────────────────────────────────────────────────────

func TestNewFingerprinter_BuiltinsLoaded(t *testing.T) {
	fp := NewFingerprinter("")
	require.NotNil(t, fp)
	assert.NotEmpty(t, fp.rules, "built-in rules must be loaded")
}

func TestNewFingerprinter_NoExtraPath(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(22, "SSH-2.0-OpenSSH_8.9")
	assert.Equal(t, "OpenSSH", svc)
}

func TestNewFingerprinter_ExtraPathMissing(t *testing.T) {
	// A missing extra file must not crash — built-ins still loaded.
	fp := NewFingerprinter("/nonexistent/path.json")
	require.NotNil(t, fp)
	svc, _ := fp.Match(22, "SSH-2.0-OpenSSH_8.9")
	assert.Equal(t, "OpenSSH", svc)
}

func TestNewFingerprinter_ExtraPathLoaded(t *testing.T) {
	extra := []Fingerprint{
		{Port: 9999, Pattern: `(?i)MyCustomService`, Service: "CustomSvc", VersionPattern: `v([0-9.]+)`},
	}
	data, err := json.Marshal(extra)
	require.NoError(t, err)

	f, err := os.CreateTemp(t.TempDir(), "fp-*.json")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	f.Close()

	fp := NewFingerprinter(f.Name())
	svc, ver := fp.Match(9999, "MyCustomService v2.3.1")
	assert.Equal(t, "CustomSvc", svc)
	assert.Equal(t, "2.3.1", ver)
}

func TestNewFingerprinter_ExtraPathInvalidJSON(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "fp-*.json")
	require.NoError(t, err)
	_, _ = f.WriteString("not json at all")
	f.Close()

	// Invalid JSON must not crash — built-ins still loaded.
	fp := NewFingerprinter(f.Name())
	require.NotNil(t, fp)
	svc, _ := fp.Match(22, "SSH-2.0-OpenSSH_8.9")
	assert.Equal(t, "OpenSSH", svc)
}

// ── Match — service detection ─────────────────────────────────────────────────

func TestMatch_OpenSSH_WithVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(22, "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.6")
	assert.Equal(t, "OpenSSH", svc)
	assert.Equal(t, "8.9p1", ver)
}

func TestMatch_OpenSSH_NoVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(22, "SSH-2.0-dropbear_2022.83")
	assert.Equal(t, "OpenSSH", svc)
	assert.Empty(t, ver, "no OpenSSH version in dropbear banner")
}

func TestMatch_Redis(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(6379, "-NOAUTH Authentication required")
	assert.Equal(t, "Redis", svc)
}

func TestMatch_Redis_InfoBanner(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(0, "# Server\r\nredis_version:7.0.5\r\nredis_git_sha1:00000000\r\n")
	assert.Equal(t, "Redis", svc)
	assert.Equal(t, "7.0.5", ver)
}

func TestMatch_PostgreSQL(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(5432, "FATAL:  database \"foo\" does not exist")
	assert.Equal(t, "PostgreSQL", svc)
}

func TestMatch_MySQL(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(3306, "\x4a\x00\x00\x00\x0a8.0.32\x00")
	assert.Equal(t, "MySQL", svc)
}

func TestMatch_Memcached(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(11211, "VERSION 1.6.17\r\n")
	assert.Equal(t, "Memcached", svc)
	assert.Equal(t, "1.6.17", ver)
}

func TestMatch_RabbitMQ(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(5672, "AMQP\x00\x00\x09\x01")
	assert.Equal(t, "RabbitMQ", svc)
}

func TestMatch_SMB_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(445, "\x00\x00\x00\x85\xff\x53\x4d\x42")
	assert.Equal(t, "SMB", svc)
}

func TestMatch_RDP_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(3389, "\x03\x00\x00\x13\x0e\xd0\x00\x00")
	assert.Equal(t, "RDP", svc)
}

func TestMatch_Nginx(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(80, "HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\n")
	assert.Equal(t, "nginx", svc)
	assert.Equal(t, "1.24.0", ver)
}

func TestMatch_Apache(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(80, "HTTP/1.1 403 Forbidden\r\nServer: Apache/2.4.57 (Debian)\r\n")
	assert.Equal(t, "Apache", svc)
	assert.Equal(t, "2.4.57", ver)
}

func TestMatch_HTTP_Generic(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(8080, "HTTP/1.1 200 OK\r\n")
	assert.Equal(t, "HTTP", svc)
}

func TestMatch_FTP_vsftpd(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(21, "220 (vsFTPd 3.0.5)\r\n")
	assert.Equal(t, "FTP", svc)
	assert.Equal(t, "3.0.5", ver)
}

func TestMatch_FTP_ProFTPD(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(21, "220 ProFTPD 1.3.6 Server (Unix)")
	assert.Equal(t, "FTP", svc)
	assert.Equal(t, "1.3.6", ver)
}

func TestMatch_SMTP(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(25, "220 mail.example.com ESMTP Postfix")
	assert.Equal(t, "SMTP", svc)
}

func TestMatch_IMAP(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(143, "* OK [CAPABILITY IMAP4rev1] Dovecot ready.")
	assert.Equal(t, "IMAP", svc)
}

func TestMatch_POP3(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(110, "+OK POP3 server ready")
	assert.Equal(t, "POP3", svc)
}

func TestMatch_NoMatch(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(9999, "some random unrecognized banner")
	assert.Empty(t, svc)
	assert.Empty(t, ver)
}

func TestMatch_EmptyBanner_PortOnlyRule(t *testing.T) {
	// Port-only rules (Pattern="") match regardless of banner content.
	fp := NewFingerprinter("")
	svc, _ := fp.Match(445, "")
	assert.Equal(t, "SMB", svc)
}

func TestMatch_WrongPort_NoMatch(t *testing.T) {
	// Redis banner on a non-Redis port without a general redis pattern.
	fp := NewFingerprinter("")
	// port 9999 has no matching rule for this banner
	svc, _ := fp.Match(9999, "-NOAUTH Authentication required")
	assert.Empty(t, svc)
}

// ── loadFingerprintFile ───────────────────────────────────────────────────────

func TestLoadFingerprintFile_ValidJSON(t *testing.T) {
	rules := []Fingerprint{
		{Port: 1234, Pattern: `test`, Service: "TestSvc"},
	}
	data, _ := json.Marshal(rules)
	path := filepath.Join(t.TempDir(), "rules.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	loaded, err := loadFingerprintFile(path)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, "TestSvc", loaded[0].Service)
}

func TestLoadFingerprintFile_NotFound(t *testing.T) {
	_, err := loadFingerprintFile("/nonexistent/path.json")
	assert.Error(t, err)
}

func TestLoadFingerprintFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{bad json}"), 0o600))
	_, err := loadFingerprintFile(path)
	assert.Error(t, err)
}

// ── New infrastructure rules ──────────────────────────────────────────────────

func TestMatch_DNS_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(53, "")
	assert.Equal(t, "DNS", svc)
}

func TestMatch_DNS_BannerHint(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(53, "REFUSED")
	assert.Equal(t, "DNS", svc)
}

func TestMatch_DoT(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(853, "")
	assert.Equal(t, "DNS-over-TLS", svc)
}

func TestMatch_VNC_WithVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(5900, "RFB 003.008\n")
	assert.Equal(t, "VNC", svc)
	assert.Equal(t, "003.008", ver)
}

func TestMatch_LDAP_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(389, "")
	assert.Equal(t, "LDAP", svc)
}

func TestMatch_Telnet_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(23, "")
	assert.Equal(t, "Telnet", svc)
}

func TestMatch_DockerAPI(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(2375, `{"ApiVersion":"1.43","Os":"linux"}`)
	assert.Equal(t, "Docker API", svc)
}

func TestMatch_Prometheus(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(9090, "# HELP process_cpu_seconds_total")
	assert.Equal(t, "Prometheus", svc)
}

func TestMatch_NodeExporter(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(9100, "# HELP node_cpu_seconds_total\n# HELP go_gc_duration_seconds")
	assert.Equal(t, "Node Exporter", svc)
}

func TestMatch_ZooKeeper(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(2181, "imok")
	assert.Equal(t, "ZooKeeper", svc)
}

func TestMatch_CouchDB_WithVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(5984, `{"couchdb":"Welcome","version":"3.3.2"}`)
	assert.Equal(t, "CouchDB", svc)
	assert.Equal(t, "3.3.2", ver)
}

func TestMatch_SIP(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(5060, "SIP/2.0 200 OK\r\n")
	assert.Equal(t, "SIP", svc)
}

func TestMatch_Nginx_WithVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(443, "HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\n")
	assert.Equal(t, "nginx", svc)
	assert.Equal(t, "1.24.0", ver)
}

func TestMatch_IIS_WithVersion(t *testing.T) {
	fp := NewFingerprinter("")
	svc, ver := fp.Match(80, "HTTP/1.1 200 OK\r\nServer: Microsoft-IIS/10.0\r\n")
	assert.Equal(t, "IIS", svc)
	assert.Equal(t, "10.0", ver)
}

func TestMatch_Caddy(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(443, "HTTP/2 200\r\nServer: Caddy\r\n")
	assert.Equal(t, "Caddy", svc)
}

func TestMatch_HAProxy(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(80, "HTTP/1.0 200 OK\r\nServer: HAProxy\r\n")
	assert.Equal(t, "HAProxy", svc)
}

func TestMatch_MSSQL_PortOnly(t *testing.T) {
	fp := NewFingerprinter("")
	svc, _ := fp.Match(1433, "")
	assert.Equal(t, "MSSQL", svc)
}

// ── User override rules — prepend takes priority over built-ins ───────────────

func TestNewFingerprinter_UserRuleOverridesBuiltin(t *testing.T) {
	// A user rule on port 22 with a custom service name should win over the
	// built-in OpenSSH rule, because user rules are prepended before built-ins.
	extra := []Fingerprint{
		{Port: 22, Pattern: `(?i)^SSH-`, Service: "CustomSSH"},
	}
	data, err := json.Marshal(extra)
	require.NoError(t, err)

	f, err := os.CreateTemp(t.TempDir(), "fp-*.json")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	f.Close()

	fp := NewFingerprinter(f.Name())
	svc, _ := fp.Match(22, "SSH-2.0-OpenSSH_8.9")
	assert.Equal(t, "CustomSSH", svc, "user rules should override built-in OpenSSH rule")
}
