// Package enrichment provides post-scan host enrichment: reverse DNS lookup,
// DNS record collection, and related operations.
package enrichment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	internaldns "github.com/anstrom/scanorama/internal/dns"
)

// netLookups is the subset of *net.Resolver used by forwardRecords. It allows
// tests to inject a fake resolver without starting a real DNS server.
type netLookups interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupMX(ctx context.Context, host string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, host string) ([]string, error)
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// DNSEnricher enriches hosts with DNS records collected after discovery or scanning.
//
// It performs:
//   - A PTR (reverse) lookup for the host IP to discover its hostname.
//   - If a hostname is found and the host has none set, it writes it back to hosts.hostname.
//   - Forward A/AAAA lookups on the discovered hostname.
//   - TXT and MX record collection on the discovered hostname.
//
// All results are stored in host_dns_records (replacing any previous records).
type DNSEnricher struct {
	resolver    *internaldns.Resolver
	netResolver netLookups
	dnsRepo     *db.DNSRepository
	hostRepo    *db.HostRepository
	logger      *slog.Logger
}

// NewDNSEnricher creates a DNSEnricher.
func NewDNSEnricher(
	resolver *internaldns.Resolver,
	dnsRepo *db.DNSRepository,
	hostRepo *db.HostRepository,
) *DNSEnricher {
	return &DNSEnricher{
		resolver:    resolver,
		netResolver: net.DefaultResolver,
		dnsRepo:     dnsRepo,
		hostRepo:    hostRepo,
		logger:      slog.Default(),
	}
}

// WithNetResolver overrides the net.Resolver used for CNAME, MX, TXT, and SRV
// lookups. The primary use-case is unit tests.
func (e *DNSEnricher) WithNetResolver(r netLookups) *DNSEnricher {
	e.netResolver = r
	return e
}

// EnrichHost performs DNS enrichment for a single host. ctx should carry a
// suitable deadline (the caller is responsible for timeout management).
func (e *DNSEnricher) EnrichHost(ctx context.Context, host *db.Host) error {
	ip := host.IPAddress.String()

	var records []db.DNSRecord

	// PTR lookup — use the cached resolver for deduplication.
	hostname, ptrErr := e.resolver.LookupAddr(ctx, ip)
	if ptrErr == nil && hostname != "" {
		records = append(records, db.DNSRecord{
			RecordType: "PTR",
			Value:      hostname,
		})
		e.maybeSetHostname(ctx, host, hostname)
	} else if ptrErr != nil && !errors.Is(ptrErr, internaldns.ErrNoRecords) {
		e.logger.Debug("enrichment: PTR lookup failed",
			"ip", ip, "error", ptrErr)
	}

	// Forward lookups only make sense if we have a resolved hostname.
	if hostname != "" {
		records = append(records, e.forwardRecords(ctx, host.ID, hostname)...)
	}

	if len(records) == 0 {
		return nil
	}
	return e.dnsRepo.UpsertDNSRecords(ctx, host.ID, records)
}

// EnrichHosts runs EnrichHost for each host in the slice. Errors from
// individual hosts are logged but do not abort the remaining enrichments.
func (e *DNSEnricher) EnrichHosts(ctx context.Context, hosts []*db.Host) {
	for _, h := range hosts {
		if ctx.Err() != nil {
			return
		}
		// Per-host timeout so one slow host can't block the whole batch.
		hCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := e.EnrichHost(hCtx, h); err != nil {
			e.logger.Warn("enrichment: DNS enrichment failed",
				"ip", h.IPAddress.String(), "error", err)
		}
		cancel()
	}
}

// maybeSetHostname writes hostname back to hosts.hostname when the host
// currently has none. Non-fatal: a write failure is only logged.
func (e *DNSEnricher) maybeSetHostname(ctx context.Context, host *db.Host, hostname string) {
	if host.Hostname != nil && *host.Hostname != "" {
		return
	}
	if _, err := e.hostRepo.UpdateHost(ctx, host.ID, db.UpdateHostInput{
		Hostname: &hostname,
	}); err != nil {
		e.logger.Warn("enrichment: failed to write PTR hostname",
			"host_id", host.ID, "hostname", hostname, "error", err)
		return
	}
	e.logger.Debug("enrichment: set hostname from PTR",
		"host_id", host.ID, "hostname", hostname)
}

// forwardRecords collects A, AAAA, CNAME, MX, TXT, and SRV records for the given hostname.
func (e *DNSEnricher) forwardRecords(ctx context.Context, hostID uuid.UUID, hostname string) []db.DNSRecord {
	var records []db.DNSRecord

	// A / AAAA — use the cached resolver for deduplication across concurrent enrichments.
	addrs, err := e.resolver.LookupHost(ctx, hostname)
	if err == nil {
		for _, addr := range addrs {
			rtype := "A"
			if strings.Contains(addr, ":") {
				rtype = "AAAA"
			}
			records = append(records, db.DNSRecord{
				HostID:     hostID,
				RecordType: rtype,
				Value:      addr,
			})
		}
	}

	// CNAME — the canonical name; omit if it equals the query name (no alias).
	cname, err := e.netResolver.LookupCNAME(ctx, hostname)
	if err == nil {
		cname = strings.TrimSuffix(cname, ".")
		if cname != "" && cname != hostname {
			records = append(records, db.DNSRecord{
				HostID:     hostID,
				RecordType: "CNAME",
				Value:      cname,
			})
		}
	}

	// MX
	mxRecords, err := e.netResolver.LookupMX(ctx, hostname)
	if err == nil {
		for _, mx := range mxRecords {
			records = append(records, db.DNSRecord{
				HostID:     hostID,
				RecordType: "MX",
				Value:      strings.TrimSuffix(mx.Host, "."),
			})
		}
	}

	// TXT
	txts, err := e.netResolver.LookupTXT(ctx, hostname)
	if err == nil {
		for _, txt := range txts {
			records = append(records, db.DNSRecord{
				HostID:     hostID,
				RecordType: "TXT",
				Value:      txt,
			})
		}
	}

	// SRV — probe well-known service prefixes. We deliberately skip "service"
	// and "proto" discovery; the goal is best-effort population for common services.
	//
	// net.Resolver.LookupSRV prepends underscores automatically, so the service
	// and proto values must NOT include a leading underscore.
	srvServices := []struct{ service, proto string }{
		{"http", "tcp"},
		{"https", "tcp"},
		{"smtp", "tcp"},
		{"imap", "tcp"},
		{"imaps", "tcp"},
		{"ldap", "tcp"},
		{"ldaps", "tcp"},
		{"xmpp-client", "tcp"},
		{"xmpp-server", "tcp"},
	}
	for _, s := range srvServices {
		_, addrs, err := e.netResolver.LookupSRV(ctx, s.service, s.proto, hostname)
		if err != nil {
			continue
		}
		for _, srv := range addrs {
			records = append(records, db.DNSRecord{
				HostID:     hostID,
				RecordType: "SRV",
				Value: fmt.Sprintf("_%s._%s.%s %d %d %d",
					s.service, s.proto, strings.TrimSuffix(srv.Target, "."),
					srv.Priority, srv.Weight, srv.Port),
			})
		}
	}

	return records
}
