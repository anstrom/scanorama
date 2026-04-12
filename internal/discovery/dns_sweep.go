package discovery

import (
	"context"
	"log/slog"
	"net"
	"time"

	internaldns "github.com/anstrom/scanorama/internal/dns"
)

// dnsLookupTimeout is the per-IP deadline for a PTR lookup inside dnsSweep.
const dnsLookupTimeout = 3 * time.Second

// dnsSweep resolves PTR records for every IP in ipnet and returns results for
// IPs that had a successful PTR response. It uses the provided resolver with
// its built-in cache to deduplicate concurrent lookups.
//
// The per-IP timeout is fixed at dnsLookupTimeout. Overall context
// cancellation is respected.
func dnsSweep(ctx context.Context, ipnet net.IPNet, resolver *internaldns.Resolver, maxHosts int) []Result {
	ips := enumerateIPs(ipnet, maxHosts)
	if len(ips) == 0 {
		return nil
	}

	results := make([]Result, 0)

	for _, ip := range ips {
		if ctx.Err() != nil {
			break
		}
		ipStr := ip.String()

		lookupCtx, cancel := context.WithTimeout(ctx, dnsLookupTimeout)
		hostname, err := resolver.LookupAddr(lookupCtx, ipStr)
		cancel()

		if err != nil {
			continue // no PTR record — host not considered up
		}

		results = append(results, Result{
			IPAddress: ip,
			Status:    "up",
			Method:    "dns",
		})
		slog.Debug("dns sweep: PTR found", "ip", ipStr, "hostname", hostname)
	}

	return results
}

// enumerateIPs returns all usable host IPs in ipnet, capped at maxHosts.
func enumerateIPs(ipnet net.IPNet, maxHosts int) []net.IP {
	var ips []net.IP

	ip := cloneIP(ipnet.IP)
	// Skip network address (first IP).
	ip = nextIPAddr(ip)

	for ipnet.Contains(ip) {
		if maxHosts > 0 && len(ips) >= maxHosts {
			break
		}
		// Skip broadcast (all host bits = 1). Broadcast is the last IP; we
		// detect it by checking the next IP falls outside the network.
		next := nextIPAddr(ip)
		if !ipnet.Contains(next) {
			break // this is the broadcast address — stop before adding it
		}
		ips = append(ips, cloneIP(ip))
		ip = next
	}

	return ips
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

func nextIPAddr(ip net.IP) net.IP {
	next := cloneIP(ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}
