# Scanorama Product Roadmap

> Milestone-based roadmap focused on feature completeness.
> Last updated: 2026-04-19 · Current version: v0.27.0-dev

---

## Status Key

| Label | Meaning |
|-------|---------|
| **Done** | Shipped |
| **On Track** | In progress, no blockers |
| **Not Started** | Planned but not yet begun |

---

## v0.23 — Table Power-Ups & Discovery Changelog

**Theme:** Make every list view fast, sortable, and bulk-actionable — and make it obvious what changed since last time you looked.

The tables are the core interface. Right now only the hosts table has sorting, and nothing supports multi-select. This milestone brings all tables up to the same standard, adds the bulk operations that make managing hundreds of hosts practical, and introduces a discovery changelog so you can instantly see what's new, what changed, and what disappeared.

### Discovery Changelog

After every discovery run, users should be able to see a clear summary of what happened — not just "discovery completed" but *what actually changed on the network*. This is the view you check first thing in the morning.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Discovery diff view | After each discovery, show a changelog: new hosts (first seen), updated hosts (status changed, OS changed, new ports), and gone hosts (previously up, now unreachable). Accessible from the discovery detail page and as a dashboard widget. | M | Not Started |
| Host status transitions | Track and display state changes over time: up → down, down → up, new → up, up → gone. Show the transition timestamp and how long the host was in the previous state. | M | Not Started |
| "New hosts" badge | Visual indicator on the dashboard and hosts list showing how many hosts were first discovered since your last visit. Clicking it filters to just those hosts. | S | Not Started |
| "Gone hosts" retention | When a host stops responding, don't delete it — mark it as "gone" with a last-seen timestamp. Keep it visible in a separate tab or filter so users can see what disappeared and when. | S | Not Started |
| Discovery summary notification | After a scheduled discovery completes, push a WebSocket notification with a one-line summary: "Discovery on 10.0.1.0/24: 2 new hosts, 1 gone, 47 unchanged" | S | Not Started |
| Discovery history comparison | Compare any two discovery runs side by side: what hosts appeared between run A and run B, what disappeared, what changed status | M | Not Started |

### Tool Integration: MAC Vendor Lookup

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| OUI vendor identification | Integrate a Go OUI library (e.g., `klauspost/oui`) to resolve MAC addresses to hardware vendors during discovery. Display vendor name (Cisco, Raspberry Pi, Dell, etc.) on the host list and detail panel. | S | Not Started |
| Vendor column & filter | Add a "Vendor" column to the hosts table (sortable) and a vendor filter dropdown. Especially useful in the discovery changelog — "New host: 10.0.1.50 (Raspberry Pi Foundation)" is far more informative than just an IP. | S | Not Started |

### Response Timing

Network responsiveness is valuable signal — a host that takes 800ms to respond to a ping is telling you something different than one that responds in 2ms. Capture and surface this data.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Response time capture | Record RTT (round-trip time) during discovery pings and scan probes. Store per-host: min, max, average, and latest response time. | M | Not Started |
| Response time display | Show response time on the host list (sortable column) and host detail panel. Color-code: green (<50ms), yellow (50-200ms), red (>200ms), with configurable thresholds. | S | Not Started |
| Response time history | Track response time over time per host. Line chart on host detail showing latency trends — useful for spotting degradation before a host goes down entirely. | M | Not Started |
| Slow host detection | Flag hosts whose response time has increased significantly compared to their baseline. Surface on dashboard: "3 hosts responding slower than usual" | S | Not Started |
| Scan duration per host | Record and display how long each scan took per host (not just total scan duration). Helps identify hosts that are slow to scan vs. fast ones. | S | Not Started |
| Timeout tracking | When a host times out during scan or discovery, record it as a distinct event (not just "down"). Track timeout frequency — a host that times out intermittently is different from one that's cleanly offline. | S | Not Started |

### Table Improvements

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Universal sorting | Add server-side sorting to Scans, Networks, Schedules, Profiles, and Exclusions tables (hosts already done) | M | Not Started |
| Multi-select rows | Checkbox column on all list views with shift-click range selection and "select all on page" | M | Not Started |
| Bulk delete | Delete multiple scans, hosts, or networks in one action with confirmation dialog | S | Not Started |
| Bulk scan | Select multiple hosts → launch a scan against the selection | S | Not Started |
| Bulk tag | Apply or remove tags from multiple hosts at once (depends on v0.24 tag UI) | S | Not Started |
| Advanced filtering | Date range pickers, compound filters (e.g., "hosts that are up AND running Linux AND have port 22 open"), saved filter presets | L | Not Started |
| Column visibility | Let users show/hide table columns; persist choice in local state | S | Not Started |
| Keyboard table nav | Arrow keys to move between rows, Enter to open detail, Space to toggle select | S | Not Started |

### Dependencies

- Server-side sorting requires new `sort_by` / `sort_order` query params on Scans, Networks, Schedules, Profiles API endpoints (hosts already supports this).
- Bulk operations need new batch API endpoints (e.g., `DELETE /api/v1/scans/batch`, `POST /api/v1/scans/batch`).
- Discovery diff requires storing a snapshot of host state before each discovery run, or computing the diff from the hosts table using `first_seen` / `last_seen` timestamps.
- Response time capture requires changes to the discovery and scanning engine to record RTT data, and a new `host_metrics` table or additional columns on the hosts table.
- Timeout tracking needs a distinction between "host is down" and "host timed out" in the data model.

### Definition of Done

All six list views (Hosts, Scans, Networks, Schedules, Profiles, Exclusions) support sorting, multi-select, and at least one bulk action. After every discovery run, users can see a clear new/updated/gone changelog. Response times are captured, displayed, and sortable. Keyboard navigation works end to end.

---

## v0.24 — Tags, Groups, Dashboard & Admin

**Theme:** Give users the tools to organize their infrastructure the way they think about it — and make the dashboard and admin pages actually useful.

The backend already stores a `tags` field on hosts and scans, but it's invisible in the UI. This milestone surfaces tags, adds host grouping, and makes the profile clone endpoint usable. At the same time, the dashboard gets richer stats and a live activity feed, and the admin page gets a real settings UI and a proper system status view.

### Tags & Groups

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Tag management UI | Tag input component with autocomplete; add/remove tags on hosts and scans from detail panels and inline | M | Not Started |
| Tag-based filtering | Filter any list view by tag; combine with existing filters | S | Not Started |
| Host groups | Create named groups of hosts (e.g., "Production web servers", "DMZ") either manually or by filter rule | M | Not Started |
| Clone profile | Wire up existing `/profiles/{id}/clone` backend endpoint with a UI button and rename dialog | S | Not Started |
| Bulk tag from filter | "Tag all matching hosts" action when a filter is active | S | Not Started |

### Dashboard Improvements

The dashboard currently shows a version card, network stats (4 counters), a 7-day scan chart, recent scans, and discovery changes. This milestone makes it a proper operational overview.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Enhanced stats widgets | New dashboard cards: hosts by status (up/down/gone breakdown), hosts by OS family, top open ports, scan queue depth, average scan duration, hosts not scanned recently. Each card is a compact, glanceable summary. | M | Not Started |
| Live activity feed | Real-time event stream on the dashboard showing scans starting/completing, hosts discovered, hosts going up/down, and status changes. Uses existing WebSocket infrastructure (`/ws` endpoint). Displays as a scrolling timeline with event-type icons and relative timestamps. | M | Not Started |
| Quick actions | Action buttons/cards on the dashboard: "Run Discovery" (on a network), "Quick Scan" (launches a scan with defaults), "View New Hosts" (jumps to filtered host list). Reduces clicks for the most common daily tasks. | S | Not Started |
| Discovery changes widget | Improve the existing discovery changes display: show new/gone/changed counts as colored badges with click-through to filtered views. Show the last 3 discovery runs, not just the latest. | S | Not Started |

### Admin & Settings

The admin page has system status and workers but the config section is stubbed ("Coming soon"). This milestone replaces the stub with a real settings page and enriches the system status view.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Settings page | Editable settings UI replacing the "Coming soon" stub. Sections: scan defaults (default profile, timing, concurrency), discovery settings (ping timeout, methods), data retention (auto-purge old scans after N days), notification preferences (which events trigger WebSocket/future webhook alerts). Backed by a `PUT /api/v1/admin/config` endpoint. | L | Not Started |
| System status page | Enhance the existing admin status card into a dedicated system health view: DB connection pool stats (active/idle/max), scan worker utilization (busy/idle with current job details), memory and goroutine counts, disk usage for scan data, uptime, and Go runtime version. Auto-refreshes via existing `useStatus()` and `useWorkers()` hooks. | M | Not Started |
| Config persistence | Backend support for runtime-mutable settings. Store in a `settings` table (key/value with types) so config changes survive restarts without editing `config.yaml`. Read order: DB settings → config file → defaults. | M | Not Started |

### Dependencies

- Tag autocomplete requires a `GET /api/v1/tags` endpoint returning all known tags.
- Host groups may need a new `host_groups` database table and migration.
- Enhanced stats widgets need new aggregation endpoints or extension of existing `/networks/stats`.
- Live activity feed reuses the existing WebSocket broadcast channels — no new backend infrastructure needed, just a new frontend consumer.
- Settings persistence needs a `settings` table and a migration.
- Config persistence needs careful handling of which settings are runtime-mutable vs. require a restart.

### Definition of Done

Users can tag hosts, filter by tag, and create named host groups. Profile cloning works from the UI. The dashboard shows a rich operational overview with live events and one-click actions. The admin page has a working settings editor and detailed system health view.

---

## v0.25 — Smart Profiles & Smart Scan

**Theme:** Seed the system with a little info and let it progressively build deep knowledge about your network.

This is the flagship feature. The core philosophy: you shouldn't have to be an nmap expert to get thorough scan coverage. You point scanorama at a network, it runs a lightweight discovery, and then it *keeps going* — learning about each host in stages, choosing the right scan strategy based on what it already knows. Over time, your network inventory gets richer and deeper with no manual profile tuning.

### The Progressive Knowledge Loop

```
Discovery (nmap)        →  "These IPs are alive"        + MAC vendor (OUI, from v0.23)
     ↓
Reverse DNS (DNSX)      →  "10.0.1.5 is fileserver.local"
     ↓
OS Fingerprint (nmap)   →  "This one runs Linux, that one's Windows"
     ↓
SNMP Enrichment         →  "It's a Cisco 2960, IOS 15.2, uptime 142 days"
(GoSNMP)                   "3 interfaces, sysName=core-sw-01"
     ↓
OS-Aware Ports (nmap)   →  "Linux host → check 22, 80, 443, 111, 3306, 5432, 8080..."
                            "Windows host → check 135, 139, 445, 3389, 5985..."
     ↓
Banner Grab (ZGrab2)    →  "Port 443: TLS 1.3, cert CN=*.example.com, expires 2027-01"
                            "Port 22: OpenSSH 9.6, key type ed25519"
     ↓
Service Detection       →  "Port 443 is nginx 1.25, port 5432 is PostgreSQL 16.2"
(nmap -sV + ZGrab2)
     ↓
Deep Scan (nmap)        →  "Comprehensive scan on hosts with interesting findings"
     ↓
Ongoing Refresh         →  "Re-check hosts whose info is getting stale"
```

Each pass adds a layer of knowledge. The user seeds the system with a network range and scanorama fills in the rest. Nmap remains the core engine, but purpose-built tools handle specific enrichment steps faster and better.

### Smart Profiles

The existing profile editor (create/edit modal) works, but the built-in profiles are generic. This milestone adds **OS-aware profile templates** — curated scan configurations that know which ports and scan techniques matter for each platform.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Profile templates library | Ship a set of built-in, read-only "smart" profiles: Linux Standard, Windows Standard, Network Device, Web Server, Database Server, IoT/Embedded — each with the right ports and scan type for the job | M | Not Started |
| OS-aware port sets | Each template profile includes a curated port list for that OS/role (e.g., Windows: 135, 139, 445, 3389, 5985, 5986; Linux: 22, 80, 443, 111, 2049, 3306, 5432) | M | Not Started |
| Profile recommendations | After OS detection, suggest the best profile for each host: "3 Windows hosts detected — scan with Windows Standard?" | S | Not Started |
| Custom profile from template | "Start from Linux Standard, then customize" — fork a template into an editable user profile | S | Not Started |
| Profile effectiveness stats | Show per-profile stats: how many hosts scanned, average ports found, last used — so users know which profiles are actually useful | S | Not Started |

### Smart Scan Engine

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Smart Scan orchestrator | Backend engine that evaluates what's known about each host and selects the right scan stage (OS detect → port expand → service scan → deep scan) | L | Not Started |
| Knowledge score | Per-host "completeness" metric (0–100) based on: OS known, port coverage, service versions detected, scan freshness, and response timing baseline (from v0.23). Visible on host detail and list view. | M | Not Started |
| Smart Scan trigger | "Smart Scan" button on host detail, host group, or full network — only scans what needs scanning, skips hosts that are already well-known and recently scanned | S | Not Started |
| Smart Scan scheduling | Schedule recurring smart scans that only target hosts whose knowledge score has decayed (e.g., last scanned > 7 days ago, or score below threshold) | M | Not Started |
| Scan strategy preview | Before launching, show what Smart Scan plans to do: "12 hosts need OS detection, 8 need port expansion, 3 flagged for deep scan" — user can approve or adjust | S | Not Started |
| Suggestions engine | Dashboard suggestions that guide the user toward deeper knowledge: "23 hosts have no OS info — run OS detection?", "5 hosts haven't been scanned in 30 days", "New host found on 10.0.1.0/24" | M | Not Started |
| Scan result learning | After each scan completes, update the host's knowledge score and queue the next appropriate scan stage if the score is still below threshold | M | Not Started |
| Device identity merge | When a new MAC is seen but hostname and OS fingerprint match a known host, surface a "possible duplicate" suggestion in the discovery changelog. User can merge (preserves scan history, tags, notes) or dismiss. Handles MAC address randomization on iOS/Android. (ref: #694) | M | Not Started |

### Port & Service Intelligence

The standard `/etc/services` file is bare-minimum. Scanorama should ship with a richer, curated database of port-to-service mappings — and be able to learn from what it actually sees on the network.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Curated port database | Ship a rich port/service database that goes well beyond `/etc/services` — include common non-standard ports (8080, 8443, 9090, 3000, etc.), cloud/container ports, database ports, IoT protocols, and known malware ports. Source from nmap-services + community lists, tagged by category. | M | Not Started |
| Service fingerprint library | Map port + banner + protocol to a specific application and version. "Port 6379 with `REDIS` banner → Redis", "Port 27017 → MongoDB", "Port 9200 with JSON response → Elasticsearch" | M | Not Started |
| Banner grabbing | After port discovery, connect to open ports and capture service banners/headers. Extract version strings, TLS certificate info, HTTP server headers, and protocol handshake data. | M | Not Started |
| Banner display in UI | Show captured banners in host/scan detail views — raw banner text, parsed service name, version, and any TLS cert details | S | Not Started |
| Port database browser | Searchable reference page in the UI: look up any port number and see what services commonly run on it, which OS families use it, and whether it's in your network | S | Not Started |
| Learned port associations | Track what scanorama actually sees running on each port across your network. "In your environment, port 8080 is always Traefik" — override the generic database with local knowledge | M | Not Started |
| NSE script output parsing | Parse nmap NSE script output already present in `--oX` XML: SSL cert fields (CN, SANs, expiry, issuer), HTTP page titles, SMB OS detection, Netbios names, SSH key fingerprints, and raw banners. Zero new dependencies — this data is already captured. (ref: #693) | M | Not Started |

### Tool Integration: ZGrab2 (Banner Grabbing & TLS)

ZGrab2 is an application-layer scanner from the ZMap project (Apache 2.0, pure Go module). It's 10-100x faster than `nmap -sV` for banner grabbing and has modular protocol support.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| ZGrab2 integration | Integrate `zmap/zgrab2` as a Go dependency for application-layer scanning. Use as the primary engine for banner grabbing instead of relying solely on nmap's `-sV`. | M | Not Started |
| Protocol modules | Enable ZGrab2's built-in modules: HTTP (headers, redirects, tech fingerprint), TLS (cert chain, cipher suites, expiry), SSH (key type, version), FTP, SMTP, DNS, and more. | M | Not Started |
| TLS certificate inventory | For every TLS-enabled port, extract and store: subject CN, SANs, issuer, expiry date, key type/size, protocol version. Surface in host detail and as a network-wide certificate inventory. | M | Not Started |
| Certificate expiry alerts | Flag certificates expiring within 30/14/7 days. Dashboard widget: "5 certificates expiring this month." | S | Not Started |

### Tool Integration: GoSNMP (Network Device Enrichment)

GoSNMP is a pure Go SNMP library (BSD license). Network switches, routers, printers, and managed devices speak SNMP but often ignore port scans. This unlocks a whole class of device intelligence.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| SNMP discovery probe | During Smart Scan, attempt SNMPv2c/v3 queries against hosts that look like network devices (based on vendor, open ports, or OS). Configurable community strings and v3 credentials. | M | Not Started |
| Device metadata extraction | Pull sysName, sysDescr, sysLocation, sysContact, sysUpTime, interface count, and interface names via standard MIBs. Store as structured host metadata. | M | Not Started |
| Interface inventory | For SNMP-capable devices, enumerate network interfaces with: name, status (up/down), speed, MAC address, IP address, traffic counters. Display in a dedicated "Interfaces" tab on host detail. | L | Not Started |
| SNMP credentials management | Settings page to configure SNMP community strings and v3 credentials per network or host group. Credentials stored encrypted. | S | Not Started |
| LLDP/CDP neighbor discovery | Query LLDP-MIB and CDP MIB to discover switch-to-switch and switch-to-host connections. Reveals which switch port a device is connected to. Store in a `host_neighbors` table for future topology visualization. (ref: #696) | M | Not Started |
| ENTITY-MIB hardware inventory | Query ENTITY-MIB (RFC 4133) to extract chassis serial numbers, installed modules, hardware revisions, and firmware versions from SNMP-capable devices. Store as structured host metadata; include in CSV export. (ref: #697) | S | Not Started |

### Tool Integration: DNSX (DNS Enrichment)

DNSX is a fast multi-purpose DNS toolkit from ProjectDiscovery (MIT license, Go module). It handles reverse DNS, multi-record queries, and bulk resolution far faster than nmap's DNS scripts.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Reverse DNS enrichment | After discovery, run PTR lookups on all discovered IPs to auto-populate hostnames. Much faster than nmap's built-in reverse DNS, and supports custom resolver lists. | S | Not Started |
| DNS record collection | For hosts with known hostnames, collect A, AAAA, CNAME, MX, TXT, and SRV records. Store and display alongside host metadata. | M | Not Started |
| DNS-based discovery | Discover hosts by sweeping PTR records across a CIDR range — finds hosts that don't respond to ping but have DNS entries. Complements nmap's ping-based discovery. | M | Not Started |

### Dependencies

- Requires the tag/group infrastructure from v0.24 for targeting groups of hosts.
- Knowledge score calculation uses scan history and host metadata already in the DB.
- OS-aware port profiles ship as bundled YAML/JSON config files that can be extended by users.
- Profile templates are seeded via a DB migration or first-run setup.
- ZGrab2, GoSNMP, and DNSX are all pure Go modules — added as dependencies in `go.mod`, no external binaries needed.
- SNMP credentials management needs encrypted storage (vault or config encryption).
- TLS certificate inventory needs a `certificates` table linked to hosts and ports.
- Curated port database ships as a JSON/YAML seed file with a migration to populate the DB.

### Definition of Done

A user can click "Smart Scan" on a network or host group and watch it progressively discover and scan with zero manual profile configuration. Hosts are enriched with reverse DNS, SNMP metadata, TLS certificates, and service banners automatically. The dashboard surfaces suggestions for hosts that need attention.

---

## v0.26 — Analytics, Reporting & Queries

**Theme:** Turn raw scan data into understanding — and let users ask their own questions.

Scanorama collects a wealth of data but currently only shows a 7-day scan activity chart and some counters. This milestone adds the dashboards and visualizations that help users actually understand their network posture.

### Features

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Port distribution chart | Bar/treemap chart: most common open ports across all hosts, with service names | M | Not Started |
| OS family breakdown | Pie/donut chart showing host count by OS family, with drill-down to version | S | Not Started |
| Service inventory | Searchable table of all detected services across the network, grouped by service name and version | M | Not Started |
| Scan performance trends | Line chart: average scan duration over time, success/failure rates, scan volume trends | S | Not Started |
| Business metrics | Expose operational metrics beyond charts: scan throughput (scans/hour), discovery rates (hosts/run), duration breakdown by scan type, queue depth. Feeds into Prometheus for external dashboards. (ref: #222) | M | Not Started |
| System health metrics | Internal health telemetry: DB connection pool utilization, API latency percentiles (p50/p95/p99), scan worker queue depth, memory/goroutine counts. Exposed via existing `/metrics` endpoint. (ref: #222) | M | Not Started |
| Network health score | Per-network composite score based on: host responsiveness, scan freshness, known vs unknown services | M | Not Started |
| Host timeline | Per-host timeline view showing all scans, port changes, OS changes, and status transitions over time | M | Not Started |
| Custom date ranges | All charts support selectable time windows: 7d, 30d, 90d, 1y, custom range | S | Not Started |
| Dashboard widgets | Configurable dashboard where users can add/remove/reorder widget cards | L | Not Started |

### Reporting

Scan data is only useful if you can get it out of the tool and in front of the right people. This section adds structured reports and on-demand querying so users can answer their own questions without eyeballing tables.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Saved queries | Let users define and save reusable queries against the host/scan data: "All Linux hosts with port 22 open", "Hosts not scanned in 30+ days", "Networks with >10% unreachable hosts". Query builder UI with AND/OR logic. | L | Not Started |
| Query results view | Dedicated results page for saved queries: sortable table, export to CSV/JSON, shareable permalink | M | Not Started |
| Scheduled reports | Run a saved query on a schedule (daily, weekly, monthly) and deliver the results via email or webhook. "Every Monday, email me a list of hosts that went down last week." | M | Not Started |
| Report templates | Pre-built report types: Network Inventory (all hosts, OS, open ports), Change Report (what's new/changed/gone since last report), Security Posture (unusual ports, outdated services, unscanned hosts) | M | Not Started |
| PDF/HTML report generation | Render any report as a formatted PDF or HTML document with charts, tables, and summary stats — suitable for stakeholders or compliance | L | Not Started |
| Compliance snapshots | Point-in-time snapshots of network state that can be compared over time. "Show me the diff between this month's inventory and last month's." | M | Not Started |
| API query endpoint | `POST /api/v1/query` — programmatic access to the same query engine so users can integrate with external tools, scripts, or dashboards | M | Not Started |

### Tool Integration: Httpx (Web Technology Fingerprinting)

Httpx is a fast multi-purpose HTTP toolkit from ProjectDiscovery (MIT license, Go module). For any host with an open web port, it tells you exactly what's running — far richer than nmap's HTTP service detection.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Httpx integration | Integrate `projectdiscovery/httpx` to probe all HTTP/HTTPS ports discovered by nmap. Capture: status code, page title, server header, content type, redirect chain, response size. | M | Not Started |
| Web technology detection | Use httpx's built-in tech detection (Wappalyzer-based) to identify CMS, frameworks, languages, CDN, analytics, and hosting platforms. Store as structured tags per host. | M | Not Started |
| Web service inventory | Network-wide view of all detected web technologies: "12 hosts running nginx, 4 running Apache, 2 running IIS, 1 running Caddy". Filterable and drillable. | S | Not Started |
| Screenshot capture | Optional: capture screenshots of web interfaces via httpx for visual inventory. Display thumbnails in host detail. | M | Not Started |

### Tool Integration: Nuclei (Lightweight Vulnerability Scanning)

Nuclei is a template-driven vulnerability scanner from ProjectDiscovery (MIT license, Go module). Thousands of community templates detect misconfigurations, exposed panels, default credentials, and known CVEs — without the overhead of a full vulnerability scanner.

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Nuclei integration | Integrate `projectdiscovery/nuclei` as an optional scan stage. Run targeted templates against discovered services — not a full internet vuln scan, but focused checks based on what's actually running. | L | Not Started |
| Template management | Ship a curated subset of nuclei templates (misconfiguration, exposure, default-login, CVE) and let users enable/disable template categories. Auto-update templates from the community feed. | M | Not Started |
| Finding model | Store nuclei findings as structured data: severity (info/low/medium/high/critical), template ID, matched URL, description, remediation. Link findings to hosts and ports. | M | Not Started |
| Security posture dashboard | Dashboard panel showing findings by severity, most common issues, hosts with critical findings, and trend over time. Feeds into the Security Posture report template. | M | Not Started |
| Finding lifecycle | Track finding status: open, acknowledged, resolved, false-positive. Re-check on next scan to auto-close resolved findings. | M | Not Started |

### Dependencies

- Several charts require new aggregation API endpoints (e.g., `GET /api/v1/stats/ports`, `GET /api/v1/stats/os`, `GET /api/v1/stats/services`).
- Host timeline requires scan results to be queryable by host across time.
- Network health score builds on the knowledge score concept from v0.25.
- Saved queries need a `saved_queries` table and a query execution engine that translates the builder UI into SQL.
- Scheduled reports depend on the existing scheduler infrastructure and a notification delivery mechanism (email/webhook from v0.27, or a simpler approach shipped here).
- PDF generation is built in this milestone and reused by later features.
- Httpx and Nuclei are both Go modules — added as dependencies, no external binaries.
- Nuclei templates are downloaded/updated at runtime; needs a template cache directory and update mechanism.
- Findings model needs a `findings` table with severity indexing for dashboard queries.

### Definition of Done

The dashboard tells a story. A user can glance at it and understand: how many hosts are on the network, what they're running, what's changed recently, and what needs attention. Users can build, save, and schedule custom queries. Reports can be generated on demand or automatically and exported as PDF.

---

## v0.27 — UX Polish & Quality of Life

**Theme:** The details that make the difference between a tool and a product.

### Features

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Scan diff/comparison | Select two scans of the same host and see what changed: new ports, closed ports, service version changes, OS changes | M | Not Started |
| CSV/JSON export | Export any table view to CSV or JSON, respecting current filters and sort order | M | Not Started |
| Global search | Cmd/Ctrl+K command palette: search across hosts, scans, networks, profiles by name, IP, tag, or status | M | Not Started |
| Keyboard shortcuts | Full shortcut system: `n` for new scan, `d` for dashboard, `?` for shortcut help overlay | S | Not Started |
| Dark mode | Theme toggle with system preference detection; persist choice | M | Not Started |
| Notification preferences | Per-host and global alert rules: choose which hosts trigger notifications (online, offline, or both), and which delivery channel (webhook). Granularity confirmed by community demand — users want "alert me when this NAS goes offline", not just global toggles. (ref: #695) | M | Not Started |
| Webhook management UI | Configure outgoing webhooks for scan events (backend type already defined, needs UI + delivery engine) | M | Not Started |
| Inline editing | Edit hostname, tags, and notes directly in table rows without opening a panel | S | Not Started |
| Onboarding flow | First-run wizard: add your first network, run a discovery, review results | M | Not Started |

### Dependencies

- PDF reports require a server-side PDF generation library or a frontend-rendered approach.
- Webhook delivery needs a new backend component (queue + retry logic).
- Global search needs a lightweight search index or a unified search API endpoint.

### Definition of Done

The app feels polished. Common tasks are fast, discoverable, and keyboard-accessible. Users can export and share their findings. New users can get started without reading docs.

---

## v1.0 — Production Release

**Theme:** Confidence. Everything works, nothing is missing, the docs are solid.

This milestone is about closing the remaining gaps before calling scanorama production-ready for a wider audience. It doesn't introduce major new features — it hardens what's already there.

### Features

| Feature | Description | Effort | Status |
|---------|-------------|--------|--------|
| Multi-user auth | User accounts with login, password reset, and session management (currently API-key only) | L | Not Started |
| Role-based access | Admin / operator / viewer roles controlling who can modify networks, run scans, change settings | L | Not Started |
| Audit log | Record who did what and when — scan launches, config changes, host edits. Include compliance trail with retention policy. (ref: #222) | M | Not Started |
| Linux privilege management | Fix broken `dropPrivileges()` — extract into `internal/privdrop` using `Setresuid`/`Setresgid`/`Setgroups` + `runtime.LockOSThread()`. Wire into both `server` and `daemon` startup paths. (ref: #554) | M | Not Started |
| `scanorama doctor` command | Pre-flight check CLI command: verify nmap binary, version, `cap_net_raw`/`cap_net_admin` capabilities, and report exactly what to fix. (ref: #554) | S | Not Started |
| Raw socket capability detection | `CheckRawSocketAccess()` function that probes for raw socket permission at startup. Log warnings when missing, return 400 from API when SYN/ACK/UDP scans are submitted without capabilities. (ref: #554) | M | Not Started |
| Bundled nmap decision | Evaluate shipping a pinned nmap binary under `/opt/scanorama/bin/nmap` with `setcap` — pins version, owns the capability step, removes ambiguity. Document decision as ADR. (ref: #554) | M | Not Started |
| Alerting infrastructure | Alerting rules engine with configurable triggers (host down, scan failed, cert expiring). Integration points for Alertmanager, PagerDuty, and webhook delivery. (ref: #222) | L | Not Started |
| Distributed tracing | OpenTelemetry integration: trace scan lifecycle from API request through worker to nmap execution. Span context propagation for cross-component visibility. (ref: #222) | L | Not Started |
| Operational dashboards | Grafana dashboard templates for scan throughput, system health, and network inventory. Pre-built JSON dashboard definitions shipped with the project. (ref: #222) | M | Not Started |
| Rate limiting | Protect the API against abuse; configurable per-endpoint limits | S | Not Started |
| Scan concurrency queue | Prevent resource exhaustion under load — API returns 429/503 when full (already planned in Phase 4) | M | Not Started |
| Test coverage push | Bring service-layer coverage from 29% to 60%+; CLI coverage from 14% to 40%+ | L | Not Started |
| End-to-end tests | Playwright or Cypress test suite covering critical user flows | L | Not Started |
| Performance benchmarks | Establish baseline performance for scans, API response times, and DB queries at scale | M | Not Started |
| Security audit | Review authentication, input validation, SQL injection vectors, dependency vulnerabilities | M | Not Started |
| Documentation refresh | Update all docs to reflect features shipped in v0.23–v0.27; add user guide with screenshots | M | Not Started |

### Definition of Done

Scanorama is ready for other people to depend on. Auth works, the test suite is solid, the docs are current, and the app handles load gracefully. Linux privilege management is correct and verified via `scanorama doctor`. Operators have alerting, tracing, and Grafana dashboards for production visibility.

---

## Milestone Summary

| Milestone | Theme | Key Deliverables | Status |
|-----------|-------|------------------|--------|
| **v0.23** | Tables & Discovery Changelog | Sorting, bulk actions, discovery diff (new/updated/gone), MAC vendor lookup (OUI), response timing, slow host detection | Done ✓ |
| **v0.24** | Tags, Groups, Dashboard & Admin | Tag UI, host groups, profile cloning, enhanced dashboard stats, live activity feed, quick actions, settings page, system status | Done ✓ |
| **v0.25** | Smart Profiles & Smart Scan | Smart profiles, progressive scanning, ZGrab2 banners, GoSNMP enrichment, DNSX, curated port DB, knowledge scores | Done ✓ |
| **v0.26** | Analytics, Reporting & Queries | Port/OS/service charts, web tech fingerprinting (httpx), vuln scanning (nuclei), saved queries, scheduled reports, PDF export, business & system health metrics | Done ✓ |
| **v0.27** | UX Polish | Scan diff, export, global search, dark mode, webhooks, per-host alert rules (#695), device identity (#713), onboarding | Current |
| **v0.28** | Network Topology | Visual graph of network topology using LLDP/CDP neighbor data from v0.25; nodes = hosts/switches, edges = confirmed connections, filterable by network/group/tag (#698) | Later |
| **v1.0** | Production Release | Multi-user auth, RBAC, audit log, Linux privilege management (#554), `scanorama doctor`, capability detection, alerting infrastructure, test coverage, security audit | Later |

---

## Quick Wins (can be shipped independently)

These are small items that can land in any release without waiting for their parent milestone:

1. **Clone profile button** — backend endpoint exists, just needs a UI button (v0.24 scope but trivial)
2. **Expose host tags** — backend field exists, add a tag display component to host detail panel
3. **Sort on Scans table** — add `sort_by` support to the scans API and wire up column headers
4. **Network stats on dashboard** — `/networks/stats` endpoint exists but isn't fully used
5. **Schedule next-run display** — `/schedules/{id}/next-run` endpoint exists, show it in the schedule list

---

## Dependency Graph

```
v0.23 (Tables & Discovery Changelog)
  └─► v0.24 (Tags, Groups, Dashboard & Admin)
        └─► v0.25 (Smart Scan) ◄── builds on tags, groups, and host metadata
              │     └── LLDP/CDP topology data (#696) ─────────────────────┐
              └─► v0.26 (Analytics & Reporting) ◄── uses knowledge scores  │
                    └─► v0.27 (UX Polish) ◄── benefits from all prior data  │
                          └─► v0.28 (Topology) ◄── requires LLDP data ─────┘
                                └─► v1.0 (Production)
```

Each milestone builds on the one before it, but individual features within a milestone can often be developed in parallel. The Quick Wins listed above can be shipped at any time.

---

## Tool Integrations Reference

Open source tools integrated across the roadmap. All are actively maintained, have permissive licenses, and (except OUI) are available as Go modules for native integration — no shelling out to external binaries.

| Tool | What it does | License | Language | Milestone | GitHub |
|------|-------------|---------|----------|-----------|--------|
| **OUI lookup** (`klauspost/oui`) | MAC address → hardware vendor identification | MIT | Go | v0.23 | [klauspost/oui](https://github.com/klauspost/oui) |
| **ZGrab2** (`zmap/zgrab2`) | Fast application-layer banner grabbing, TLS cert extraction, multi-protocol scanning | Apache 2.0 | Go | v0.25 | [zmap/zgrab2](https://github.com/zmap/zgrab2) |
| **GoSNMP** (`gosnmp/gosnmp`) | SNMP v1/v2c/v3 client — device metadata, interface inventory, uptime | BSD | Go | v0.25 | [gosnmp/gosnmp](https://github.com/gosnmp/gosnmp) |
| **DNSX** (`projectdiscovery/dnsx`) | Fast reverse DNS, multi-record queries, bulk resolution | MIT | Go | v0.25 | [projectdiscovery/dnsx](https://github.com/projectdiscovery/dnsx) |
| **Httpx** (`projectdiscovery/httpx`) | HTTP fingerprinting, web technology detection (Wappalyzer), response metadata | MIT | Go | v0.26 | [projectdiscovery/httpx](https://github.com/projectdiscovery/httpx) |
| **Nuclei** (`projectdiscovery/nuclei`) | Template-driven vulnerability scanning — misconfigs, exposed panels, CVEs | MIT | Go | v0.26 | [projectdiscovery/nuclei](https://github.com/projectdiscovery/nuclei) |

### Tools Considered but Not Included

| Tool | Why not (for now) |
|------|-------------------|
| **Masscan** | Extremely fast port scanning, but adds a C binary dependency. Nmap + naabu cover the same ground. Revisit if scan speed on very large networks becomes a bottleneck. |
| **Testssl.sh** | Deep TLS/SSL vulnerability analysis, but CLI-only (Bash). ZGrab2 covers certificate extraction; testssl.sh could be added later as an optional deep-dive tool. |
| **Gowitness** | Web screenshot capture, but GPLv3 license and Chrome dependency. Httpx covers most web fingerprinting needs; screenshots are optional in v0.26. |
| **Amass** | Comprehensive subdomain enumeration, but heavy and overlaps with DNSX + subfinder. Better suited for external/internet-facing recon than internal network monitoring. |
| **OpenVAS** | Full vulnerability scanner, but very heavy (requires its own infrastructure). Nuclei covers the 80% case with 1% of the overhead. |

---

## GitHub Issue References

Items from older GitHub issues incorporated into this roadmap:

| Issue | Title | Mapped To | Notes |
|-------|-------|-----------|-------|
| #222 | Observability: monitoring & alerting | v0.26 (metrics), v1.0 (alerting, tracing, dashboards, audit log) | Business metrics and system health metrics fit v0.26 analytics; infrastructure-level observability (alerting, OpenTelemetry, Grafana) is v1.0 |
| #554 | Linux nmap privilege management | v1.0 | Fix `dropPrivileges()`, `scanorama doctor`, raw socket detection, bundled nmap ADR |
| #541–#546 | Frontend features (toast, scan lifecycle, schedule/network editing, host management, dashboard chart) | — | All closed/completed as of 2026-04-06 |
