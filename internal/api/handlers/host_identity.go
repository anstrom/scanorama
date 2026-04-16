package handlers

import (
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/services"
)

// basicIdentityInputs builds an IdentityInputs snapshot using only fields that
// live on the Host row itself — no joins against host_dns_records,
// certificates, or host_snmp_data. hosts.hostname is passed as a PTR
// observation because that's the historical source (the DNS enricher was the
// sole writer before migration 027 added hostname_source). The quality filter
// (DNSNameIsUsable) still gates it, so garbage ISP PTR values are rejected
// correctly even if hostname_source is NULL.
//
// Use fullIdentityInputs when the caller has already fetched DNS records,
// cert subjects and SNMP data (e.g. GetHost).
func basicIdentityInputs(host *db.Host) services.IdentityInputs {
	in := services.IdentityInputs{
		IPAddress:  host.IPAddress.String(),
		CustomName: host.CustomName,
		MDNSName:   host.MDNSName,
		MDNSSeenAt: host.LastSeen,
	}
	if host.Hostname != nil && *host.Hostname != "" {
		in.PTRRecords = []services.PTRObservation{
			{Name: *host.Hostname, ObservedAt: host.LastSeen},
		}
	}
	return in
}

// fullIdentityInputs layers the related-table data fetched by GetHost on top
// of basicIdentityInputs so the Identity tab can render every observed name
// candidate — including the unusable ones with explanations.
//
// Cert subjects are reported with ForwardMatchesIP=false by default: verifying
// the forward A/AAAA lookup against the host IP requires live DNS and is not
// performed synchronously on the hot GetHost path. Unverified certs therefore
// appear in the candidate list as unusable with a reason, and never win the
// display_name race.
func fullIdentityInputs(
	host *db.Host,
	dnsRecords []db.DNSRecord,
	certs []*db.Certificate,
	snmp *db.HostSNMPData,
) services.IdentityInputs {
	in := basicIdentityInputs(host)

	if ptrs := ptrObservationsFromDNS(dnsRecords); len(ptrs) > 0 {
		// Replace the single hostname-as-PTR fallback from basicIdentityInputs
		// with the authoritative list from host_dns_records.
		in.PTRRecords = ptrs
	}

	if snmp != nil && snmp.SysName != nil && *snmp.SysName != "" {
		in.SNMPSysName = snmp.SysName
		if !snmp.CollectedAt.IsZero() {
			in.SNMPSeenAt = snmp.CollectedAt
		}
	}

	in.CertSubjects = certSubjectsFromCerts(certs)
	return in
}

// ptrObservationsFromDNS extracts PTR records from the host's DNS record set.
// Non-PTR rows and PTR rows with empty values are skipped.
func ptrObservationsFromDNS(dnsRecords []db.DNSRecord) []services.PTRObservation {
	out := make([]services.PTRObservation, 0, len(dnsRecords))
	for i := range dnsRecords {
		r := dnsRecords[i]
		if r.RecordType != "PTR" || r.Value == "" {
			continue
		}
		out = append(out, services.PTRObservation{
			Name:       r.Value,
			ObservedAt: r.ResolvedAt,
		})
	}
	return out
}

// certSubjectsFromCerts flattens certs into subject entries (CN first, then
// each SAN). ForwardMatchesIP is always false — verification is deferred.
func certSubjectsFromCerts(certs []*db.Certificate) []services.CertSubject {
	out := make([]services.CertSubject, 0, len(certs))
	for _, c := range certs {
		if c == nil {
			continue
		}
		observed := c.ScannedAt
		if c.SubjectCN != nil && *c.SubjectCN != "" {
			out = append(out, services.CertSubject{
				Name:             *c.SubjectCN,
				Kind:             "cn",
				ObservedAt:       observed,
				ForwardMatchesIP: false,
			})
		}
		for _, san := range c.SANs {
			if san == "" {
				continue
			}
			out = append(out, services.CertSubject{
				Name:             san,
				Kind:             "san",
				ObservedAt:       observed,
				ForwardMatchesIP: false,
			})
		}
	}
	return out
}

// toNameCandidateResponses converts resolver candidates to the API shape,
// converting zero-valued ObservedAt timestamps to nil so they serialize as
// omitted rather than "0001-01-01T00:00:00Z".
func toNameCandidateResponses(cs []services.NameCandidate) []NameCandidateResponse {
	out := make([]NameCandidateResponse, 0, len(cs))
	for _, c := range cs {
		r := NameCandidateResponse{
			Name:            c.Name,
			Source:          string(c.Source),
			Usable:          c.Usable,
			NotUsableReason: c.NotUsableReason,
		}
		if !c.ObservedAt.IsZero() {
			t := c.ObservedAt
			r.ObservedAt = &t
		}
		out = append(out, r)
	}
	return out
}

// resolveAndListCandidates builds the display_name / candidate list pair for
// a host given its related data, and writes them onto the response. Intended
// for the GET /hosts/{id} path where all identity-related tables are joined.
func (h *HostHandler) augmentWithFullIdentity(
	resp *HostResponse,
	host *db.Host,
	dnsRecords []db.DNSRecord,
	certs []*db.Certificate,
	snmp *db.HostSNMPData,
) {
	inputs := fullIdentityInputs(host, dnsRecords, certs, snmp)
	resolution := services.ResolveDisplayName(inputs, services.DefaultIdentityRankOrder)
	resp.DisplayName = resolution.Name
	resp.DisplayNameSource = string(resolution.Source)
	resp.NameCandidates = toNameCandidateResponses(services.ListNameCandidates(inputs))
}
