# Scanorama — Product Vision & Design Principles

## What it is

Scanorama is a self-hosted network scanning and asset management tool. It gives infrastructure teams a continuously updated, progressively richer picture of their own network — what's there, what's running, what changed, and what needs attention.

It is **not** a vulnerability scanner, a SIEM, or a compliance platform. It is a network inventory and observability tool that happens to use scanning as its primary data collection mechanism.

## Who it's for

Small-to-medium infrastructure teams (1–20 people) who:
- Manage on-premise or hybrid networks they own and have permission to scan
- Don't have budget for enterprise tools (Rapid7, Tenable, Qualys)
- Want more than what nmap alone gives them, but less than a full security platform
- Are comfortable self-hosting a Go binary + SQLite/Postgres

The prototypical user is a sysadmin or DevOps engineer who runs nmap manually today and wants something that automates it, remembers results, and surfaces changes.

## Core philosophy

**Progressive knowledge over single-shot scans.** The system should keep learning. A first run gets you IPs; subsequent runs get you hostnames, OS, services, banners, and credentials. Each pass adds a layer. The user shouldn't need to configure this — the system should figure out what the next logical step is.

**Zero mandatory expertise.** A user who doesn't know what an nmap profile is should still get good scan coverage. Smart defaults, OS-aware profiles, and the Smart Scan orchestrator exist so experts can tune but beginners don't have to.

**Visibility without noise.** Surface what changed, what's new, what's degrading — and stay quiet when nothing interesting is happening. The discovery changelog, host status model, and knowledge score all serve this principle.

**Data stays local.** No cloud dependency, no telemetry, no SaaS. The user's network topology is sensitive. Everything stays in their database.

## Milestone sequencing logic

The milestones were designed to layer value:
- **v0.23** laid the table/UX foundation (bulk ops, sorting, discovery diff)
- **v0.24** added organisation primitives (tags, groups, admin)
- **v0.25** built the intelligence layer (Smart Scan, enrichment tools, profiles)
- **v0.26** turns collected data into insight (analytics, reporting, queries)
- **v0.27** polishes the experience (UX, keyboard nav, onboarding)

Features in later milestones often depend on data collected in earlier ones. Don't pull forward features that require infrastructure not yet built.

## Design principles for feature decisions

1. **Earn complexity.** Every new feature must justify its UI surface. If a feature adds a tab, a setting, or a new page, it needs to be genuinely useful for the core user — not a nice-to-have for edge cases.

2. **Instruments before dashboards.** Collect the data before building the view. A chart without reliable underlying data is worse than no chart.

3. **Tools over magic.** Integrate proven open-source tools (nmap, ZGrab2, GoSNMP, DNSX, Httpx, Nuclei) rather than reimplementing their capabilities. The value is in orchestration, storage, and presentation — not reinventing scan engines.

4. **Scan safety first.** Scanorama scans networks its users control. Never add features that encourage scanning outside the configured networks. Respect rate limits and scan timing by default.

5. **Backward compatibility.** Users running v0.23 should be able to upgrade to v0.26 without data loss or manual migration steps. DB migrations are always additive.

## What's deliberately out of scope

- Scanning the internet / non-owned networks
- User authentication / multi-tenancy (single-user tool)
- Real-time packet capture / IDS functionality
- Automated remediation or patching
- Mobile app
