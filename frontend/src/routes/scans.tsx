import { useState, useCallback } from "react";
import { X, ScanLine } from "lucide-react";
import { Button } from "../components/button";
import { useScans } from "../api/hooks/use-scans";
import { useScanResults } from "../api/hooks/use-scans";
import { useProfile } from "../api/hooks/use-profiles";
import {
  StatusBadge,
  Skeleton,
  PaginationBar,
  RunScanModal,
} from "../components";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type ScanResponse = components["schemas"]["docs.ScanResponse"];

const PAGE_SIZE = 25;

type ScanStatus =
  | "all"
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "stopped";

function SkeletonRows({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="border-b border-border/50">
          <td className="py-3 px-4 pr-4">
            <Skeleton className="h-3.5 w-40" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-5 w-20" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-12" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-16" />
          </td>
          <td className="py-3">
            <Skeleton className="h-3.5 w-20" />
          </td>
        </tr>
      ))}
    </>
  );
}

// ──────────────────────────────────────────────
// Detail panel — shown when a scan row is clicked
// ──────────────────────────────────────────────

interface DetailPanelProps {
  scan: ScanResponse;
  onClose: () => void;
}

function MetaRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-xs">
      <span className="text-text-muted w-28 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value ?? "—"}</span>
    </div>
  );
}

function ResultsSkeletonRows({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="border-b border-border/50">
          <td className="py-2 pr-3">
            <Skeleton className="h-3 w-24" />
          </td>
          <td className="py-2 pr-3">
            <Skeleton className="h-3 w-28" />
          </td>
          <td className="py-2 pr-3">
            <Skeleton className="h-3 w-10" />
          </td>
          <td className="py-2 pr-3">
            <Skeleton className="h-3 w-8" />
          </td>
          <td className="py-2 pr-3">
            <Skeleton className="h-3 w-12" />
          </td>
          <td className="py-2">
            <Skeleton className="h-3 w-16" />
          </td>
        </tr>
      ))}
    </>
  );
}

export function ScanDetailPanel({ scan, onClose }: DetailPanelProps) {
  const { data: profileData, isLoading: profileLoading } = useProfile(
    scan.profile_id,
  );
  const { data: resultsData, isLoading: resultsLoading } = useScanResults(
    scan.id ?? "",
    scan.status,
  );

  const allResults = resultsData?.results ?? [];

  // Port state counts derived from the full result set.
  const portCounts = allResults.reduce<Record<string, number>>((acc, r) => {
    const state = r.state ?? "unknown";
    acc[state] = (acc[state] ?? 0) + 1;
    return acc;
  }, {});

  // Unique IPs that responded in this scan.
  const uniqueHostCount = new Set(
    allResults.map((r) => r.host_ip).filter(Boolean),
  ).size;

  // OS info — take from the first result that has it (same host across all rows).
  const osInfo = allResults.find((r) => r.os_name || r.os_family);
  const osLabel = osInfo
    ? [
        osInfo.os_name,
        osInfo.os_confidence != null
          ? `(${osInfo.os_confidence}% confidence)`
          : null,
      ]
        .filter(Boolean)
        .join(" ")
    : undefined;

  // Only show open ports in the results table.
  const openResults = allResults.filter((r) => r.state === "open");

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Panel */}
      <div
        role="dialog"
        aria-label="Scan details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-120",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden",
          "shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-xs text-text-muted">Scan targets</p>
            <p className="text-sm font-mono text-text-primary truncate">
              {scan.targets?.join(", ") ?? "—"}
            </p>
            <StatusBadge status={scan.status ?? "unknown"} />
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close panel"
            className="shrink-0 p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Scrollable body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
          {/* Metadata */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Details
            </h3>
            <div className="space-y-2">
              <MetaRow label="ID" value={scan.id} />
              <MetaRow label="Name" value={scan.name} />
              {scan.profile_id && (
                <MetaRow
                  label="Profile"
                  value={
                    profileLoading ? (
                      <Skeleton className="h-3 w-28 inline-block" />
                    ) : (
                      (profileData?.name ?? scan.profile_id)
                    )
                  }
                />
              )}
              <MetaRow label="Scan type" value={scan.scan_type} />
              <MetaRow label="Ports" value={scan.ports} />
              {!resultsLoading && osInfo && (
                <>
                  <MetaRow label="OS" value={osLabel} />
                  {osInfo.os_family && (
                    <MetaRow label="OS family" value={osInfo.os_family} />
                  )}
                  {osInfo.os_version && (
                    <MetaRow label="OS version" value={osInfo.os_version} />
                  )}
                </>
              )}
              <MetaRow
                label="Hosts"
                value={
                  resultsLoading
                    ? "…"
                    : uniqueHostCount > 0
                      ? `${uniqueHostCount} responding`
                      : undefined
                }
              />
              <MetaRow
                label="Completed"
                value={
                  scan.completed_at
                    ? formatRelativeTime(scan.completed_at)
                    : undefined
                }
              />
              <MetaRow label="Duration" value={scan.duration} />
              <MetaRow
                label="Ports scanned"
                value={
                  resultsLoading ? (
                    "…"
                  ) : allResults.length > 0 ? (
                    <span className="flex flex-wrap gap-2">
                      {Object.entries(portCounts)
                        .sort(([a], [b]) => a.localeCompare(b))
                        .map(([state, count]) => (
                          <span key={state} className="flex items-center gap-1">
                            <StatusBadge status={state} />
                            <span className="tabular-nums text-text-secondary">
                              {count}
                            </span>
                          </span>
                        ))}
                    </span>
                  ) : (
                    (scan.ports_scanned ?? "—")
                  )
                }
              />
              {scan.error_message && (
                <MetaRow
                  label="Error"
                  value={
                    <span className="text-danger">{scan.error_message}</span>
                  }
                />
              )}
            </div>
          </section>

          {/* Results */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              {resultsLoading
                ? "Results"
                : `Open Ports (${openResults.length})`}
            </h3>

            {resultsLoading ? (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="border-b border-border text-text-muted">
                      <th className="text-left font-medium pb-2 pr-3">
                        Host IP
                      </th>
                      <th className="text-left font-medium pb-2 pr-3">
                        Hostname
                      </th>
                      <th className="text-left font-medium pb-2 pr-3">Port</th>
                      <th className="text-left font-medium pb-2 pr-3">
                        Protocol
                      </th>
                      <th className="text-left font-medium pb-2">Service</th>
                    </tr>
                  </thead>
                  <tbody>
                    <ResultsSkeletonRows count={5} />
                  </tbody>
                </table>
              </div>
            ) : openResults.length === 0 ? (
              <p className="text-xs text-text-muted">
                {allResults.length > 0
                  ? "No open ports found."
                  : "No results found."}
              </p>
            ) : (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-border text-text-muted">
                        <th className="text-left font-medium pb-2 pr-3">
                          Host IP
                        </th>
                        <th className="text-left font-medium pb-2 pr-3">
                          Hostname
                        </th>
                        <th className="text-left font-medium pb-2 pr-3">
                          Port
                        </th>
                        <th className="text-left font-medium pb-2 pr-3">
                          Protocol
                        </th>
                        <th className="text-left font-medium pb-2">Service</th>
                      </tr>
                    </thead>
                    <tbody>
                      {openResults.map((r, idx) => (
                        <tr
                          key={r.id ?? idx}
                          className="border-b border-border/50 last:border-0"
                        >
                          <td className="py-2 pr-3 font-mono text-text-primary whitespace-nowrap">
                            {r.host_ip ?? "—"}
                          </td>
                          <td className="py-2 pr-3 text-text-secondary">
                            {r.hostname ?? "—"}
                          </td>
                          <td className="py-2 pr-3 font-mono text-text-secondary tabular-nums">
                            {r.port ?? "—"}
                          </td>
                          <td className="py-2 pr-3 text-text-muted">
                            {r.protocol ?? "—"}
                          </td>
                          <td className="py-2 text-text-secondary">
                            {r.service ?? "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </section>
        </div>
      </div>
    </>
  );
}

// ──────────────────────────────────────────────
// Main page
// ──────────────────────────────────────────────

export function ScansPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<ScanStatus>("all");
  const [selectedScanId, setSelectedScanId] = useState<string | null>(null);
  const [showRunScan, setShowRunScan] = useState(false);

  const handleStatusChange = useCallback((value: ScanStatus) => {
    setStatusFilter(value);
    setPage(1);
  }, []);

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    ...(statusFilter !== "all" ? { status: statusFilter } : {}),
  };

  const { data, isLoading } = useScans(queryParams);

  const scans = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  // Clamp page back when a filter change reduces total_pages below current page.
  if (!isLoading && totalPages > 0 && page > totalPages) {
    setPage(totalPages);
  }

  return (
    <>
      <div className="space-y-4">
        {/* Filter bar */}
        <div className="flex flex-col sm:flex-row gap-3">
          <select
            value={statusFilter}
            onChange={(e) => handleStatusChange(e.target.value as ScanStatus)}
            className={cn(
              "px-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
            aria-label="Filter by status"
          >
            <option value="all">All statuses</option>
            <option value="pending">Pending</option>
            <option value="running">Running</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="cancelled">Cancelled</option>
            <option value="stopped">Stopped</option>
          </select>

          <Button
            onClick={() => setShowRunScan(true)}
            icon={<ScanLine className="h-3.5 w-3.5" />}
            className="sm:ml-auto"
          >
            New scan
          </Button>
        </div>

        {/* Table card */}
        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border bg-surface">
                  <th className="text-left font-medium text-text-muted px-4 py-3 pr-4">
                    Targets
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Status
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Ports
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Duration
                  </th>
                  <th className="text-left font-medium text-text-muted py-3">
                    Started
                  </th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <SkeletonRows count={8} />
                ) : scans.length === 0 ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="py-10 text-center text-xs text-text-muted"
                    >
                      No scans found.
                    </td>
                  </tr>
                ) : (
                  scans.map((scan) => (
                    <tr
                      key={scan.id}
                      onClick={() => setSelectedScanId(scan.id ?? null)}
                      className={cn(
                        "border-b border-border/50 last:border-0",
                        "hover:bg-surface-raised/50 transition-colors cursor-pointer",
                      )}
                    >
                      <td className="py-3 px-4 pr-4 font-mono text-text-secondary max-w-50 truncate">
                        {scan.targets?.join(", ") ?? "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <StatusBadge status={scan.status ?? "unknown"} />
                      </td>
                      <td className="py-3 pr-4 text-text-secondary tabular-nums">
                        {scan.ports_scanned ?? "—"}
                      </td>
                      <td className="py-3 pr-4 text-text-muted">
                        {scan.duration ?? "—"}
                      </td>
                      <td className="py-3 text-text-muted whitespace-nowrap">
                        {scan.started_at
                          ? formatRelativeTime(scan.started_at)
                          : "—"}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {!isLoading && scans.length > 0 && (
            <div className="px-4 pb-3">
              <PaginationBar
                page={page}
                totalPages={totalPages}
                onPrev={() => setPage((p) => Math.max(1, p - 1))}
                onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
              />
            </div>
          )}
        </div>
      </div>

      {/* Detail panel */}
      {(() => {
        const liveScan = selectedScanId
          ? (scans.find((s) => s.id === selectedScanId) ?? null)
          : null;
        return liveScan ? (
          <ScanDetailPanel
            scan={liveScan}
            onClose={() => setSelectedScanId(null)}
          />
        ) : null;
      })()}

      {/* Run scan modal */}
      {showRunScan && <RunScanModal onClose={() => setShowRunScan(false)} />}
    </>
  );
}
