// Package scanning — NSE script output parsing for nmap XML results.
package scanning

import (
	"fmt"
	"strings"
	"time"

	nmap "github.com/Ullaakut/nmap/v3"
)

// parsePortScripts extracts structured NSE data from nmap port-level scripts.
// Returns nil when no scripts are present or none contain actionable data.
func parsePortScripts(scripts []nmap.Script) *NSEData {
	if len(scripts) == 0 {
		return nil
	}
	out := &NSEData{}
	for i := range scripts {
		s := &scripts[i]
		switch s.ID {
		case "banner":
			out.Banner = strings.TrimSpace(s.Output)
		case "http-title":
			out.HTTPTitle = extractHTTPTitle(s)
		case "ssh-hostkey":
			out.SSHKeyFingerprint = extractSSHFingerprint(s)
		case "ssl-cert":
			out.SSLCert = parseSSLCert(s)
		}
	}
	if out.isEmpty() {
		return nil
	}
	return out
}

// parseHostScripts processes nmap host-level scripts and updates the host in place.
// Host-level scripts (smb-os-discovery, nbstat) appear in the <hostscript> element
// rather than under individual ports.
func parseHostScripts(scripts []nmap.Script, host *Host) {
	for i := range scripts {
		s := &scripts[i]
		switch s.ID {
		case "smb-os-discovery":
			applySSBOSDiscovery(s, host)
		case "nbstat":
			if name := findElem(s.Elements, "NetBIOS_Computer_Name"); name != "" && host.SMBHostname == "" {
				host.SMBHostname = name
			}
		}
	}
}

// extractHTTPTitle returns the page title from an http-title script output.
func extractHTTPTitle(s *nmap.Script) string {
	title := strings.TrimSpace(s.Output)
	// Filter out nmap error messages that aren't real titles.
	if strings.HasPrefix(title, "ERROR:") ||
		strings.HasPrefix(title, "Did not follow redirect") {
		return ""
	}
	return title
}

// extractSSHFingerprint returns the first key fingerprint line from ssh-hostkey output.
func extractSSHFingerprint(s *nmap.Script) string {
	line := strings.SplitN(strings.TrimSpace(s.Output), "\n", 2)[0]
	return strings.TrimSpace(line)
}

// parseSSLCert extracts TLS certificate fields from an ssl-cert NSE script.
// nmap XML structure:
//
//	<script id="ssl-cert">
//	  <table key="subject"><elem key="commonName">example.com</elem></table>
//	  <table key="issuer"><elem key="commonName">Let's Encrypt</elem></table>
//	  <table key="validity">
//	    <elem key="notBefore">2024-01-01T00:00:00</elem>
//	    <elem key="notAfter">2024-04-01T00:00:00</elem>
//	  </table>
//	  <elem key="sig_algo">sha256WithRSAEncryption</elem>
//	</script>
func parseSSLCert(s *nmap.Script) *NSECertData {
	subjectTable := findTable(s.Tables, "subject")
	if subjectTable == nil {
		return nil
	}
	cert := &NSECertData{
		SubjectCN: findElem(subjectTable.Elements, "commonName"),
	}
	if issuerTable := findTable(s.Tables, "issuer"); issuerTable != nil {
		cert.Issuer = findElem(issuerTable.Elements, "commonName")
	}
	if validityTable := findTable(s.Tables, "validity"); validityTable != nil {
		if nb := findElem(validityTable.Elements, "notBefore"); nb != "" {
			cert.NotBefore, _ = parseNmapTime(nb)
		}
		if na := findElem(validityTable.Elements, "notAfter"); na != "" {
			cert.NotAfter, _ = parseNmapTime(na)
		}
	}
	// Subject Alternative Names appear nested under extensions.
	if extTable := findTable(s.Tables, "extensions"); extTable != nil {
		for i := range extTable.Tables {
			sub := &extTable.Tables[i]
			if sub.Key == "Subject Alternative Name" {
				for _, e := range sub.Elements {
					cert.SANs = append(cert.SANs, e.Value)
				}
			}
		}
	}
	cert.KeyType = findElem(s.Elements, "sig_algo")
	return cert
}

// applySSBOSDiscovery updates the host with OS information from smb-os-discovery.
// Only applied when nmap -O produced no result (OSAccuracy == 0), so the
// fingerprint-grade detection is never overridden by SMB.
func applySSBOSDiscovery(s *nmap.Script, host *Host) {
	for _, e := range s.Elements {
		switch e.Key {
		case "os":
			if host.OSAccuracy == 0 && host.OSName == "" {
				host.OSName = e.Value
				host.OSAccuracy = 60 // SMB OS is reliable but not fingerprint-grade
				host.OSFamily = "Windows"
			}
		case "domain":
			host.SMBDomain = e.Value
		case "FQDN", "fqdn":
			if host.SMBHostname == "" {
				host.SMBHostname = e.Value
			}
		}
	}
}

// findElem returns the value of the first Element with the given key.
func findElem(elems []nmap.Element, key string) string {
	for _, e := range elems {
		if e.Key == key {
			return strings.TrimSpace(e.Value)
		}
	}
	return ""
}

// findTable returns a pointer to the first Table with the given key.
func findTable(tables []nmap.Table, key string) *nmap.Table {
	for i := range tables {
		if tables[i].Key == key {
			return &tables[i]
		}
	}
	return nil
}

// nmapTimeFormats are the timestamp formats nmap uses in cert validity fields.
var nmapTimeFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// parseNmapTime parses a timestamp string from nmap XML (assumed UTC).
func parseNmapTime(s string) (time.Time, error) {
	for _, f := range nmapTimeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized nmap time format: %q", s)
}
