import { useState, useCallback } from "react";
import { X } from "lucide-react";
import { useScans } from "../api/hooks/use-scans";
import { useScanResults } from "../api/hooks/use-scans";
import { StatusBadge, Skeleton, PaginationBar } from "../components";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type ScanResponse = components["schemas"]["docs.ScanResponse"];

const PAGE_SIZE = 25;
const RESULTS_PAGE_SIZE = 20;

type ScanStatus =
  | "all"
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

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
            <Skeleton className="h-3.5 w-10" />
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

function ScanDetailPanel({ scan, onClose }: DetailPanelProps) {
  const [resultsPage, setResultsPage] = useState(1);
  const { data: resultsData, isLoading: resultsLoading } = useScanResults(
    scan.id ?? "",
    { page: resultsPage, page_size: RESULTS_PAGE_SIZE },
  );

  const allResults = resultsData?.results ?? [];
  // Client-side pagination of the results array since the endpoint returns
  // the full result set in a single call (no server-side pagination param).
  const totalResultPages = Math.max(
    1,
    Math.ceil(allResults.length / RESULTS_PAGE_SIZE),
  );
  const pageResults = allResults.slice(
    (resultsPage - 1) * RESULTS_PAGE_SIZE,
    resultsPage * RESULTS_PAGE_SIZE,
  );

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
          "w-full max-w-[480px]",
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
              <MetaRow label="Profile ID" value={scan.profile_id} />
              <MetaRow
                label="Created"
                value={
                  scan.created_at
                    ? formatRelativeTime(scan.created_at)
                    : undefined
                }
              />
              <MetaRow
                label="Started"
                value={
                  scan.started_at
                    ? formatRelativeTime(scan.started_at)
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
              <MetaRow label="Hosts discovered" value={scan.hosts_discovered} />
              <MetaRow label="Ports scanned" value={scan.ports_scanned} />
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
              Results
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
                      <th className="text-left font-medium pb-2 pr-3">State</th>
                      <th className="text-left font-medium pb-2">Service</th>
                    </tr>
                  </thead>
                  <tbody>
                    <ResultsSkeletonRows count={5} />
                  </tbody>
                </table>
              </div>
            ) : pageResults.length === 0 ? (
              <p className="text-xs text-text-muted">No results found.</p>
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
                        <th className="text-left font-medium pb-2 pr-3">
                          State
                        </th>
                        <th className="text-left font-medium pb-2">Service</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pageResults.map((r, idx) => (
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
                          <td className="py-2 pr-3">
                            {r.state ? (
                              <StatusBadge status={r.state} />
                            ) : (
                              <span className="text-text-muted">—</span>
                            )}
                          </td>
                          <td className="py-2 text-text-secondary">
                            {r.service ?? "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                {totalResultPages > 1 && (
                  <PaginationBar
                    page={resultsPage}
                    totalPages={totalResultPages}
                    onPrev={() => setResultsPage((p) => Math.max(1, p - 1))}
                    onNext={() =>
                      setResultsPage((p) => Math.min(totalResultPages, p + 1))
                    }
                    className="mt-2"
                  />
                )}
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
  const [selectedScan, setSelectedScan] = useState<ScanResponse | null>(null);

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
          </select>
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
                    Hosts
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
                      colSpan={6}
                      className="py-10 text-center text-xs text-text-muted"
                    >
                      No scans found.
                    </td>
                  </tr>
                ) : (
                  scans.map((scan) => (
                    <tr
                      key={scan.id}
                      onClick={() => setSelectedScan(scan)}
                      className={cn(
                        "border-b border-border/50 last:border-0",
                        "hover:bg-surface-raised/50 transition-colors cursor-pointer",
                      )}
                    >
                      <td className="py-3 px-4 pr-4 font-mono text-text-secondary max-w-[200px] truncate">
                        {scan.targets?.join(", ") ?? "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <StatusBadge status={scan.status ?? "unknown"} />
                      </td>
                      <td className="py-3 pr-4 text-text-secondary tabular-nums">
                        {scan.hosts_discovered ?? "—"}
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
      {selectedScan && (
        <ScanDetailPanel
          scan={selectedScan}
          onClose={() => setSelectedScan(null)}
        />
      )}
    </>
  );
}
