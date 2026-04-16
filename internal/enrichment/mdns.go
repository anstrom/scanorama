package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	mdnsDefaultTimeout = 2 * time.Second
	mdnsDefaultPort    = 5353
)

// MDNSEnricher queries a host directly (unicast) on its mDNS port using a
// DNS PTR query for the reversed-IP .in-addr.arpa name. Apple, Android, and
// Linux/avahi devices respond with a stable .local name.
//
// This is not multicast mDNS browsing — it is a targeted unicast query sent
// directly to the host IP. No listener is required.
type MDNSEnricher struct {
	timeout time.Duration
	port    int
	logger  *slog.Logger
}

// MDNSOption configures an MDNSEnricher.
type MDNSOption func(*MDNSEnricher)

// WithMDNSTimeout overrides the per-query deadline (default: 2s).
func WithMDNSTimeout(d time.Duration) MDNSOption {
	return func(e *MDNSEnricher) { e.timeout = d }
}

// WithMDNSPort overrides the destination port (default: 5353). Intended for
// testing only — production always uses port 5353.
func WithMDNSPort(p int) MDNSOption {
	return func(e *MDNSEnricher) { e.port = p }
}

// NewMDNSEnricher creates an MDNSEnricher with the given options.
func NewMDNSEnricher(opts ...MDNSOption) *MDNSEnricher {
	e := &MDNSEnricher{
		timeout: mdnsDefaultTimeout,
		port:    mdnsDefaultPort,
		logger:  slog.Default(),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Enrich sends a unicast DNS PTR query to <ip>:<port> and returns the resolved
// .local name, stripped of its trailing dot. Returns ("", nil) when the host
// does not respond, returns NXDOMAIN, or has no PTR record — a non-response is
// not treated as an error.
func (e *MDNSEnricher) Enrich(ctx context.Context, ip string) (string, error) {
	arpa, err := dns.ReverseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("mdns: build reverse addr for %s: %w", ip, err)
	}

	msg := new(dns.Msg)
	msg.SetQuestion(arpa, dns.TypePTR)
	msg.RecursionDesired = false

	timeout := e.timeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	client := &dns.Client{
		Net:     "udp",
		Timeout: timeout,
	}

	target := net.JoinHostPort(ip, fmt.Sprintf("%d", e.port))
	resp, _, err := client.Exchange(msg, target)
	if err != nil {
		// Timeout or connection refused — the host simply does not support mDNS.
		e.logger.Debug("mdns: no response", "ip", ip, "error", err)
		return "", nil
	}
	if resp == nil || len(resp.Answer) == 0 {
		return "", nil
	}

	for _, rr := range resp.Answer {
		if ptr, ok := rr.(*dns.PTR); ok {
			name := strings.TrimSuffix(ptr.Ptr, ".")
			e.logger.Debug("mdns: resolved", "ip", ip, "name", name)
			return name, nil
		}
	}
	return "", nil
}
