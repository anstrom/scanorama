package enrichment_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/enrichment"
)

// startFakeMDNSServer opens a random UDP port, serves one PTR response, then closes.
// It returns the port it is listening on.
func startFakeMDNSServer(t *testing.T, responseName string) int {
	t.Helper()

	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { pc.Close() })

	port := pc.LocalAddr().(*net.UDPAddr).Port

	go func() {
		buf := make([]byte, 512)
		n, remote, readErr := pc.ReadFrom(buf)
		if readErr != nil {
			return
		}
		req := new(dns.Msg)
		if parseErr := req.Unpack(buf[:n]); parseErr != nil || len(req.Question) == 0 {
			return
		}
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = append(resp.Answer, &dns.PTR{
			Hdr: dns.RR_Header{
				Name:   req.Question[0].Name,
				Rrtype: dns.TypePTR,
				Class:  dns.ClassINET,
				Ttl:    120,
			},
			Ptr: responseName + ".",
		})
		b, packErr := resp.Pack()
		if packErr != nil {
			return
		}
		_, _ = pc.WriteTo(b, remote)
	}()

	return port
}

func TestMDNSEnricher_Enrich_Success(t *testing.T) {
	port := startFakeMDNSServer(t, "mydevice.local")

	e := enrichment.NewMDNSEnricher(
		enrichment.WithMDNSTimeout(2*time.Second),
		enrichment.WithMDNSPort(port),
	)

	name, err := e.Enrich(context.Background(), "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(name, ".local"),
		"expected .local suffix, got %q", name)
}

func TestMDNSEnricher_Enrich_NoResponse(t *testing.T) {
	// Use port 1 — unreachable, should time out immediately and return ("", nil).
	e := enrichment.NewMDNSEnricher(
		enrichment.WithMDNSTimeout(100*time.Millisecond),
		enrichment.WithMDNSPort(1),
	)

	name, err := e.Enrich(context.Background(), "127.0.0.1")
	assert.NoError(t, err, "timeout must not be returned as an error")
	assert.Empty(t, name)
}

func TestMDNSEnricher_Enrich_ContextDeadlineRespected(t *testing.T) {
	e := enrichment.NewMDNSEnricher(
		enrichment.WithMDNSTimeout(5*time.Second),
		enrichment.WithMDNSPort(1),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	name, err := e.Enrich(ctx, "127.0.0.1")
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Empty(t, name)
	// Should have returned in under 1s, not waited the full 5s enricher timeout.
	assert.Less(t, elapsed, time.Second, "should respect context deadline")
}
