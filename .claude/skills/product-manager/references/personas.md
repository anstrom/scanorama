# Scanorama User Personas

> Last updated: 2026-04-13
> Edit this file to add, remove, or refine personas. The product-manager skill reads it when evaluating features and planning milestones.

---

## How to use these personas

When evaluating a feature, ask:
- Which persona(s) does this primarily serve?
- Does it help the core persona (Alex) or a more advanced one?
- Does it create friction for any persona who doesn't need it?
- Features that serve 2+ personas with no friction tradeoffs are the highest priority.

---

## Persona 1 — Alex, the Homelab Enthusiast

**"I just want to know what's on my network without becoming an nmap expert."**

**Background:** Alex runs a home network with 30–60 devices — a NAS, a handful of Raspberry Pis, old laptops repurposed as servers, smart home hubs, a managed switch, printers, and whatever else has accumulated over the years. Alex is technically comfortable but not a network specialist. Has used nmap a few times but never remembers the flags, loses the results, and can't easily see what changed between runs.

**Goals:**
- Know what's on the network at any point in time
- See when something new appears or something disappears
- Get useful info (OS, open ports, hostnames) without manual configuration
- Self-hosted, runs on a Raspberry Pi or similar modest hardware, no cloud dependency

**Frustrations:**
- nmap output is a wall of text with no memory
- Has to re-run scans from scratch every time
- Doesn't know which ports are "normal" for each device
- No easy way to see "what changed since last week?"

**What Alex values most:** Zero-config smart defaults, persistent inventory, clear change detection, works on modest hardware.

**What Alex doesn't need:** Compliance reports, API integrations, multi-tenant support, fine-grained RBAC.

**Representative asks:**
- "Can it just scan automatically on a schedule?"
- "Why does this device show up some days and not others?"
- "I want to see what changed since I last looked"

---

## Persona 2 — Sam, the SMB Sysadmin

**"I need a reliable network inventory I can trust, not babysit."**

**Background:** Sam manages IT for a 50–200 person company — maybe a manufacturing firm, a legal office, or a school. The network has 80–200 hosts across a few VLANs: workstations, servers, printers, switches, access points, and a growing number of IoT devices nobody asked Sam about. Uses Lansweeper or a spreadsheet today but neither is accurate or automatic.

**Goals:**
- Asset inventory that stays current without manual effort
- Know when unauthorized devices appear on the network
- See which machines haven't been scanned recently
- Generate a basic inventory report for management or auditors

**Frustrations:**
- Spreadsheet inventory is always stale
- Lansweeper is expensive and overkill for the network size
- No way to know if that "unknown device" is the CEO's personal hotspot or a rogue switch
- Compliance asks for an asset list and Sam has to scramble

**What Sam values most:** Scheduled discovery, host status tracking (up/down/gone), basic reporting, low maintenance overhead.

**What Sam doesn't need:** Deep packet inspection, CLI-only workflows, heavy infrastructure requirements.

**Representative asks:**
- "Can I schedule it to scan every night and email me changes?"
- "I need to export a list of all hosts and their open ports for the audit"
- "How do I flag devices that don't belong?"

---

## Persona 3 — Jordan, the Security-Conscious DevOps Engineer

**"I need rich data I can act on, not just a list of IPs."**

**Background:** Jordan works at a 20–200 person tech company managing hybrid cloud + on-prem infrastructure. Runs Kubernetes, has servers both in data centres and cloud VPCs, manages CI/CD pipelines. Security isn't Jordan's job title but it's in the job description. Regularly asked by the CISO: "what's actually exposed on the internal network?"

**Goals:**
- Know exactly what services are running and on what version
- Track TLS certificate expiry across all services
- Detect unauthorized services (e.g., someone ran a dev server on port 8080 in prod)
- Feed scan data into Grafana/Prometheus or trigger webhooks on change events
- API access to query scan results programmatically

**Frustrations:**
- nmap gives great data but storing and querying it is a pain
- No easy way to know when a cert is about to expire across 50 hosts
- Can't correlate "new host appeared" with "deploy pipeline ran"
- Wants structured data, not HTML tables

**What Jordan values most:** Service version detection, TLS inventory, structured API, metrics export, webhook integration.

**What Jordan doesn't need:** Simplified UI, onboarding wizard, "beginner" profile recommendations.

**Representative asks:**
- "Can I query the scan results via API?"
- "I need an alert when a new port opens on the prod network"
- "Show me all TLS certs expiring in the next 30 days"

---

## Persona 4 — Morgan, the MSP Technician

**"I manage 15 client networks. I need to switch context fast and produce proof of work."**

**Background:** Morgan works at a Managed Service Provider, responsible for monitoring and maintaining networks for 10–20 small business clients. Each client has 20–100 devices. Morgan needs to keep a separate view per client, run scans on demand or on schedule, and produce monthly reports showing what was done and what was found.

**Goals:**
- Separate network namespaces per client (or clearly labelled)
- Run targeted scans per network/group
- Generate a per-client report: inventory, changes this month, anomalies
- Quick context-switching between clients without confusion

**Frustrations:**
- Most tools assume a single network — multi-network management is an afterthought
- Reports are either too technical (raw nmap XML) or don't exist
- Can't quickly show a client "here's what changed on your network this month"
- Scheduling is per-tool, not per-client

**What Morgan values most:** Multi-network organization, host groups, scheduled per-network scans, exportable reports.

**What Morgan doesn't need:** Deep packet inspection, raw protocol debugging, developer API.

**Representative asks:**
- "Can I have one profile for Client A's network and a different one for Client B?"
- "I need a PDF I can send to the client showing this month's scan results"
- "Set up a weekly scan for each client network automatically"

---

## Updating personas

To add a persona: follow the same structure — quote, background, goals, frustrations, values, doesn't need, representative asks.

To retire a persona: move it to a `## Archived personas` section at the bottom with a note on why it was retired (e.g., "too similar to Sam, merged").

To refine a persona: edit in place and update the `Last updated` date at the top.

When a new feature request comes in that doesn't fit any existing persona, consider whether it signals a missing persona rather than an edge case.
