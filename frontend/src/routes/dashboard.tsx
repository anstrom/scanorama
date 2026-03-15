import { useState } from "react";
import { useHealth, useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { useRecentScans } from "../api/hooks/use-scans";
import { useActiveHostCount } from "../api/hooks/use-hosts";
import { StatusBadge } from "../components/status-badge";
import { StatCard } from "../components/stat-card";
import { RecentScansTable } from "../components/recent-scans-table";
import { ScanDetailPanel } from "./scans";
import { Network, Server, MonitorCheck, ShieldOff } from "lucide-react";
import type { components } from "../api/types";

type ScanResponse = components["schemas"]["docs.ScanResponse"];

export function DashboardPage() {
  const { data: health, isLoading: healthLoading } = useHealth();
  const { data: version } = useVersion();
  const { data: stats, isLoading: statsLoading } = useNetworkStats();
  const { data: recentScans, isLoading: scansLoading } = useRecentScans();
  const { data: activeHostCount, isLoading: activeHostsLoading } =
    useActiveHostCount();

  const [selectedScan, setSelectedScan] = useState<ScanResponse | null>(null);

  return (
    <>
      {/* System status */}
      <div className="mb-6">
        <div className="flex items-center gap-3 mb-4">
          <h2 className="text-lg font-medium text-text-primary">System</h2>
          {healthLoading ? (
            <span className="text-xs text-text-muted">Checking...</span>
          ) : health ? (
            <StatusBadge status={health.status ?? "unknown"} />
          ) : (
            <StatusBadge status="error" />
          )}
          {version && (
            <span className="text-xs font-mono text-text-muted">
              {version.version}
            </span>
          )}
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

      {/* Recent scans */}
      <RecentScansTable
        scans={recentScans?.data}
        loading={scansLoading}
        onScanClick={(scan) => setSelectedScan(scan as ScanResponse)}
      />

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
