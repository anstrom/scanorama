import { useState, useCallback } from "react";
import { ScanLine } from "lucide-react";
import { Button } from "../components/button";
import { useScans } from "../api/hooks/use-scans";
import {
  StatusBadge,
  Skeleton,
  ScanDetailPanel,
  PaginationBar,
  RunScanModal,
} from "../components";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";

const PAGE_SIZE = 25;

type ScanStatus = "all" | "pending" | "running" | "completed" | "failed";

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

  const { data, isLoading, isError } = useScans(queryParams);

  const scans = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 0;

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
                {isError ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="py-10 text-center text-xs text-danger"
                    >
                      Failed to load scans.
                    </td>
                  </tr>
                ) : isLoading ? (
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
          {!isLoading && scans.length > 0 && totalPages > 1 && (
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
