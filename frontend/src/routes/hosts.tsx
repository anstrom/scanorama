import { useState, useEffect, useCallback } from "react";
import { Search, ScanLine } from "lucide-react";
import { Button } from "../components/button";
import { useHosts } from "../api/hooks/use-hosts";
import {
  StatusBadge,
  Skeleton,
  PaginationBar,
  RunScanModal,
} from "../components";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";

const PAGE_SIZE = 25;

type HostStatus = "all" | "up" | "down" | "unknown";

function PortTags({ ports }: { ports?: number[] }) {
  if (!ports || ports.length === 0)
    return <span className="text-text-muted">—</span>;

  const MAX_SHOWN = 5;
  const shown = ports.slice(0, MAX_SHOWN);
  const overflow = ports.length - MAX_SHOWN;

  return (
    <div className="flex flex-wrap gap-1">
      {shown.map((port) => (
        <span
          key={port}
          className="inline-block px-1.5 py-0.5 rounded bg-surface-raised text-xs font-mono text-text-secondary border border-border"
        >
          {port}
        </span>
      ))}
      {overflow > 0 && (
        <span className="inline-block px-1.5 py-0.5 rounded bg-surface-raised text-xs text-text-muted border border-border">
          +{overflow} more
        </span>
      )}
    </div>
  );
}

function SkeletonRows({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="border-b border-border/50">
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-28" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-36" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-5 w-14" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-32" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-24" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-20" />
          </td>
          <td className="py-3">
            <Skeleton className="h-3.5 w-8" />
          </td>
        </tr>
      ))}
    </>
  );
}

export function HostsPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<HostStatus>("all");
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [scanIP, setScanIP] = useState<string | null>(null);

  // Debounce search input ~300ms
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Reset page when filter changes
  const handleStatusChange = useCallback((value: HostStatus) => {
    setStatusFilter(value);
    setPage(1);
  }, []);

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    ...(statusFilter !== "all" ? { status: statusFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  };

  const { data, isLoading } = useHosts(queryParams);

  const hosts = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  return (
    <>
      <div className="space-y-4">
        {/* Filter bar */}
        <div className="flex flex-col sm:flex-row gap-3">
          {/* Search input */}
          <div className="relative flex-1 max-w-sm">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-text-muted pointer-events-none" />
            <input
              type="text"
              placeholder="Search by IP or hostname…"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className={cn(
                "w-full pl-8 pr-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border focus:border-border",
              )}
              aria-label="Search hosts"
            />
          </div>

          {/* Status select */}
          <select
            value={statusFilter}
            onChange={(e) => handleStatusChange(e.target.value as HostStatus)}
            className={cn(
              "px-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
            aria-label="Filter by status"
          >
            <option value="all">All statuses</option>
            <option value="up">Up</option>
            <option value="down">Down</option>
            <option value="unknown">Unknown</option>
          </select>

          <Button
            onClick={() => setScanIP("")}
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
                    IP Address
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Hostname
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Status
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    MAC Address
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Open Ports
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Last Seen
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Scans
                  </th>
                  <th className="py-3" />
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <SkeletonRows count={8} />
                ) : hosts.length === 0 ? (
                  <tr>
                    <td
                      colSpan={7}
                      className="py-10 text-center text-xs text-text-muted"
                    >
                      No hosts found.
                    </td>
                  </tr>
                ) : (
                  hosts.map((host) => (
                    <tr
                      key={host.id}
                      className="border-b border-border/50 last:border-0 hover:bg-surface-raised/50 transition-colors"
                    >
                      <td className="py-3 px-4 pr-4 font-mono text-text-primary whitespace-nowrap">
                        {host.ip_address ?? "—"}
                      </td>
                      <td className="py-3 pr-4 text-text-secondary">
                        {host.hostname ?? "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <StatusBadge status={host.status ?? "unknown"} />
                      </td>
                      <td className="py-3 pr-4 font-mono text-text-muted whitespace-nowrap">
                        {host.mac_address ?? "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <PortTags ports={host.open_ports} />
                      </td>
                      <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                        {host.last_seen
                          ? formatRelativeTime(host.last_seen)
                          : "—"}
                      </td>
                      <td className="py-3 pr-4 text-text-secondary tabular-nums">
                        {host.scan_count ?? "—"}
                      </td>
                      <td className="py-3">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            setScanIP(host.ip_address ?? "");
                          }}
                          aria-label={`Scan ${host.ip_address ?? "host"}`}
                          className={cn(
                            "flex items-center gap-1 px-2 py-1 rounded text-xs",
                            "text-text-muted border border-border",
                            "hover:text-accent hover:border-accent hover:bg-accent/5",
                            "transition-colors",
                          )}
                        >
                          <ScanLine className="h-3 w-3" />
                          Scan
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {!isLoading && hosts.length > 0 && (
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

      {scanIP !== null && (
        <RunScanModal initialTarget={scanIP} onClose={() => setScanIP(null)} />
      )}
    </>
  );
}
