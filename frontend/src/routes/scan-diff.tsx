import { useSearch, Link } from "@tanstack/react-router";
import { ArrowLeft, GitCompare } from "lucide-react";
import { useScanDiff } from "../api/hooks/use-scan-diff";
import type { ScanDiffEntry } from "../api/hooks/use-scan-diff";
import { Skeleton } from "../components";
import { cn } from "../lib/utils";

// ── Status badge ──────────────────────────────────────────────────────────────

type DiffStatus = "new" | "closed" | "changed" | "unchanged";

const STATUS_LABELS: Record<DiffStatus, string> = {
  new: "NEW",
  closed: "CLOSED",
  changed: "CHANGED",
  unchanged: "–",
};

const STATUS_CLASSES: Record<DiffStatus, string> = {
  new: "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300",
  closed: "bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300",
  changed: "bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300",
  unchanged: "bg-surface-muted text-text-muted",
};

function DiffBadge({ status }: { status: string }) {
  const s = (status as DiffStatus) in STATUS_LABELS ? (status as DiffStatus) : "unchanged";
  return (
    <span
      className={cn(
        "inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
        STATUS_CLASSES[s],
      )}
    >
      {STATUS_LABELS[s]}
    </span>
  );
}

// ── Loading skeleton ──────────────────────────────────────────────────────────

function DiffSkeleton() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-4 w-48 animate-pulse" />
      <Skeleton className="h-4 w-full animate-pulse" />
      <Skeleton className="h-4 w-full animate-pulse" />
      <Skeleton className="h-4 w-3/4 animate-pulse" />
    </div>
  );
}

// ── Port row ─────────────────────────────────────────────────────────────────

function PortRow({ entry }: { entry: ScanDiffEntry }) {
  const isChanged = entry.status === "changed";
  const isClosed = entry.status === "closed";

  const currentService = entry.service_name ?? "—";
  const prevService = entry.prev_service_name ?? "—";

  return (
    <tr
      className={cn(
        "border-b border-border/50 text-xs last:border-0",
        entry.status === "new" && "bg-emerald-50/40 dark:bg-emerald-950/20",
        isClosed && "bg-red-50/40 dark:bg-red-950/20",
        isChanged && "bg-amber-50/30 dark:bg-amber-950/15",
      )}
    >
      {/* Port / Protocol */}
      <td className="py-2 pr-4 font-mono text-text-primary">
        {entry.port}/{entry.protocol}
      </td>

      {/* Status */}
      <td className="py-2 pr-4">
        <DiffBadge status={entry.status ?? "unchanged"} />
      </td>

      {/* State */}
      <td className="py-2 pr-4 text-text-secondary">
        {entry.state ?? "—"}
      </td>

      {/* Current service */}
      <td className="py-2 pr-4 text-text-secondary">
        {isClosed ? (
          <span className="line-through text-text-muted">{prevService}</span>
        ) : (
          currentService
        )}
      </td>

      {/* Previous service (only shown for changed/closed) */}
      <td className="py-2 text-text-muted">
        {(isChanged || isClosed) ? (
          <span className="line-through">{prevService}</span>
        ) : null}
        {isChanged && entry.prev_state && entry.prev_state !== entry.state && (
          <span className="ml-1 text-text-muted">({entry.prev_state})</span>
        )}
      </td>
    </tr>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

interface ScanDiffSearchParams {
  a?: string;
  b?: string;
}

export function ScanDiffPage() {
  // TanStack Router typed search params
  const search = useSearch({ strict: false }) as ScanDiffSearchParams;
  const scanAId = search.a;
  const scanBId = search.b;

  const { data: diff, isLoading, isError, error } = useScanDiff(scanAId, scanBId);

  // ── Missing params ──────────────────────────────────────────────────────────
  if (!scanAId || !scanBId) {
    return (
      <div className="p-6 text-sm text-text-muted">
        <p>Two scan IDs are required. Use ?a=&lt;id&gt;&amp;b=&lt;id&gt; in the URL.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-6 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link
          to="/scans"
          className="inline-flex items-center gap-1.5 text-xs text-text-muted hover:text-text-primary"
        >
          <ArrowLeft size={14} />
          Back to scans
        </Link>
      </div>

      <div className="flex items-center gap-2">
        <GitCompare size={20} className="text-accent shrink-0" />
        <h1 className="text-lg font-semibold text-text-primary">Scan Comparison</h1>
      </div>

      {/* Scan ID pills */}
      <div className="flex flex-wrap items-center gap-2 text-xs text-text-muted">
        <span className="font-medium text-text-secondary">Baseline:</span>
        <code className="font-mono bg-surface-muted px-2 py-0.5 rounded">{scanAId}</code>
        <span>→</span>
        <span className="font-medium text-text-secondary">Current:</span>
        <code className="font-mono bg-surface-muted px-2 py-0.5 rounded">{scanBId}</code>
      </div>

      {/* Loading */}
      {isLoading && <DiffSkeleton />}

      {/* Error */}
      {isError && (
        <div className="rounded-md border border-border bg-surface p-4 text-sm text-red-600 dark:text-red-400">
          Failed to load diff: {error instanceof Error ? error.message : "Unknown error"}
        </div>
      )}

      {/* Results */}
      {diff && (
        <>
          {/* OS change notice */}
          {diff.os_changed && (
            <div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-2.5 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
              OS changed:{" "}
              <span className="line-through mr-1">{diff.prev_os_name ?? "unknown"}</span>
              →{" "}
              <span className="font-semibold">{diff.curr_os_name ?? "unknown"}</span>
            </div>
          )}

          {/* Summary counts */}
          <div className="flex flex-wrap gap-3 text-sm">
            {(diff.new_count ?? 0) > 0 && (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-100 px-3 py-1 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300">
                <span className="font-semibold">{diff.new_count}</span> new
              </span>
            )}
            {(diff.closed_count ?? 0) > 0 && (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-red-100 px-3 py-1 text-red-800 dark:bg-red-900/40 dark:text-red-300">
                <span className="font-semibold">{diff.closed_count}</span> closed
              </span>
            )}
            {(diff.changed_count ?? 0) > 0 && (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-3 py-1 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">
                <span className="font-semibold">{diff.changed_count}</span> changed
              </span>
            )}
            {(diff.unchanged_count ?? 0) > 0 && (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-surface-muted px-3 py-1 text-text-muted">
                <span className="font-semibold">{diff.unchanged_count}</span> unchanged
              </span>
            )}
            {diff.new_count === 0 &&
              diff.closed_count === 0 &&
              diff.changed_count === 0 &&
              (diff.unchanged_count ?? 0) === 0 && (
              <span className="text-text-muted">No changes detected between these scans.</span>
            )}
          </div>

          {/* Port table */}
          {(diff.ports?.length ?? 0) > 0 ? (
            <div className="overflow-x-auto rounded-lg border border-border">
              <table className="w-full text-xs border-collapse">
                <thead>
                  <tr className="bg-surface-muted text-left text-text-muted text-[11px] uppercase tracking-wider">
                    <th className="px-3 py-2.5 font-medium">Port</th>
                    <th className="px-3 py-2.5 font-medium">Status</th>
                    <th className="px-3 py-2.5 font-medium">State</th>
                    <th className="px-3 py-2.5 font-medium">Service</th>
                    <th className="px-3 py-2.5 font-medium">Previous</th>
                  </tr>
                </thead>
                <tbody>
                  {diff.ports.map((entry, i) => (
                    <PortRow key={`${entry.port}-${entry.protocol}-${i}`} entry={entry} />
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="text-sm text-text-muted">No port data for these scans.</p>
          )}
        </>
      )}
    </div>
  );
}
