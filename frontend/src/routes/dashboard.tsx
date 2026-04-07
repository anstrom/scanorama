import { useState } from "react";
import { useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { useRecentScans } from "../api/hooks/use-scans";
import { useActiveHostCount } from "../api/hooks/use-hosts";
import { useDiscoveryJobs, useDiscoveryDiff } from "../api/hooks/use-discovery";
import { StatCard } from "../components/stat-card";
import { SystemInfoCard } from "../components/system-info-card";
import { RecentScansTable } from "../components/recent-scans-table";
import { ScanActivityChart } from "../components/scan-activity-chart";
import { ScanDetailPanel, Skeleton } from "../components";
import { Network, Server, MonitorCheck, ShieldOff } from "lucide-react";
import { formatRelativeTime } from "../lib/utils";
import type { components } from "../api/types";

type ScanResponse = components["schemas"]["docs.ScanResponse"];

// ── Recent Discovery Changes widget ───────────────────────────────────────────

function RecentDiscoveryChanges() {
  const { data: jobsData, isLoading: jobsLoading } = useDiscoveryJobs({
    page: 1,
    page_size: 1,
    status: "completed",
  });

  const jobs = jobsData?.data ?? [];
  const latestJob = jobs[0] ?? null;

  const { data: diff, isLoading: diffLoading } = useDiscoveryDiff(
    latestJob?.id ?? "",
    !!latestJob,
  );

  const isLoading = jobsLoading || (!!latestJob && diffLoading);

  if (isLoading) {
    return (
      <div className="mt-6 bg-surface rounded-lg border border-border p-4">
        <p className="text-xs font-medium text-text-primary mb-3">
          Recent Discovery Changes
        </p>
        <div className="space-y-2">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
      </div>
    );
  }

  if (!latestJob || !diff) {
    return (
      <div className="mt-6 bg-surface rounded-lg border border-border p-4">
        <p className="text-xs font-medium text-text-primary mb-3">
          Recent Discovery Changes
        </p>
        <p className="text-xs text-text-muted">No discovery runs yet.</p>
      </div>
    );
  }

  return (
    <div className="mt-6 bg-surface rounded-lg border border-border p-4">
      <p className="text-xs font-medium text-text-primary mb-3">
        Recent Discovery Changes
      </p>
      <p className="text-xs text-text-muted mb-3">
        Last run: {latestJob.networks?.join(", ") ?? "—"}
        {latestJob.started_at
          ? ` · ${formatRelativeTime(latestJob.started_at)}`
          : ""}
      </p>
      <div className="flex items-center gap-4 text-xs flex-wrap">
        <span className="text-success">● {diff.new_hosts.length} new</span>
        <span className="text-danger">● {diff.gone_hosts.length} gone</span>
        <span
          className={
            diff.changed_hosts.length > 0 ? "text-warning" : "text-text-muted"
          }
        >
          ○ {diff.changed_hosts.length} changed
        </span>
        <span className="text-text-muted">
          {diff.unchanged_count} unchanged
        </span>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function DashboardPage() {
  const { data: version, isLoading: versionLoading } = useVersion();
  const { data: stats, isLoading: statsLoading } = useNetworkStats();
  const { data: recentScans, isLoading: scansLoading } = useRecentScans();
  const { data: activeHostCount, isLoading: activeHostsLoading } =
    useActiveHostCount();

  const [selectedScan, setSelectedScan] = useState<ScanResponse | null>(null);

  return (
    <>
      {/* System status */}
      <div className="mb-6">
        <div className="mb-3">
          <SystemInfoCard version={version} loading={versionLoading} />
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <StatCard
            label="Networks"
            value={(stats?.networks?.total as number) ?? "—"}
            icon={Network}
            loading={statsLoading}
            href="/networks"
          />
          <StatCard
            label="Hosts"
            value={(stats?.hosts?.total as number) ?? "—"}
            icon={Server}
            loading={statsLoading}
            href="/hosts"
          />
          <StatCard
            label="Active Hosts"
            value={activeHostCount ?? "—"}
            icon={MonitorCheck}
            loading={activeHostsLoading}
            href="/hosts"
          />
          <StatCard
            label="Exclusions"
            value={(stats?.exclusions?.total as number) ?? "—"}
            icon={ShieldOff}
            loading={statsLoading}
          />
        </div>
      </div>

      {/* Scan activity chart */}
      <div className="mb-6">
        <ScanActivityChart />
      </div>

      {/* Recent scans */}
      <RecentScansTable
        scans={recentScans?.data}
        loading={scansLoading}
        onScanClick={(scan) => setSelectedScan(scan as ScanResponse)}
      />

      {/* Recent discovery changes */}
      <RecentDiscoveryChanges />

      {/* Scan detail panel */}
      {selectedScan && (
        <ScanDetailPanel
          scan={selectedScan}
          onClose={() => setSelectedScan(null)}
        />
      )}
    </>
  );
}
