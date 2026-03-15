import { GitCommit, Clock, Package } from "lucide-react";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type VersionResponse = components["schemas"]["docs.VersionResponse"];

interface SystemInfoCardProps {
  version?: VersionResponse;
  loading?: boolean;
}

function formatBuildTime(buildTime?: string): string {
  if (!buildTime || buildTime === "unknown") return "—";
  try {
    return new Date(buildTime).toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      timeZoneName: "short",
    });
  } catch {
    return buildTime;
  }
}

export function SystemInfoCard({
  version,
  loading = false,
}: SystemInfoCardProps) {
  const isDev = !version?.version || version.version === "dev";

  if (loading) {
    return (
      <div className="bg-surface rounded-lg border border-border p-4">
        <div className="animate-pulse space-y-3">
          <div className="h-3 w-16 rounded bg-surface-raised" />
          <div className="h-px bg-surface-raised" />
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-1.5">
              <div className="h-2.5 w-12 rounded bg-surface-raised" />
              <div className="h-3 w-20 rounded bg-surface-raised" />
            </div>
            <div className="space-y-1.5">
              <div className="h-2.5 w-12 rounded bg-surface-raised" />
              <div className="h-3 w-16 rounded bg-surface-raised" />
            </div>
            <div className="space-y-1.5">
              <div className="h-2.5 w-12 rounded bg-surface-raised" />
              <div className="h-3 w-28 rounded bg-surface-raised" />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs uppercase tracking-wide text-text-muted">
          Build info
        </span>
        {isDev && (
          <span className="text-xs px-1.5 py-0.5 rounded-full font-medium bg-warning/10 text-warning">
            dev build
          </span>
        )}
      </div>

      {/* Divider */}
      <div className="h-px bg-border mb-3" />

      {/* Build info grid */}
      <div className="grid grid-cols-3 gap-x-4 gap-y-3">
        {/* Version */}
        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <Package className="h-3 w-3 shrink-0" />
            <span className="text-xs uppercase tracking-wide">Version</span>
          </div>
          <span className="text-sm font-mono text-text-primary truncate">
            {version?.version ?? "—"}
          </span>
        </div>

        {/* Commit */}
        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <GitCommit className="h-3 w-3 shrink-0" />
            <span className="text-xs uppercase tracking-wide">Commit</span>
          </div>
          <span
            className={cn(
              "text-sm font-mono truncate",
              version?.commit && version.commit !== "none"
                ? "text-text-primary"
                : "text-text-muted",
            )}
          >
            {version?.commit && version.commit !== "none"
              ? version.commit
              : "—"}
          </span>
        </div>

        {/* Built */}
        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <Clock className="h-3 w-3 shrink-0" />
            <span className="text-xs uppercase tracking-wide">Built</span>
          </div>
          <span
            className={cn(
              "text-sm truncate",
              version?.build_time && version.build_time !== "unknown"
                ? "text-text-secondary"
                : "text-text-muted",
            )}
          >
            {formatBuildTime(version?.build_time)}
          </span>
        </div>
      </div>
    </div>
  );
}
