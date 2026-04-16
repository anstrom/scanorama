package services

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultRankOrder() []string { return []string{"mdns", "snmp", "ptr", "cert"} }

func TestResolveDisplayName_CustomWinsOverEverything(t *testing.T) {
	in := IdentityInputs{
		IPAddress:    "192.168.1.10",
		CustomName:   strPtr("my-laptop"),
		MDNSName:     strPtr("sams-macbook.local"),
		SNMPSysName:  strPtr("laptop-snmp"),
		PTRRecords:   []PTRObservation{{Name: "laptop.example.com", ObservedAt: time.Now()}},
		CertSubjects: []CertSubject{{Name: "laptop.example.com", ForwardMatchesIP: true}},
	}

	r := ResolveDisplayName(in, defaultRankOrder())

	assert.Equal(t, "my-laptop", r.Name)
	assert.Equal(t, SourceCustom, r.Source)
	assert.InDelta(t, confidenceCustom, r.Confidence, 0.0001)
}

func TestResolveDisplayName_CustomWhitespaceOnlyIsIgnored(t *testing.T) {
	in := IdentityInputs{
		IPAddress:  "192.168.1.10",
		CustomName: strPtr("   "),
		MDNSName:   strPtr("laptop.local"),
	}

	r := ResolveDisplayName(in, defaultRankOrder())

	assert.Equal(t, "laptop.local", r.Name)
	assert.Equal(t, SourceMDNS, r.Source)
}

func TestResolveDisplayName_CustomTrimmed(t *testing.T) {
	in := IdentityInputs{
		IPAddress:  "192.168.1.10",
		CustomName: strPtr("  hub  "),
	}

	r := ResolveDisplayName(in, defaultRankOrder())

	assert.Equal(t, "hub", r.Name)
	assert.Equal(t, SourceCustom, r.Source)
}

func TestResolveDisplayName_RankOrderRespected(t *testing.T) {
	in := IdentityInputs{
		IPAddress:   "192.168.1.10",
		MDNSName:    strPtr("laptop.local"),
		SNMPSysName: strPtr("laptop-snmp"),
	}

	mdnsFirst := ResolveDisplayName(in, []string{"mdns", "snmp"})
	assert.Equal(t, "laptop.local", mdnsFirst.Name)
	assert.Equal(t, SourceMDNS, mdnsFirst.Source)

	snmpFirst := ResolveDisplayName(in, []string{"snmp", "mdns"})
	assert.Equal(t, "laptop-snmp", snmpFirst.Name)
	assert.Equal(t, SourceSNMP, snmpFirst.Source)
}

func TestResolveDisplayName_SkipsUnusableMDNS(t *testing.T) {
	in := IdentityInputs{
		IPAddress:   "192.168.1.10",
		MDNSName:    strPtr("laptop"), // missing .local suffix
		SNMPSysName: strPtr("fallback"),
	}

	r := ResolveDisplayName(in, defaultRankOrder())

	assert.Equal(t, "fallback", r.Name)
	assert.Equal(t, SourceSNMP, r.Source)
}

func TestResolveDisplayName_PTRQualityFilterRejects(t *testing.T) {
	in := IdentityInputs{
		IPAddress: "10.1.2.3",
		PTRRecords: []PTRObservation{
			{Name: "dhcp-1-2-3.dynamic.comcast.net", ObservedAt: time.Now()},
		},
	}

	r := ResolveDisplayName(in, []string{"ptr"})

	assert.Equal(t, "10.1.2.3", r.Name, "quality-filtered PTR should fall through to IP")
	assert.Equal(t, SourceIP, r.Source)
}

func TestResolveDisplayName_PTRFirstUsableWins(t *testing.T) {
	now := time.Now()
	in := IdentityInputs{
		IPAddress: "10.1.2.3",
		PTRRecords: []PTRObservation{
			{Name: "dhcp-1-2-3.dynamic.comcast.net", ObservedAt: now},
			{Name: "laptop.example.com", ObservedAt: now},
		},
	}

	r := ResolveDisplayName(in, []string{"ptr"})

	assert.Equal(t, "laptop.example.com", r.Name)
	assert.Equal(t, SourcePTR, r.Source)
}

func TestResolveDisplayName_CertRequiresReverseMatch(t *testing.T) {
	in := IdentityInputs{
		IPAddress: "10.1.2.3",
		CertSubjects: []CertSubject{
			{Name: "unrelated.example.com", Kind: "cn", ForwardMatchesIP: false},
		},
	}

	skipped := ResolveDisplayName(in, []string{"cert"})
	assert.Equal(t, SourceIP, skipped.Source)

	in.CertSubjects[0].ForwardMatchesIP = true
	matched := ResolveDisplayName(in, []string{"cert"})
	assert.Equal(t, "unrelated.example.com", matched.Name)
	assert.Equal(t, SourceCert, matched.Source)
}

func TestResolveDisplayName_UnknownSourceSilentlySkipped(t *testing.T) {
	in := IdentityInputs{
		IPAddress: "192.168.1.10",
		MDNSName:  strPtr("laptop.local"),
	}

	r := ResolveDisplayName(in, []string{"made-up-source", "", "mdns"})

	assert.Equal(t, "laptop.local", r.Name)
	assert.Equal(t, SourceMDNS, r.Source)
}

func TestResolveDisplayName_FallbackToIP(t *testing.T) {
	in := IdentityInputs{IPAddress: "10.1.2.3"}

	r := ResolveDisplayName(in, defaultRankOrder())

	assert.Equal(t, "10.1.2.3", r.Name)
	assert.Equal(t, SourceIP, r.Source)
	assert.InDelta(t, confidenceIP, r.Confidence, 0.0001)
}

func TestResolveDisplayName_EmptyRankOrderFallsThroughToIP(t *testing.T) {
	in := IdentityInputs{
		IPAddress: "10.1.2.3",
		MDNSName:  strPtr("laptop.local"),
	}

	r := ResolveDisplayName(in, nil)

	assert.Equal(t, "10.1.2.3", r.Name)
	assert.Equal(t, SourceIP, r.Source)
}

func TestValidateSNMPSysName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantUsable bool
		wantReason string
	}{
		{"normal", "switch-01", true, ""},
		{"with spaces", "Office Printer 3", true, ""},
		{
			"too long",
			strings.Repeat("a", maxSNMPSysNameLen+1),
			false,
			"sys_name too long",
		},
		{"contains tab", "switch\t01", false, "sys_name contains non-printable characters"},
		{"contains null byte", "switch\x00", false, "sys_name contains non-printable characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			usable, reason := validateSNMPSysName(tc.input)
			assert.Equal(t, tc.wantUsable, usable)
			assert.Equal(t, tc.wantReason, reason)
		})
	}
}

func TestListNameCandidates_ExcludesCustomName(t *testing.T) {
	in := IdentityInputs{
		IPAddress:  "10.1.2.3",
		CustomName: strPtr("alias"),
		MDNSName:   strPtr("laptop.local"),
	}

	cs := ListNameCandidates(in)

	for _, c := range cs {
		assert.NotEqual(t, SourceCustom, c.Source, "custom_name must not appear in candidate list")
	}
	require.Len(t, cs, 1)
	assert.Equal(t, "laptop.local", cs[0].Name)
}

func TestListNameCandidates_StableOrderMDNSSNMPPTRCert(t *testing.T) {
	now := time.Now()
	in := IdentityInputs{
		IPAddress:   "10.1.2.3",
		MDNSName:    strPtr("laptop.local"),
		SNMPSysName: strPtr("laptop-snmp"),
		PTRRecords: []PTRObservation{
			{Name: "laptop.example.com", ObservedAt: now},
		},
		CertSubjects: []CertSubject{
			{Name: "laptop.example.com", Kind: "cn", ForwardMatchesIP: true, ObservedAt: now},
		},
	}

	cs := ListNameCandidates(in)

	require.Len(t, cs, 4)
	assert.Equal(t, SourceMDNS, cs[0].Source)
	assert.Equal(t, SourceSNMP, cs[1].Source)
	assert.Equal(t, SourcePTR, cs[2].Source)
	assert.Equal(t, SourceCert, cs[3].Source)
}

func TestListNameCandidates_ReturnsEmptySliceNotNil(t *testing.T) {
	in := IdentityInputs{IPAddress: "10.1.2.3"}

	cs := ListNameCandidates(in)

	require.NotNil(t, cs, "must be empty slice not nil so JSON encodes as [] not null")
	assert.Len(t, cs, 0)
}

func TestListNameCandidates_IncludesUnusableWithReason(t *testing.T) {
	now := time.Now()
	in := IdentityInputs{
		IPAddress: "10.1.2.3",
		MDNSName:  strPtr("bare-mdns"), // missing .local
		PTRRecords: []PTRObservation{
			{Name: "dhcp-1-2-3.dynamic.comcast.net", ObservedAt: now},
		},
		CertSubjects: []CertSubject{
			{Name: "unrelated.example.com", Kind: "cn", ForwardMatchesIP: false, ObservedAt: now},
		},
	}

	cs := ListNameCandidates(in)

	require.Len(t, cs, 3)

	mdns := cs[0]
	assert.Equal(t, SourceMDNS, mdns.Source)
	assert.False(t, mdns.Usable)
	assert.Contains(t, mdns.NotUsableReason, ".local")

	ptr := cs[1]
	assert.Equal(t, SourcePTR, ptr.Source)
	assert.False(t, ptr.Usable)
	assert.Contains(t, ptr.NotUsableReason, "PTR pattern")

	cert := cs[2]
	assert.Equal(t, SourceCert, cert.Source)
	assert.False(t, cert.Usable)
	assert.Contains(t, cert.NotUsableReason, "forward-resolve")
}

func TestListNameCandidates_SNMPEmptyOrNilProducesNothing(t *testing.T) {
	in := IdentityInputs{IPAddress: "10.1.2.3", SNMPSysName: strPtr("")}

	cs := ListNameCandidates(in)

	assert.Empty(t, cs)
}

func TestConfidenceFor(t *testing.T) {
	tests := []struct {
		source Source
		want   float64
	}{
		{SourceCustom, confidenceCustom},
		{SourceMDNS, confidenceMDNS},
		{SourceSNMP, confidenceSNMP},
		{SourcePTR, confidencePTR},
		{SourceCert, confidenceCert},
		{SourceIP, confidenceIP},
		{Source("unknown"), confidenceIP},
	}
	for _, tc := range tests {
		t.Run(string(tc.source), func(t *testing.T) {
			assert.InDelta(t, tc.want, confidenceFor(tc.source), 0.0001)
		})
	}
}
