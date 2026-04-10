import { useEffect, useRef } from "react";
import {
  ScanLine,
  CheckCircle2,
  XCircle,
  ScanSearch,
  Server,
  Radio,
} from "lucide-react";
import { cn, formatRelativeTime } from "../lib/utils";
import { useActivityFeed } from "../hooks/use-activity-feed";
import { useWsStatus } from "../lib/use-ws";
import type { ActivityEventKind } from "../hooks/use-activity-feed";

// ── Icon + color per event kind ───────────────────────────────────────────────

function eventIcon(kind: ActivityEventKind) {
  switch (kind) {
    case "scan_started":
      return ScanLine;
    case "scan_completed":
      return CheckCircle2;
    case "scan_failed":
      return XCircle;
    case "discovery_started":
    case "discovery_completed":
      return ScanSearch;
    case "host_status_change":
      return Server;
  }
}

function eventColorClass(kind: ActivityEventKind): string {
  switch (kind) {
    case "scan_started":
      return "text-accent";
    case "scan_completed":
      return "text-success";
    case "scan_failed":
      return "text-danger";
    case "discovery_started":
      return "text-accent";
    case "discovery_completed":
      return "text-success";
    case "host_status_change":
      return "text-warning";
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

export function ActivityFeed() {
  const events = useActivityFeed();
  const wsStatus = useWsStatus();
  const listRef = useRef<HTMLUListElement>(null);

  // Auto-scroll to top when new event arrives
  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = 0;
    }
  }, [events.length]);

  return (
    <div className="bg-surface rounded-lg border border-border overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Radio className="h-4 w-4 text-text-muted" />
          <span className="text-xs font-medium text-text-primary">
            Activity
          </span>
        </div>
        <span
          className={cn(
            "text-[10px] font-medium px-1.5 py-0.5 rounded",
            wsStatus === "connected"
              ? "bg-success/15 text-success"
              : wsStatus === "connecting"
                ? "bg-warning/15 text-warning"
                : "bg-text-muted/15 text-text-muted",
          )}
        >
          {wsStatus === "connected"
            ? "Live"
            : wsStatus === "connecting"
              ? "Connecting…"
              : "Disconnected"}
        </span>
      </div>

      {/* Feed */}
      <ul
        ref={listRef}
        className="max-h-72 overflow-y-auto divide-y divide-border/40"
      >
        {events.length === 0 ? (
          <li className="px-4 py-8 text-center text-xs text-text-muted">
            No recent activity
          </li>
        ) : (
          events.slice(0, 20).map((ev) => {
            const Icon = eventIcon(ev.kind);
            return (
              <li key={ev.id}>
                {ev.href ? (
                  <a
                    href={`#${ev.href}`}
                    className="flex items-start gap-3 px-4 py-2.5 hover:bg-surface-raised transition-colors"
                  >
                    <Icon
                      className={cn(
                        "h-3.5 w-3.5 shrink-0 mt-0.5",
                        eventColorClass(ev.kind),
                      )}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium text-text-primary truncate">
                        {ev.title}
                      </p>
                      {ev.detail && (
                        <p className="text-[11px] text-text-muted truncate">
                          {ev.detail}
                        </p>
                      )}
                    </div>
                    <span className="text-[10px] text-text-muted whitespace-nowrap shrink-0 mt-0.5">
                      {formatRelativeTime(ev.timestamp)}
                    </span>
                  </a>
                ) : (
                  <div className="flex items-start gap-3 px-4 py-2.5">
                    <Icon
                      className={cn(
                        "h-3.5 w-3.5 shrink-0 mt-0.5",
                        eventColorClass(ev.kind),
                      )}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium text-text-primary truncate">
                        {ev.title}
                      </p>
                      {ev.detail && (
                        <p className="text-[11px] text-text-muted truncate">
                          {ev.detail}
                        </p>
                      )}
                    </div>
                    <span className="text-[10px] text-text-muted whitespace-nowrap shrink-0 mt-0.5">
                      {formatRelativeTime(ev.timestamp)}
                    </span>
                  </div>
                )}
              </li>
            );
          })
        )}
      </ul>
    </div>
  );
}
