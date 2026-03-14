import { RootLayout } from "../components/layout/root-layout";
import { useHealth, useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { StatusBadge } from "../components/status-badge";

function StatCard({
  label,
  value,
  subtext,
}: {
  label: string;
  value: string | number;
  subtext?: string;
}) {
  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <p className="text-xs text-text-muted mb-1">{label}</p>
      <p className="text-2xl font-semibold text-text-primary">{value}</p>
      {subtext && <p className="text-xs text-text-secondary mt-1">{subtext}</p>}
    </div>
  );
}

export function DashboardPage() {
  const { data: health, isLoading: healthLoading } = useHealth();
  const { data: version } = useVersion();
  const { data: stats } = useNetworkStats();

  return (
    <RootLayout title="Dashboard">
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
          />
          <StatCard
            label="Hosts"
            value={(stats?.hosts?.total as number) ?? "—"}
          />
          <StatCard
            label="Active Hosts"
            value={(stats?.hosts?.active as number) ?? "—"}
          />
          <StatCard
            label="Exclusions"
            value={(stats?.exclusions?.total as number) ?? "—"}
          />
        </div>
      </div>

      {/* Placeholder for recent scans - will be built in iteration 1 */}
      <div className="bg-surface rounded-lg border border-border p-4">
        <h2 className="text-sm font-medium text-text-primary mb-2">
          Recent Scans
        </h2>
        <p className="text-xs text-text-muted">Coming in the next iteration.</p>
      </div>
    </RootLayout>
  );
}
