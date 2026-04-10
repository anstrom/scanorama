import { useState, useEffect, useRef } from "react";
import { useWs } from "../lib/use-ws";
import type { WsMessage } from "../lib/ws";

// ── Event model ───────────────────────────────────────────────────────────────

export type ActivityEventKind =
  | "scan_started"
  | "scan_completed"
  | "scan_failed"
  | "discovery_started"
  | "discovery_completed"
  | "host_status_change";

export interface ActivityEvent {
  id: string;
  kind: ActivityEventKind;
  title: string;
  detail: string;
  timestamp: string;
  href?: string; // optional navigation target
}

const MAX_EVENTS = 100;

// ── Parsers ───────────────────────────────────────────────────────────────────

function parseScanUpdate(msg: WsMessage): ActivityEvent | null {
  const d = msg.data as Record<string, unknown>;
  const scanId = String(d.scan_id ?? "");
  const status = String(d.status ?? "");
  const ts = (msg.timestamp as string) ?? new Date().toISOString();

  if (status === "running" || status === "queued") {
    return {
      id: `scan-${scanId}-started-${ts}`,
      kind: "scan_started",
      title: "Scan started",
      detail: scanId ? `Scan #${scanId}` : "Scan",
      timestamp: ts,
      href: scanId ? `/scans` : undefined,
    };
  }
  if (status === "completed") {
    const count = d.results_count as number | undefined;
    return {
      id: `scan-${scanId}-completed-${ts}`,
      kind: "scan_completed",
      title: "Scan completed",
      detail: `${count != null ? `${count} results` : ""}${scanId ? ` · #${scanId}` : ""}`.trim(),
      timestamp: ts,
      href: `/scans`,
    };
  }
  if (status === "failed" || status === "error") {
    const errMsg = d.error ? String(d.error) : "Unknown error";
    return {
      id: `scan-${scanId}-failed-${ts}`,
      kind: "scan_failed",
      title: "Scan failed",
      detail: errMsg.slice(0, 80),
      timestamp: ts,
      href: `/scans`,
    };
  }
  return null;
}

function parseDiscoveryUpdate(msg: WsMessage): ActivityEvent | null {
  const d = msg.data as Record<string, unknown>;
  const jobId = String(d.job_id ?? "");
  const status = String(d.status ?? "");
  const ts = (msg.timestamp as string) ?? new Date().toISOString();

  if (status === "running") {
    return {
      id: `disc-${jobId}-started-${ts}`,
      kind: "discovery_started",
      title: "Discovery started",
      detail: jobId ? `Job ${jobId.slice(0, 8)}` : "Discovery",
      timestamp: ts,
      href: `/discovery`,
    };
  }
  if (status === "completed") {
    const newH = (d.new_hosts_count as number | undefined) ?? 0;
    const goneH = (d.gone_hosts_count as number | undefined) ?? 0;
    const detail = [
      newH > 0 ? `+${newH} new` : "",
      goneH > 0 ? `-${goneH} gone` : "",
    ]
      .filter(Boolean)
      .join(", ");
    return {
      id: `disc-${jobId}-completed-${ts}`,
      kind: "discovery_completed",
      title: "Discovery completed",
      detail: detail || "No changes",
      timestamp: ts,
      href: `/discovery`,
    };
  }
  return null;
}

function parseHostStatusChange(msg: WsMessage): ActivityEvent | null {
  const d = msg.data as Record<string, unknown>;
  const ip = String(d.ip_address ?? "");
  const newStatus = String(d.new_status ?? d.status ?? "");
  const ts = (msg.timestamp as string) ?? new Date().toISOString();

  if (!ip || !newStatus) return null;

  const title =
    newStatus === "up"
      ? "Host online"
      : newStatus === "down"
        ? "Host offline"
        : newStatus === "gone"
          ? "Host gone"
          : "Host status changed";

  return {
    id: `host-${ip}-${newStatus}-${ts}`,
    kind: "host_status_change",
    title,
    detail: ip,
    timestamp: ts,
    href: `/hosts`,
  };
}

// ── Hook ──────────────────────────────────────────────────────────────────────

export function useActivityFeed(): ActivityEvent[] {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const { manager } = useWs();
  // Track last-seen scan/discovery state to suppress duplicate progress ticks
  const lastScanStatus = useRef<Record<string, string>>({});
  const lastDiscStatus = useRef<Record<string, string>>({});

  useEffect(() => {
    if (!manager) return;

    function addEvent(ev: ActivityEvent | null) {
      if (!ev) return;
      setEvents((prev) => {
        // Deduplicate by id
        if (prev.some((e) => e.id === ev.id)) return prev;
        return [ev, ...prev].slice(0, MAX_EVENTS);
      });
    }

    const unsubScan = manager.on("scan_update", (msg: WsMessage) => {
      const d = msg.data as Record<string, unknown>;
      const scanId = String(d.scan_id ?? "");
      const status = String(d.status ?? "");
      // Only fire on state transitions, not repeated progress ticks
      if (lastScanStatus.current[scanId] === status) return;
      lastScanStatus.current[scanId] = status;
      addEvent(parseScanUpdate(msg));
    });

    const unsubDisc = manager.on("discovery_update", (msg: WsMessage) => {
      const d = msg.data as Record<string, unknown>;
      const jobId = String(d.job_id ?? "");
      const status = String(d.status ?? "");
      if (lastDiscStatus.current[jobId] === status) return;
      lastDiscStatus.current[jobId] = status;
      addEvent(parseDiscoveryUpdate(msg));
    });

    const unsubHost = manager.on("host_status_change", (msg: WsMessage) => {
      addEvent(parseHostStatusChange(msg));
    });

    return () => {
      unsubScan();
      unsubDisc();
      unsubHost();
    };
  }, [manager]);

  return events;
}
