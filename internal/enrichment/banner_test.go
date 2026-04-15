// Package enrichment — unit tests for banner-grabbing helper functions.
package enrichment

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zmap/zgrab2"
	zgrabhttp_lib "github.com/zmap/zgrab2/lib/http"
	zgrabhttp "github.com/zmap/zgrab2/modules/http"
)

// ── parseBannerText ─────────────────────────────────────────────────────────

func TestParseBannerText_SSH(t *testing.T) {
	raw := "SSH-2.0-OpenSSH_8.9"
	svc, _ := parseBannerText("ssh-2.0-openssh_8.9", raw, 0)
	assert.Equal(t, "ssh", svc)
}

func TestParseBannerText_FTP(t *testing.T) {
	raw := "220 FTP Server ready"
	svc, _ := parseBannerText("220 ftp server ready", raw, 0)
	assert.Equal(t, "ftp", svc)
}

func TestParseBannerText_SMTP(t *testing.T) {
	raw := "220 mail.example.com ESMTP"
	svc, _ := parseBannerText("220 mail.example.com esmtp", raw, 0)
	assert.Equal(t, "smtp", svc)
}

func TestParseBannerText_POP3(t *testing.T) {
	raw := "+OK POP3 ready"
	svc, _ := parseBannerText("+ok pop3 ready", raw, 0)
	assert.Equal(t, "pop3", svc)
}

func TestParseBannerText_IMAP(t *testing.T) {
	raw := "* OK Dovecot ready"
	svc, _ := parseBannerText("* ok dovecot ready", raw, 0)
	assert.Equal(t, "imap", svc)
}

func TestParseBannerText_HTTP(t *testing.T) {
	raw := "HTTP/1.1 200 OK"
	svc, _ := parseBannerText("http/1.1 200 ok", raw, 0)
	assert.Equal(t, "http", svc)
}

func TestParseBannerText_Hint_FTP_Port21(t *testing.T) {
	raw := "welcome to the server"
	svc, _ := parseBannerText("welcome to the server", raw, 21)
	assert.Equal(t, portServiceHints[21], svc)
	assert.Equal(t, "ftp", svc)
}

func TestParseBannerText_Hint_SSH_Port22(t *testing.T) {
	raw := "some unrecognized banner"
	svc, _ := parseBannerText("some unrecognized banner", raw, 22)
	assert.Equal(t, portServiceHints[22], svc)
	assert.Equal(t, "ssh", svc)
}

func TestParseBannerText_Unknown(t *testing.T) {
	raw := "XYZ/3.0 custom protocol"
	svc, _ := parseBannerText("xyz/3.0 custom protocol", raw, 9999)
	assert.Equal(t, "", svc)
}

// ── keyTypeFromCert ─────────────────────────────────────────────────────────

func TestKeyTypeFromCert_RSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	cert := &x509.Certificate{PublicKey: &key.PublicKey}
	assert.Equal(t, "RSA", keyTypeFromCert(cert))
}

func TestKeyTypeFromCert_ECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{PublicKey: &key.PublicKey}
	assert.Equal(t, "ECDSA", keyTypeFromCert(cert))
}

func TestKeyTypeFromCert_Nil(t *testing.T) {
	assert.Equal(t, "", keyTypeFromCert(nil))
}

func TestKeyTypeFromCert_Unknown(t *testing.T) {
	// Use a non-RSA/ECDSA public key type — a raw string satisfies interface{}.
	cert := &x509.Certificate{PublicKey: "not-a-real-key"}
	assert.Equal(t, "", keyTypeFromCert(cert))
}

// ── findTLSLog ──────────────────────────────────────────────────────────────

// TestFindTLSLog_FromRedirectChain verifies that findTLSLog returns the TLS
// log from the redirect chain before checking the final response.
func TestFindTLSLog_FromRedirectChain(t *testing.T) {
	tlsLog := &zgrab2.TLSLog{}
	results := &zgrabhttp.Results{
		RedirectResponseChain: []*zgrabhttp_lib.Response{
			{Request: &zgrabhttp_lib.Request{TLSLog: tlsLog}},
		},
		Response: &zgrabhttp_lib.Response{}, // final response has no TLS
	}
	assert.Equal(t, tlsLog, findTLSLog(results), "redirect chain TLS log should take precedence")
}

// TestFindTLSLog_NilRequestInRedirectChain verifies that a nil Request in the
// redirect chain is skipped and findTLSLog falls through to the final response.
func TestFindTLSLog_NilRequestInRedirectChain(t *testing.T) {
	tlsLog := &zgrab2.TLSLog{}
	results := &zgrabhttp.Results{
		RedirectResponseChain: []*zgrabhttp_lib.Response{
			{Request: nil}, // no request — must be skipped
		},
		Response: &zgrabhttp_lib.Response{
			Request: &zgrabhttp_lib.Request{TLSLog: tlsLog},
		},
	}
	assert.Equal(t, tlsLog, findTLSLog(results))
}

// TestFindTLSLog_NilResponse verifies that nil Response returns nil.
func TestFindTLSLog_NilResponse(t *testing.T) {
	results := &zgrabhttp.Results{Response: nil}
	assert.Nil(t, findTLSLog(results))
}

// ── tlsVersionString ────────────────────────────────────────────────────────

func TestTLSVersionString(t *testing.T) {
	cases := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0000, fmt.Sprintf("TLS 0x%04X", 0x0000)},
		{0xABCD, fmt.Sprintf("TLS 0x%04X", 0xABCD)},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tlsVersionString(tc.version))
		})
	}
}
