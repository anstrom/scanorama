import { Link } from "@tanstack/react-router";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import { StatusBadge } from "./status-badge";
import { Skeleton } from "./skeleton";

interface Scan {
  id?: string;
  status?: string;
  targets?: string[];
  hosts_discovered?: number;
  ports_scanned?: number;
  created_at?: string;
  duration?: string;
}

interface RecentScansTableProps {
  scans?: Scan[];
  loading?: boolean;
  onScanClick?: (scan: Scan) => void;
}

export function RecentScansTable({
  scans,
  loading = false,
  onScanClick,
}: RecentScansTableProps) {
  if (loading) {
    return (
      <div className="bg-surface rounded-lg border border-border p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-text-primary">
            Recent Scans
          </h2>
          <Link
            to="/scans"
            className="text-xs text-text-muted hover:text-text-primary transition-colors"
          >
            View all →
          </Link>
        </div>
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex items-center gap-3">
              <Skeleton className="h-5 w-16" />
              <Skeleton className="h-4 w-32 flex-1" />
              <Skeleton className="h-4 w-12" />
              <Skeleton className="h-4 w-20" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  const isEmpty = !scans || scans.length === 0;

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-text-primary">Recent Scans</h2>
        <Link
          to="/scans"
          className="text-xs text-text-muted hover:text-text-primary transition-colors"
        >
          View all →
        </Link>
      </div>
      {isEmpty ? (
        <p className="text-xs text-text-muted">No scans found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-text-muted border-b border-border">
                <th className="text-left font-medium pb-2 pr-4">Status</th>
                <th className="text-left font-medium pb-2 pr-4">Targets</th>
                <th className="text-right font-medium pb-2 pr-4">Hosts</th>
                <th className="text-right font-medium pb-2 pr-4">Ports</th>
                <th className="text-right font-medium pb-2">When</th>
              </tr>
            </thead>
            <tbody>
              {scans.map((scan) => (
                <tr
                  key={scan.id}
                  onClick={() => onScanClick?.(scan)}
                  className={cn(
                    "border-b border-border/50 last:border-0 transition-colors",
                    onScanClick && "cursor-pointer hover:bg-surface-raised/60",
                  )}
                >
                  <td className="py-2 pr-4">
                    <StatusBadge status={scan.status ?? "unknown"} />
                  </td>
                  <td className="py-2 pr-4 text-text-secondary font-mono max-w-48 truncate">
                    {scan.targets?.join(", ") ?? "—"}
                  </td>
                  <td className="py-2 pr-4 text-right text-text-secondary tabular-nums">
                    {scan.hosts_discovered ?? "—"}
                  </td>
                  <td className="py-2 pr-4 text-right text-text-secondary tabular-nums">
                    {scan.ports_scanned ?? "—"}
                  </td>
                  <td className="py-2 text-right text-text-muted">
                    {scan.created_at
                      ? formatRelativeTime(scan.created_at)
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
