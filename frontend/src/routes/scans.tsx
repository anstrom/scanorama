import { useState, useCallback, useEffect } from "react";
import { ScanLine } from "lucide-react";
import { SortHeader } from "../components/sort-header";
import type { SortOrder } from "../components/sort-header";
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
import { ColumnToggle } from "../components/column-toggle";
import type { ColumnDef } from "../components/column-toggle";
import { useTableKeyNav } from "../hooks/use-table-key-nav";

const PAGE_SIZE = 25;

type ScanStatus = "all" | "pending" | "running" | "completed" | "failed";

const SCAN_COLUMNS: ColumnDef[] = [
  { key: "targets", label: "Targets", alwaysVisible: true },
  { key: "status", label: "Status", alwaysVisible: true },
  { key: "ports", label: "Ports" },
  { key: "duration", label: "Duration" },
  { key: "started", label: "Started" },
];

function SkeletonRows({
  count,
  colVis,
}: {
  count: number;
  colVis: Record<string, boolean>;
}) {
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
          {colVis.ports && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-12" />
            </td>
          )}
          {colVis.duration && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-16" />
            </td>
          )}
          {colVis.started && (
            <td className="py-3">
              <Skeleton className="h-3.5 w-20" />
            </td>
          )}
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
  const [sortBy, setSortBy] = useState("created_at");
  const [sortOrder, setSortOrder] = useState<SortOrder>("desc");
  const [selectedScanId, setSelectedScanId] = useState<string | null>(null);
  const [showRunScan, setShowRunScan] = useState(false);
  const [colVis, setColVis] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(SCAN_COLUMNS.map((c) => [c.key, true])),
  );

  const toggleCol = useCallback((key: string) => {
    const col = SCAN_COLUMNS.find((c) => c.key === key);
    if (col?.alwaysVisible) return;
    setColVis((prev) => ({ ...prev, [key]: !prev[key] }));
  }, []);

  const handleStatusChange = useCallback((value: ScanStatus) => {
    setStatusFilter(value);
    setPage(1);
  }, []);

  const handleSort = useCallback(
    (column: string) => {
      if (sortBy === column) {
        setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
      } else {
        setSortBy(column);
        setSortOrder("desc");
      }
      setPage(1);
    },
    [sortBy],
  );

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    sort_by: sortBy,
    sort_order: sortOrder,
    ...(statusFilter !== "all" ? { status: statusFilter } : {}),
  };

  const { data, isLoading, isError } = useScans(queryParams);

  const scans = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 0;

  const { containerProps, isFocused, setFocusedIndex } = useTableKeyNav({
    items: scans,
    onActivate: (scan) => setSelectedScanId(scan.id ?? null),
    onEscape: () => setSelectedScanId(null),
  });

  const visibleColCount = SCAN_COLUMNS.filter(
    (c) => colVis[c.key] !== false,
  ).length;

  // Reset keyboard focus when page/filters change
  useEffect(() => {
    setFocusedIndex(-1);
  }, [page, statusFilter, sortBy, sortOrder, setFocusedIndex]);

  // Clamp page back when a filter change reduces total_pages below current page.
  // Must be in useEffect — calling setPage during render causes an infinite loop.
  useEffect(() => {
    if (!isLoading && totalPages > 0 && page > totalPages) {
      setPage(totalPages);
    }
  }, [isLoading, page, totalPages, setPage]);

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

          <div className="flex items-center gap-2 sm:ml-auto">
            <Button
              onClick={() => setShowRunScan(true)}
              icon={<ScanLine className="h-3.5 w-3.5" />}
            >
              New scan
            </Button>
            <ColumnToggle
              columns={SCAN_COLUMNS}
              visibility={colVis}
              onToggle={toggleCol}
            />
          </div>
        </div>

        {/* Table card */}
        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          {/* Keyboard-navigable container */}
          <div
            {...containerProps}
            role="region"
            className="focus:outline-none"
            aria-label="Scans table"
          >
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-border bg-surface">
                    <th className="text-left font-medium text-text-muted px-4 py-3 pr-4">
                      Targets
                    </th>
                    <SortHeader
                      label="Status"
                      column="status"
                      sortBy={sortBy}
                      sortOrder={sortOrder}
                      onSort={handleSort}
                    />
                    {colVis.ports && (
                      <th className="text-left font-medium text-text-muted py-3 pr-4">
                        Ports
                      </th>
                    )}
                    {colVis.duration && (
                      <th className="text-left font-medium text-text-muted py-3 pr-4">
                        Duration
                      </th>
                    )}
                    {colVis.started && (
                      <SortHeader
                        label="Started"
                        column="started_at"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                  </tr>
                </thead>
                <tbody>
                  {isError ? (
                    <tr>
                      <td
                        colSpan={visibleColCount}
                        className="py-10 text-center text-xs text-danger"
                      >
                        Failed to load scans.
                      </td>
                    </tr>
                  ) : isLoading ? (
                    <SkeletonRows count={8} colVis={colVis} />
                  ) : scans.length === 0 ? (
                    <tr>
                      <td
                        colSpan={visibleColCount}
                        className="py-10 text-center text-xs text-text-muted"
                      >
                        No scans found.
                      </td>
                    </tr>
                  ) : (
                    scans.map((scan, idx) => (
                      <tr
                        key={scan.id}
                        onClick={() => {
                          setSelectedScanId(scan.id ?? null);
                          setFocusedIndex(idx);
                        }}
                        className={cn(
                          "border-b border-border/50 last:border-0",
                          "hover:bg-surface-raised/50 transition-colors cursor-pointer",
                          isFocused(idx) &&
                            "ring-1 ring-inset ring-accent/60 bg-surface-raised/40",
                        )}
                      >
                        <td className="py-3 px-4 pr-4 font-mono text-text-secondary max-w-50 truncate">
                          {scan.targets?.join(", ") ?? "—"}
                        </td>
                        <td className="py-3 pr-4">
                          <StatusBadge status={scan.status ?? "unknown"} />
                        </td>
                        {colVis.ports && (
                          <td className="py-3 pr-4 text-text-secondary tabular-nums">
                            {scan.ports_scanned ?? "—"}
                          </td>
                        )}
                        {colVis.duration && (
                          <td className="py-3 pr-4 text-text-muted">
                            {scan.duration ?? "—"}
                          </td>
                        )}
                        {colVis.started && (
                          <td className="py-3 text-text-muted whitespace-nowrap">
                            {scan.started_at
                              ? formatRelativeTime(scan.started_at)
                              : "—"}
                          </td>
                        )}
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
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
