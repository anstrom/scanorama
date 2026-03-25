import { useState, useEffect, useRef, useMemo, useCallback } from "react";
import { ChevronDown, ChevronUp, Search } from "lucide-react";
import { WsManager } from "../lib/ws";
import type { WsStatus } from "../lib/ws";
import { useLogs } from "../api/hooks/use-system";
import type { LogEntry, LogsParams } from "../api/hooks/use-system";
import { cn, formatRelativeTime, formatAbsoluteTime } from "../lib/utils";
import { Skeleton } from "./skeleton";

// ── Constants ──────────────────────────────────────────────────────────────────────────────

const MAX_WS_ENTRIES = 500;

// ── Helpers ────────────────────────────────────────────────────────────────────────────────

function levelBadgeClass(level: string): string {
  switch (level.toLowerCase()) {
    case "debug":
      return "text-text-muted bg-surface-raised";
    case "info":
      return "text-accent bg-accent/10";
    case "warn":
      return "text-warning bg-warning/10";
    case "error":
      return "text-danger bg-danger/10";
    default:
      return "text-text-muted bg-surface-raised";
  }
}

// Stable key per entry — combines timestamp + index so duplicates are safe
function entryRowKey(entry: LogEntry, idx: number): string {
  return `${entry.time}|${idx}|${entry.message.slice(0, 32)}`;
}

// ── Sub-components ─────────────────────────────────────────────────────────────────────────────

interface LogEntryRowProps {
  entry: LogEntry;
  rowKey: string;
  expanded: boolean;
  onToggle: (key: string) => void;
}

function LogEntryRow({ entry, rowKey, expanded, onToggle }: LogEntryRowProps) {
  const hasAttrs =
    entry.attrs !== undefined && Object.keys(entry.attrs).length > 0;

  return (
    <div className="px-3 py-2 border-b border-border last:border-0 hover:bg-surface-raised/40 transition-colors">
      {/* Main row */}
      <div className="flex items-start gap-2 flex-wrap min-w-0">
        {/* Level badge */}
        <span
          className={cn(
            "inline-flex items-center px-1.5 rounded text-[10px] font-semibold uppercase shrink-0 leading-5",
            levelBadgeClass(entry.level),
          )}
        >
          {entry.level.slice(0, 5)}
        </span>

        {/* Timestamp — absolute time on hover */}
        <span
          className="text-text-muted text-[11px] shrink-0 whitespace-nowrap leading-5 cursor-default"
          title={formatAbsoluteTime(entry.time)}
        >
          {formatRelativeTime(entry.time)}
        </span>

        {/* Component badge */}
        {entry.component !== undefined && entry.component !== "" && (
          <span className="inline-flex items-center px-1.5 rounded text-[10px] bg-surface-raised text-text-secondary shrink-0 leading-5">
            {entry.component}
          </span>
        )}

        {/* Message */}
        <span className="text-text-primary flex-1 min-w-0 wrap-break-word leading-5 text-[11px]">
          {entry.message}
        </span>

        {/* Expand / collapse attrs */}
        {hasAttrs && (
          <button
            onClick={() => onToggle(rowKey)}
            className="shrink-0 text-text-muted hover:text-text-secondary transition-colors leading-5 mt-0.5"
            title={expanded ? "Hide attributes" : "Show attributes"}
          >
            {expanded ? (
              <ChevronUp className="h-3 w-3" />
            ) : (
              <ChevronDown className="h-3 w-3" />
            )}
          </button>
        )}
      </div>

      {/* Expanded attrs */}
      {expanded && entry.attrs !== undefined && (
        <div className="mt-1.5 ml-1 pl-3 border-l-2 border-border space-y-0.5">
          {Object.entries(entry.attrs).map(([k, v]) => (
            <div key={k} className="text-[10px]">
              <span className="text-text-muted">{k}:</span>{" "}
              <span className="text-text-secondary">{v}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Main component ─────────────────────────────────────────────────────────────────────────────

export function LogViewer() {
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(1);
  const [wsEntries, setWsEntries] = useState<LogEntry[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const listRef = useRef<HTMLDivElement>(null);

  const [manager, setManager] = useState<WsManager | null>(null);
  const [status, setStatus] = useState<WsStatus>("disconnected");

  // Dedicated WS connection to the logs endpoint
  useEffect(() => {
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${wsProtocol}//${window.location.host}/api/v1/ws/logs`;
    const apiKey = import.meta.env.VITE_API_KEY ?? "";
    const mgr = new WsManager(url, apiKey);
    const unsub = mgr.onStatusChange((s) => {
      setStatus(s);
    });
    const timerId = setTimeout(() => {
      mgr.connect();
      setManager(mgr);
    }, 0);
    return () => {
      clearTimeout(timerId);
      unsub();
      mgr.disconnect();
      setManager(null);
      setStatus("disconnected");
    };
  }, []);

  // ── Debounce search ──────────────────────────────────────────────────────────────────────────────

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // ── Level filter handlers ─────────────────────────────────────────────────────────────────────────────

  const toggleLevel = useCallback((level: string) => {
    setSelectedLevels((prev) => {
      const next = new Set(prev);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next;
    });
    setPage(1);
  }, []);

  const clearLevels = useCallback(() => {
    setSelectedLevels(new Set());
    setPage(1);
  }, []);

  // ── REST query ────────────────────────────────────────────────────────────────────────────────

  const params = useMemo<LogsParams>(
    () => ({
      page,
      page_size: 50,
      ...(debouncedSearch !== "" && { search: debouncedSearch }),
    }),
    [page, debouncedSearch],
  );

  const { data: restData, isLoading } = useLogs(params);

  // ── WS subscriptions ─────────────────────────────────────────────────────────────────────────────
  //
  // Subscribes to log_history (history burst on connect) and log_entry (stream).
  // Note: the backend serves these on /api/v1/ws/logs. They will appear here if
  // the general /api/v1/ws endpoint also relays them, or when the WsProvider URL
  // is pointed at /api/v1/ws/logs.

  useEffect(() => {
    if (manager === null) return;

    const unsubHistory = manager.on("log_history", (msg) => {
      const payload = msg.data as { entries: LogEntry[]; total: number };
      if (Array.isArray(payload?.entries)) {
        setWsEntries(payload.entries.slice(-MAX_WS_ENTRIES));
      }
    });

    const unsubEntry = manager.on("log_entry", (msg) => {
      const entry = msg.data as LogEntry;
      setWsEntries((prev) => {
        const next = [...prev, entry];
        return next.length > MAX_WS_ENTRIES
          ? next.slice(-MAX_WS_ENTRIES)
          : next;
      });
    });

    return () => {
      unsubHistory();
      unsubEntry();
    };
  }, [manager]);

  // ── Auto-scroll ────────────────────────────────────────────────────────────────────────────────

  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [wsEntries, autoScroll]);

  // ── Displayed entries ──────────────────────────────────────────────────────────────────────────────

  const isLive = status === "connected";

  const levelMatches = useCallback(
    (entry: LogEntry) =>
      selectedLevels.size === 0 ||
      selectedLevels.has(entry.level.toLowerCase()),
    [selectedLevels],
  );

  const displayEntries = useMemo<LogEntry[]>(() => {
    if (isLive && wsEntries.length > 0) {
      // Client-side filter when WS is providing data
      return wsEntries.filter((entry) => {
        if (!levelMatches(entry)) return false;
        if (
          debouncedSearch !== "" &&
          !entry.message.toLowerCase().includes(debouncedSearch.toLowerCase())
        ) {
          return false;
        }
        return true;
      });
    }
    // Fall back to REST — apply client-side level filter
    return (restData?.data ?? []).filter(levelMatches);
  }, [isLive, wsEntries, restData, levelMatches, debouncedSearch]);

  const pagination = restData?.pagination;
  const hasMore =
    !isLive && pagination !== undefined && page < pagination.total_pages;

  // ── Expand/collapse attrs ─────────────────────────────────────────────────────────────────────────────

  const toggleExpand = useCallback((key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  // ── Render ────────────────────────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Level filter pills */}
        <div className="flex items-center gap-1">
          {/* All pill */}
          <button
            onClick={clearLevels}
            className={cn(
              "px-2 py-0.5 text-xs rounded transition-colors",
              selectedLevels.size === 0
                ? "bg-accent/20 text-accent font-medium"
                : "text-text-secondary hover:text-text-primary hover:bg-surface-raised",
            )}
          >
            All
          </button>

          {/* Individual level pills */}
          {(["debug", "info", "warn", "error"] as const).map((level) => (
            <button
              key={level}
              onClick={() => toggleLevel(level)}
              className={cn(
                "px-2 py-0.5 text-xs rounded transition-colors",
                selectedLevels.has(level)
                  ? "bg-accent/20 text-accent font-medium"
                  : "text-text-secondary hover:text-text-primary hover:bg-surface-raised",
              )}
            >
              {level.charAt(0).toUpperCase() + level.slice(1)}
            </button>
          ))}
        </div>

        {/* Search */}
        <div className="relative flex-1 min-w-35">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-text-muted pointer-events-none" />
          <input
            type="text"
            placeholder="Search messages…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className={cn(
              "w-full pl-6 pr-2 py-0.5 text-xs rounded border",
              "bg-surface-raised border-border text-text-primary",
              "placeholder:text-text-muted",
              "focus:outline-none focus:border-accent/50",
            )}
          />
        </div>

        {/* Live / Polling indicator */}
        <div className="flex items-center gap-1.5 text-xs shrink-0">
          <span
            className={cn(
              "w-1.5 h-1.5 rounded-full shrink-0",
              isLive ? "bg-success animate-pulse" : "bg-text-muted",
            )}
          />
          <span className={cn("text-text-muted", isLive && "text-success")}>
            {isLive ? "Live" : "Polling"}
          </span>
        </div>

        {/* Auto-scroll */}
        <label className="flex items-center gap-1.5 text-xs text-text-muted cursor-pointer select-none shrink-0">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
            className="w-3 h-3 accent-accent"
          />
          Auto-scroll
        </label>
      </div>

      {/* Log entries list */}
      <div
        ref={listRef}
        className="max-h-96 overflow-y-auto rounded border border-border bg-background font-mono text-xs"
      >
        {/* Loading skeleton */}
        {isLoading && !isLive && (
          <div className="p-3 space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-2">
                <Skeleton className="h-4 w-10 shrink-0" />
                <Skeleton className="h-4 w-16 shrink-0" />
                <Skeleton className="h-4 flex-1" />
              </div>
            ))}
          </div>
        )}

        {/* Empty state */}
        {!isLoading && displayEntries.length === 0 && (
          <div className="flex items-center justify-center py-12 text-text-muted text-xs font-sans">
            No log entries found
          </div>
        )}

        {/* Entry rows */}
        {(!isLoading || isLive) &&
          displayEntries.map((entry, idx) => {
            const key = entryRowKey(entry, idx);
            return (
              <LogEntryRow
                key={key}
                entry={entry}
                rowKey={key}
                expanded={expandedKeys.has(key)}
                onToggle={toggleExpand}
              />
            );
          })}
      </div>

      {/* Load more */}
      {hasMore && (
        <div className="flex justify-center pt-1">
          <button
            onClick={() => setPage((p) => p + 1)}
            className={cn(
              "px-3 py-1 text-xs rounded border border-border",
              "text-text-secondary hover:text-text-primary hover:bg-surface-raised",
              "transition-colors",
            )}
          >
            Load more
            {pagination !== undefined && (
              <span className="ml-1.5 text-text-muted">
                ({page} / {pagination.total_pages})
              </span>
            )}
          </button>
        </div>
      )}
    </div>
  );
}
