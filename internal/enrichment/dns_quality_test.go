package enrichment_test

import (
	"testing"

	"github.com/anstrom/scanorama/internal/enrichment"
)

func TestDNSNameIsUsable(t *testing.T) {
	accept := []string{
		"pihole.local",
		"synology.home",
		"router.lan",
		"myserver.example.com",
		"nas.internal",
		"apple-tv.local",
	}
	reject := []string{
		"192-168-1-50.local",          // IP literal with hyphens
		"10.0.0.1",                    // bare IP
		"dhcp-192.example.com",        // dhcp- prefix
		"host-10-0-0-1.isp.net",       // host- prefix
		"ip-172-16-0-5.ec2.internal",  // ip- prefix
		"client-abcd.corp",            // client- prefix
		"1.0.168.192.in-addr.arpa",    // purely numeric labels (reversed IP)
		"broadband.12345.dynamic.net", // .dynamic. fragment
		"pool.dhcp.example.com",       // .dhcp. fragment
		"abc.broadband.isp.com",       // .broadband. fragment
		"",                            // empty
	}

	for _, name := range accept {
		t.Run("accept/"+name, func(t *testing.T) {
			if !enrichment.DNSNameIsUsable(name) {
				t.Errorf("expected %q to pass quality filter, but it was rejected", name)
			}
		})
	}
	for _, name := range reject {
		t.Run("reject/"+name, func(t *testing.T) {
			if enrichment.DNSNameIsUsable(name) {
				t.Errorf("expected %q to be rejected by quality filter, but it passed", name)
			}
		})
	}
}
