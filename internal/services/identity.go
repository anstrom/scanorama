// Package services contains the scanorama application-layer orchestration.
// This file holds the host-identity resolver: given a snapshot of every
// name-like signal observed for a host, it picks one canonical display
// name and enumerates all the alternatives for the Identity tab.
package services

import (
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/enrichment"
)

// Source identifies where a host name candidate came from.
type Source string

// Known identity sources. SourceIP is the final fallback when nothing else
// produced a usable name.
const (
	SourceCustom Source = "custom"
	SourceMDNS   Source = "mdns"
	SourceSNMP   Source = "snmp"
	SourcePTR    Source = "ptr"
	SourceCert   Source = "cert"
	SourceIP     Source = "ip"
)

// DefaultIdentityRankOrder matches the seeded identity.rank_order setting
// (migration 027). Callers that can't reach the settings row should use this
// slice so behavior stays consistent with a fresh install.
//
// Don't take &DefaultIdentityRankOrder — it's meant to be copied or ranged
// over, not mutated.
var DefaultIdentityRankOrder = []string{"mdns", "snmp", "ptr", "cert"}

const (
	maxSNMPSysNameLen = 253
	minPrintableASCII = 0x20
	maxPrintableASCII = 0x7E

	confidenceCustom = 1.0
	confidenceMDNS   = 0.9
	confidenceSNMP   = 0.8
	confidencePTR    = 0.6
	confidenceCert   = 0.5
	confidenceIP     = 0.0
)

// IdentityResolution is the single winning display name for a host.
type IdentityResolution struct {
	Name       string
	Source     Source
	Confidence float64
}

// NameCandidate is one row of the Identity tab table. Unusable candidates
// are still included so the UI can show users why a given value was not
// promoted.
type NameCandidate struct {
	Name            string
	Source          Source
	Usable          bool
	NotUsableReason string
	ObservedAt      time.Time
}

// PTRObservation is a single PTR record with its observation timestamp.
type PTRObservation struct {
	Name       string
	ObservedAt time.Time
}

// CertSubject is a TLS cert CN or SAN entry with pre-computed reverse-match
// information. ForwardMatchesIP is true when a forward A/AAAA lookup of
// Name resolves back to the host's IP — the only case where the cert
// identity can be trusted as the host's identity.
type CertSubject struct {
	Name             string
	Kind             string // "cn" or "san"
	ObservedAt       time.Time
	ForwardMatchesIP bool
}

// IdentityInputs is the complete input to the resolver. Callers assemble
// it by querying the DB (and for cert subjects, forward-resolving CN/SAN
// values to compare against the host IP) before invoking ResolveDisplayName
// or ListNameCandidates.
type IdentityInputs struct {
	IPAddress string

	CustomName *string

	MDNSName   *string
	MDNSSeenAt time.Time

	SNMPSysName *string
	SNMPSeenAt  time.Time

	PTRRecords   []PTRObservation
	CertSubjects []CertSubject
}

// ResolveDisplayName picks the winning display name by walking rankOrder
// (for example ["mdns","snmp","ptr","cert"]) and returning the first usable
// candidate from each source. A non-empty CustomName always wins, and the
// host IP is the last-resort fallback. Unknown source names in rankOrder
// are silently skipped so operator-edited config can't crash the resolver.
func ResolveDisplayName(in IdentityInputs, rankOrder []string) IdentityResolution {
	if name, ok := trimmedCustomName(in.CustomName); ok {
		return IdentityResolution{Name: name, Source: SourceCustom, Confidence: confidenceCustom}
	}

	for _, sourceName := range rankOrder {
		c := pickFirstUsable(candidatesFrom(in, Source(sourceName)))
		if c == nil {
			continue
		}
		return IdentityResolution{Name: c.Name, Source: c.Source, Confidence: confidenceFor(c.Source)}
	}

	return IdentityResolution{Name: in.IPAddress, Source: SourceIP, Confidence: confidenceIP}
}

// ListNameCandidates enumerates every automatic name candidate for a host,
// usable or not, in a stable order (mdns, snmp, ptr, cert). CustomName is
// intentionally excluded — the API surfaces it as its own field because the
// UI renders it in a separate input, not in the candidate table.
func ListNameCandidates(in IdentityInputs) []NameCandidate {
	// Upper-bound: at most one row each from mdns and snmp, plus one per
	// PTR record and cert subject.
	upperBound := 2 + len(in.PTRRecords) + len(in.CertSubjects)
	out := make([]NameCandidate, 0, upperBound)
	for _, source := range []Source{SourceMDNS, SourceSNMP, SourcePTR, SourceCert} {
		out = append(out, candidatesFrom(in, source)...)
	}
	return out
}

func confidenceFor(s Source) float64 {
	switch s {
	case SourceCustom:
		return confidenceCustom
	case SourceMDNS:
		return confidenceMDNS
	case SourceSNMP:
		return confidenceSNMP
	case SourcePTR:
		return confidencePTR
	case SourceCert:
		return confidenceCert
	case SourceIP:
		return confidenceIP
	}
	return confidenceIP
}

func pickFirstUsable(cs []NameCandidate) *NameCandidate {
	for i := range cs {
		if cs[i].Usable {
			return &cs[i]
		}
	}
	return nil
}

func candidatesFrom(in IdentityInputs, source Source) []NameCandidate {
	switch source {
	case SourceMDNS:
		return mdnsCandidates(in)
	case SourceSNMP:
		return snmpCandidates(in)
	case SourcePTR:
		return ptrCandidates(in)
	case SourceCert:
		return certCandidates(in)
	case SourceCustom, SourceIP:
		// Not enumerable through the auto-candidate path.
		return nil
	}
	return nil
}

func trimmedCustomName(p *string) (string, bool) {
	if p == nil {
		return "", false
	}
	name := strings.TrimSpace(*p)
	if name == "" {
		return "", false
	}
	return name, true
}

func mdnsCandidates(in IdentityInputs) []NameCandidate {
	if in.MDNSName == nil {
		return nil
	}
	name := strings.TrimSpace(*in.MDNSName)
	if name == "" {
		return nil
	}
	usable := strings.HasSuffix(strings.ToLower(name), ".local")
	c := NameCandidate{Name: name, Source: SourceMDNS, Usable: usable, ObservedAt: in.MDNSSeenAt}
	if !usable {
		c.NotUsableReason = "mDNS name does not end in .local"
	}
	return []NameCandidate{c}
}

func snmpCandidates(in IdentityInputs) []NameCandidate {
	if in.SNMPSysName == nil {
		return nil
	}
	name := strings.TrimSpace(*in.SNMPSysName)
	if name == "" {
		return nil
	}
	usable, reason := validateSNMPSysName(name)
	c := NameCandidate{Name: name, Source: SourceSNMP, Usable: usable, ObservedAt: in.SNMPSeenAt}
	if !usable {
		c.NotUsableReason = reason
	}
	return []NameCandidate{c}
}

func validateSNMPSysName(name string) (usable bool, reason string) {
	if len(name) > maxSNMPSysNameLen {
		return false, "sys_name too long"
	}
	for _, r := range name {
		if r < minPrintableASCII || r > maxPrintableASCII {
			return false, "sys_name contains non-printable characters"
		}
	}
	return true, ""
}

func ptrCandidates(in IdentityInputs) []NameCandidate {
	if len(in.PTRRecords) == 0 {
		return nil
	}
	out := make([]NameCandidate, 0, len(in.PTRRecords))
	for _, r := range in.PTRRecords {
		usable := enrichment.DNSNameIsUsable(r.Name)
		c := NameCandidate{Name: r.Name, Source: SourcePTR, Usable: usable, ObservedAt: r.ObservedAt}
		if !usable {
			c.NotUsableReason = "filtered: unusable PTR pattern"
		}
		out = append(out, c)
	}
	return out
}

func certCandidates(in IdentityInputs) []NameCandidate {
	if len(in.CertSubjects) == 0 {
		return nil
	}
	out := make([]NameCandidate, 0, len(in.CertSubjects))
	for _, s := range in.CertSubjects {
		c := NameCandidate{Name: s.Name, Source: SourceCert, Usable: s.ForwardMatchesIP, ObservedAt: s.ObservedAt}
		if !s.ForwardMatchesIP {
			c.NotUsableReason = "cert subject does not forward-resolve to host IP"
		}
		out = append(out, c)
	}
	return out
}
