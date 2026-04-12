// Package enrichment — service fingerprint library.
// Maps (port, banner) → (application, version) using ordered regex rules.
package enrichment

import (
	"encoding/json"
	"os"
	"regexp"
)

// Fingerprint describes a single service identification rule.
// Rules are matched in order; the first match wins.
type Fingerprint struct {
	// Port restricts this rule to a specific port (0 = any port).
	Port int `json:"port,omitempty"`
	// Protocol restricts this rule to "tcp" or "udp" (empty = any).
	Protocol string `json:"protocol,omitempty"`
	// Pattern is a regular expression matched against the raw banner text.
	// An empty pattern matches any banner (port-only rule).
	Pattern string `json:"pattern"`
	// Service is the application name returned on a match (e.g. "Redis").
	Service string `json:"service"`
	// VersionPattern is an optional regex whose first capture group extracts
	// the version string from the banner. Ignored when empty.
	VersionPattern string `json:"version_pattern,omitempty"`
}

// compiledFingerprint is a Fingerprint with its regexes pre-compiled.
type compiledFingerprint struct {
	Fingerprint
	re    *regexp.Regexp // may be nil when Pattern is empty
	verRe *regexp.Regexp // may be nil when VersionPattern is empty
}

// Fingerprinter matches banners against a loaded set of fingerprint rules.
type Fingerprinter struct {
	rules []compiledFingerprint
}

// builtinFingerprints is the bundled fingerprint database.
// Rules are matched in declaration order; put more-specific rules first.
var builtinFingerprints = []Fingerprint{
	// ── SSH / OpenSSH ─────────────────────────────────────────────────────────
	{Port: 22, Pattern: `(?i)^SSH-`, Service: "OpenSSH",
		VersionPattern: `OpenSSH_([0-9][0-9a-z._-]*)`},

	// ── Redis ─────────────────────────────────────────────────────────────────
	{Port: 6379, Pattern: `(?i)redis|PONG|\-NOAUTH|\-ERR.*redis`, Service: "Redis"},
	// Redis on non-standard ports (version in INFO response)
	{Pattern: `(?i)redis_version:([0-9.]+)`, Service: "Redis",
		VersionPattern: `redis_version:([0-9.]+)`},

	// ── PostgreSQL ────────────────────────────────────────────────────────────
	{Port: 5432, Pattern: `(?i)postgresql|FATAL.*database|FATAL.*role`, Service: "PostgreSQL"},

	// ── MySQL / MariaDB ───────────────────────────────────────────────────────
	// MySQL sends a binary handshake; 0x0a = protocol version, followed by version string.
	{Port: 3306, Pattern: `(?i)mysql|mariadb|\n[\d]+\.[\d]+\.[\d]+`, Service: "MySQL",
		VersionPattern: `([\d]+\.[\d]+\.[\d]+[-\w]*)`},
	// Port-only fallback for binary MySQL handshakes without text markers.
	{Port: 3306, Pattern: ``, Service: "MySQL"},

	// ── MongoDB ───────────────────────────────────────────────────────────────
	{Port: 27017, Pattern: `(?i)mongodb|You are trying to access MongoDB`, Service: "MongoDB"},

	// ── Elasticsearch ─────────────────────────────────────────────────────────
	{Port: 9200, Pattern: `(?i)"cluster_name"|"tagline".*elasticsearch|You Know, for Search`,
		Service: "Elasticsearch", VersionPattern: `"number"\s*:\s*"([0-9.]+)"`},

	// ── Memcached ─────────────────────────────────────────────────────────────
	{Port: 11211, Pattern: `(?i)memcached|VERSION\s+[\d.]+`, Service: "Memcached",
		VersionPattern: `VERSION\s+([\d.]+)`},

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	{Port: 5672, Pattern: `(?i)AMQP|RabbitMQ`, Service: "RabbitMQ"},
	{Port: 15672, Pattern: `(?i)RabbitMQ|Management`, Service: "RabbitMQ Management"},

	// ── Kafka ─────────────────────────────────────────────────────────────────
	{Port: 9092, Pattern: `(?i)kafka|Not enough data to read RequestHeader`, Service: "Kafka"},

	// ── SMB ───────────────────────────────────────────────────────────────────
	{Port: 445, Pattern: ``, Service: "SMB"}, // binary protocol; port match only
	{Port: 139, Pattern: ``, Service: "SMB"},

	// ── RDP ───────────────────────────────────────────────────────────────────
	{Port: 3389, Pattern: ``, Service: "RDP"}, // binary protocol; port match only

	// ── HTTP: specific servers first, generic fallback last ──────────────────
	// All Server:-header rules must appear before the generic ^HTTP/ rule or
	// they will never match (first-wins ordering).
	{Pattern: `(?i)Server:\s*nginx`, Service: "nginx",
		VersionPattern: `Server:\s*nginx/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Apache`, Service: "Apache",
		VersionPattern: `Server:\s*Apache/([\d.]+)`},
	{Pattern: `(?i)Server:\s*lighttpd`, Service: "lighttpd",
		VersionPattern: `Server:\s*lighttpd/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Caddy`, Service: "Caddy",
		VersionPattern: `Server:\s*Caddy/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Microsoft-IIS`, Service: "IIS",
		VersionPattern: `Microsoft-IIS/([\d.]+)`},
	{Pattern: `(?i)Server:\s*openresty`, Service: "OpenResty",
		VersionPattern: `Server:\s*openresty/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Tengine`, Service: "Tengine"},
	{Pattern: `(?i)Server:\s*Cherokee`, Service: "Cherokee"},
	{Pattern: `(?i)Server:\s*Hiawatha`, Service: "Hiawatha"},
	{Pattern: `(?i)Server:\s*gunicorn`, Service: "Gunicorn",
		VersionPattern: `gunicorn/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Jetty`, Service: "Jetty",
		VersionPattern: `Jetty\(([\d.]+)\)`},
	{Pattern: `(?i)Server:\s*Tomcat|Apache-Coyote`, Service: "Tomcat",
		VersionPattern: `Apache Tomcat/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Node\.js|powered by Express`, Service: "Node.js/Express"},
	{Pattern: `(?i)Server:\s*Werkzeug`, Service: "Werkzeug",
		VersionPattern: `Werkzeug/([\d.]+)`},
	{Pattern: `(?i)Server:\s*waitress`, Service: "Waitress"},
	{Pattern: `(?i)Server:\s*uvicorn`, Service: "Uvicorn"},
	{Pattern: `(?i)X-Powered-By:\s*PHP`, Service: "PHP",
		VersionPattern: `X-Powered-By:\s*PHP/([\d.]+)`},
	{Pattern: `(?i)Server:\s*HAProxy|via:.*haproxy`, Service: "HAProxy"},
	{Pattern: `(?i)Server:\s*Squid`, Service: "Squid",
		VersionPattern: `Squid/([\d.]+)`},
	{Pattern: `(?i)Server:\s*Varnish|X-Varnish:`, Service: "Varnish"},
	{Pattern: `(?i)Server:\s*traefik`, Service: "Traefik"},
	{Pattern: `(?i)Server:\s*envoy`, Service: "Envoy"},
	// Generic HTTP fallback — must be last in this group so all Server: rules above win.
	{Pattern: `(?i)^HTTP/[0-9.]+ `, Service: "HTTP"},

	// ── FTP ───────────────────────────────────────────────────────────────────
	{Port: 21, Pattern: `(?i)^220.*ftp|^220.*FileZilla|^220.*ProFTPD|^220.*vsftpd`,
		Service: "FTP", VersionPattern: `(?i)(?:ProFTPD|vsftpd|FileZilla)[/ ]([\d.]+)`},
	{Port: 21, Pattern: `^220 `, Service: "FTP"},

	// ── SMTP ─────────────────────────────────────────────────────────────────
	{Pattern: `(?i)^220.*smtp|^220.*mail|^220.*postfix|^220.*exim|^220.*sendmail`,
		Service: "SMTP"},

	// ── POP3 ─────────────────────────────────────────────────────────────────
	{Pattern: `(?i)^\+OK.*pop`, Service: "POP3"},

	// ── IMAP ─────────────────────────────────────────────────────────────────
	{Pattern: `(?i)^\* OK.*IMAP|^\* OK.*Dovecot|^\* OK.*Courier`, Service: "IMAP"},

	// ── DNS ──────────────────────────────────────────────────────────────────
	// BIND sends a version string via TXT query; banner grabbers may see the
	// BIND version response or a refused/NXDOMAIN on TCP port 53.
	{Port: 53, Pattern: `(?i)bind|named|version\.bind|REFUSED|NXDOMAIN`, Service: "DNS"},
	{Port: 53, Pattern: ``, Service: "DNS"}, // port-only fallback for binary DNS frames

	// ── DNS-over-TLS (DoT) ───────────────────────────────────────────────────
	{Port: 853, Pattern: ``, Service: "DNS-over-TLS"},

	// ── DNS-over-HTTPS (DoH) ─────────────────────────────────────────────────
	{Port: 8053, Pattern: `(?i)dns`, Service: "DNS-over-HTTPS"},

	// ── mDNS / DNS-SD ────────────────────────────────────────────────────────
	{Port: 5353, Pattern: ``, Service: "mDNS"},

	// ── Database management UIs ──────────────────────────────────────────────
	{Port: 8080, Pattern: `(?i)phpMyAdmin|phpmyadmin`, Service: "phpMyAdmin"},
	{Port: 8080, Pattern: `(?i)pgAdmin`, Service: "pgAdmin"},

	// ── Telnet / banner-based ────────────────────────────────────────────────
	{Port: 23, Pattern: ``, Service: "Telnet"},

	// ── VNC ───────────────────────────────────────────────────────────────────
	{Port: 5900, Pattern: `(?i)^RFB [0-9]`, Service: "VNC",
		VersionPattern: `RFB ([\d.]+)`},
	{Port: 5901, Pattern: `(?i)^RFB [0-9]`, Service: "VNC"},

	// ── X11 ───────────────────────────────────────────────────────────────────
	{Port: 6000, Pattern: ``, Service: "X11"},

	// ── LDAP ──────────────────────────────────────────────────────────────────
	{Port: 389, Pattern: ``, Service: "LDAP"},
	{Port: 636, Pattern: ``, Service: "LDAPS"},

	// ── NTP ───────────────────────────────────────────────────────────────────
	{Port: 123, Pattern: ``, Service: "NTP"},

	// ── Syslog ───────────────────────────────────────────────────────────────
	{Port: 514, Pattern: ``, Service: "Syslog"},

	// ── Docker / container runtimes ──────────────────────────────────────────
	{Port: 2375, Pattern: `(?i)"ApiVersion"|docker`, Service: "Docker API"},
	{Port: 2376, Pattern: `(?i)"ApiVersion"|docker`, Service: "Docker API (TLS)"},

	// ── Kubernetes ───────────────────────────────────────────────────────────
	{Port: 6443, Pattern: `(?i)kubernetes|k8s|apiserver`, Service: "Kubernetes API"},
	{Port: 10250, Pattern: `(?i)kubelet`, Service: "Kubelet"},

	// ── Prometheus / metrics ─────────────────────────────────────────────────
	{Port: 9090, Pattern: `(?i)prometheus|HELP process_`, Service: "Prometheus"},
	{Port: 9100, Pattern: `(?i)HELP node_|HELP go_gc`, Service: "Node Exporter"},

	// ── Grafana ───────────────────────────────────────────────────────────────
	{Port: 3000, Pattern: `(?i)Grafana|grafana`, Service: "Grafana"},

	// ── Consul / service discovery ───────────────────────────────────────────
	{Port: 8500, Pattern: `(?i)consul`, Service: "Consul"},

	// ── etcd ─────────────────────────────────────────────────────────────────
	{Port: 2379, Pattern: `(?i)etcd`, Service: "etcd"},

	// ── HashiCorp Vault ───────────────────────────────────────────────────────
	{Port: 8200, Pattern: `(?i)vault|Vault`, Service: "Vault"},

	// ── Hadoop / HDFS ────────────────────────────────────────────────────────
	{Port: 50070, Pattern: `(?i)NameNode|HDFS`, Service: "HDFS NameNode"},
	{Port: 50075, Pattern: `(?i)DataNode|HDFS`, Service: "HDFS DataNode"},

	// ── Zookeeper ────────────────────────────────────────────────────────────
	{Port: 2181, Pattern: `(?i)zxid|Zookeeper|imok`, Service: "ZooKeeper"},

	// ── MQTT ─────────────────────────────────────────────────────────────────
	{Port: 1883, Pattern: ``, Service: "MQTT"},

	// ── CouchDB ──────────────────────────────────────────────────────────────
	{Port: 5984, Pattern: `(?i)"couchdb"|"Welcome".*"couchdb"`, Service: "CouchDB",
		VersionPattern: `"version"\s*:\s*"([^"]+)"`},

	// ── InfluxDB ─────────────────────────────────────────────────────────────
	{Port: 8086, Pattern: `(?i)influxdb|InfluxDB`, Service: "InfluxDB"},

	// ── Cassandra ────────────────────────────────────────────────────────────
	{Port: 9042, Pattern: ``, Service: "Cassandra"},

	// ── Neo4j ────────────────────────────────────────────────────────────────
	{Port: 7474, Pattern: `(?i)neo4j|Neo4j`, Service: "Neo4j"},

	// ── MSSQL ────────────────────────────────────────────────────────────────
	{Port: 1433, Pattern: ``, Service: "MSSQL"},

	// ── Oracle DB ────────────────────────────────────────────────────────────
	{Port: 1521, Pattern: ``, Service: "Oracle DB"},

	// ── Citrix / ICA ─────────────────────────────────────────────────────────
	{Port: 1494, Pattern: ``, Service: "Citrix ICA"},
	{Port: 2598, Pattern: ``, Service: "Citrix CGP"},

	// ── SIP (VoIP) ───────────────────────────────────────────────────────────
	{Port: 5060, Pattern: `(?i)^SIP/|^REGISTER SIP|^OPTIONS SIP`, Service: "SIP"},

	// ── IRC ───────────────────────────────────────────────────────────────────
	{Port: 6667, Pattern: `(?i):.*NOTICE.*\*.*:`, Service: "IRC"},
	{Port: 6697, Pattern: `(?i):.*NOTICE.*\*.*:`, Service: "IRC (TLS)"},

	// ── Git smart HTTP ───────────────────────────────────────────────────────
	{Port: 9418, Pattern: ``, Service: "Git"},
}

// NewFingerprinter builds a Fingerprinter from the bundled rules plus an optional
// user-supplied JSON file (extraPath). If extraPath is empty or does not exist,
// only the built-in rules are loaded. User rules are prepended before built-ins so
// they take priority and can override any built-in match.
func NewFingerprinter(extraPath string) *Fingerprinter {
	rules := make([]Fingerprint, len(builtinFingerprints))
	copy(rules, builtinFingerprints)

	if extraPath != "" {
		if extra, err := loadFingerprintFile(extraPath); err == nil {
			rules = append(extra, rules...)
		}
	}

	compiled := make([]compiledFingerprint, 0, len(rules))
	for _, r := range rules {
		cf := compiledFingerprint{Fingerprint: r}
		if r.Pattern != "" {
			if re, err := regexp.Compile(r.Pattern); err == nil {
				cf.re = re
			}
		}
		if r.VersionPattern != "" {
			if re, err := regexp.Compile(r.VersionPattern); err == nil {
				cf.verRe = re
			}
		}
		compiled = append(compiled, cf)
	}

	return &Fingerprinter{rules: compiled}
}

// Match returns the service name and version string for the given port and banner.
// Returns empty strings when no rule matches.
func (f *Fingerprinter) Match(port int, banner string) (service, version string) {
	for _, r := range f.rules {
		if r.Port != 0 && r.Port != port {
			continue
		}
		// Empty pattern = port-only match (always matches when port matches above).
		if r.re != nil && !r.re.MatchString(banner) {
			continue
		}
		// Pattern matched (or port-only rule reached).
		service = r.Service
		if r.verRe != nil {
			if m := r.verRe.FindStringSubmatch(banner); len(m) > 1 {
				version = m[1]
			}
		}
		return service, version
	}
	return "", ""
}

// loadFingerprintFile reads a JSON array of Fingerprint rules from disk.
func loadFingerprintFile(path string) ([]Fingerprint, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-configured path
	if err != nil {
		return nil, err
	}
	var rules []Fingerprint
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}
