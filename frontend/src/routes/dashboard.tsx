import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { useRecentScans } from "../api/hooks/use-scans";
import { useActiveHostCount } from "../api/hooks/use-hosts";
import { useDiscoveryJobs, useDiscoveryDiff } from "../api/hooks/use-discovery";
import { useStatsSummary } from "../api/hooks/use-dashboard";
import { useExpiringCerts } from "../api/hooks/use-expiring-certs";
import type { ExpiringCertificate } from "../api/hooks/use-expiring-certs";
import {
  useProfileRecommendations,
  useTriggerSmartScanBatch,
} from "../api/hooks/use-smart-scan";
import { StatCard } from "../components/stat-card";
import { SystemInfoCard } from "../components/system-info-card";
import { RecentScansTable } from "../components/recent-scans-table";
import { ScanActivityChart } from "../components/scan-activity-chart";
import { ScanDetailPanel, Skeleton } from "../components";
import { ActivityFeed } from "../components/activity-feed";
import {
  Network,
  Server,
  MonitorCheck,
  ShieldOff,
  ScanLine,
  ScanSearch,
  AlertCircle,
  Clock,
  ShieldAlert,
  Layers,
} from "lucide-react";
import { Button } from "../components/button";
import { RunScanModal } from "../components";
import { serializeFilter } from "../lib/filter-expr";
import { formatRelativeTime, cn } from "../lib/utils";
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

// ── Status donut ──────────────────────────────────────────────────────────────

const STATUS_COLORS: Record<string, string> = {
  up: "#22c55e",
  down: "#ef4444",
  unknown: "#94a3b8",
  gone: "#64748b",
};

function StatusBreakdown({
  data,
  loading,
}: {
  data: Record<string, number> | undefined;
  loading: boolean;
}) {
  const entries = data
    ? Object.entries(data).sort(([a], [b]) => a.localeCompare(b))
    : [];
  const total = entries.reduce((s, [, v]) => s + v, 0);

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <p className="text-xs font-medium text-text-primary mb-3">
        Host Status
      </p>
      {loading ? (
        <div className="space-y-2">
          {[3, 2, 2, 1].map((w, i) => (
            <Skeleton key={i} className={`h-3 w-${w * 10}`} />
          ))}
        </div>
      ) : entries.length === 0 ? (
        <p className="text-xs text-text-muted">No hosts.</p>
      ) : (
        <div className="space-y-2">
          {entries.map(([status, count]) => {
            const pct = total > 0 ? Math.round((count / total) * 100) : 0;
            const color = STATUS_COLORS[status] ?? "#94a3b8";
            return (
              <div key={status} className="flex items-center gap-2">
                <span
                  className="h-2 w-2 rounded-full shrink-0"
                  style={{ backgroundColor: color }}
                />
                <span className="text-xs text-text-secondary capitalize flex-1">
                  {status}
                </span>
                <span className="text-xs font-medium text-text-primary tabular-nums">
                  {count}
                </span>
                <div className="w-20 h-1.5 rounded-full bg-surface-raised overflow-hidden">
                  <div
                    className="h-full rounded-full transition-all"
                    style={{ width: `${pct}%`, backgroundColor: color }}
                  />
                </div>
              </div>
            );
          })}
          <p className="text-[10px] text-text-muted pt-1">
            {total} total hosts
          </p>
        </div>
      )}
    </div>
  );
}

// ── OS Family distribution ────────────────────────────────────────────────────

function OSDistribution({
  data,
  loading,
}: {
  data: Array<{ family: string; count: number }> | undefined;
  loading: boolean;
}) {
  const top = data?.slice(0, 6) ?? [];
  const maxCount = top[0]?.count ?? 1;

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <p className="text-xs font-medium text-text-primary mb-3">
        OS Distribution
      </p>
      {loading ? (
        <div className="space-y-2">
          {[4, 3, 2, 2, 1].map((w, i) => (
            <Skeleton key={i} className={`h-3 w-${w * 10}`} />
          ))}
        </div>
      ) : top.length === 0 ? (
        <p className="text-xs text-text-muted">No OS data yet.</p>
      ) : (
        <div className="space-y-2">
          {top.map(({ family, count }) => {
            const pct = Math.round((count / maxCount) * 100);
            return (
              <div key={family} className="flex items-center gap-2">
                <span className="text-xs text-text-secondary truncate flex-1 min-w-0">
                  {family}
                </span>
                <span className="text-xs font-medium text-text-primary tabular-nums shrink-0">
                  {count}
                </span>
                <div className="w-20 h-1.5 rounded-full bg-surface-raised overflow-hidden shrink-0">
                  <div
                    className="h-full rounded-full bg-accent/60 transition-all"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ── Top ports ─────────────────────────────────────────────────────────────────

function TopPorts({
  data,
  loading,
}: {
  data: Array<{ port: number; count: number }> | undefined;
  loading: boolean;
}) {
  const maxCount = data?.[0]?.count ?? 1;

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <p className="text-xs font-medium text-text-primary mb-3">
        Top Open Ports
      </p>
      {loading ? (
        <div className="space-y-2">
          {[3, 2, 2, 2, 1].map((w, i) => (
            <Skeleton key={i} className={`h-3 w-${w * 10}`} />
          ))}
        </div>
      ) : !data || data.length === 0 ? (
        <p className="text-xs text-text-muted">No port data yet.</p>
      ) : (
        <div className="space-y-2">
          {data.map(({ port, count }) => {
            const pct = Math.round((count / maxCount) * 100);
            return (
              <div key={port} className="flex items-center gap-2">
                <span className="text-xs font-mono text-text-secondary shrink-0 w-10 text-right">
                  {port}
                </span>
                <div className="flex-1 h-1.5 rounded-full bg-surface-raised overflow-hidden">
                  <div
                    className="h-full rounded-full bg-accent/70 transition-all"
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <span className="text-xs text-text-muted tabular-nums shrink-0 w-8 text-right">
                  {count}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ── Quick actions row ─────────────────────────────────────────────────────────

function QuickActions({
  onQuickScan,
  staleCount,
  loading,
}: {
  onQuickScan: () => void;
  staleCount: number;
  loading: boolean;
}) {
  const navigate = useNavigate();

  function viewNewHosts() {
    const since = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString().split("T")[0]!;
    const filter = serializeFilter({
      op: "AND",
      conditions: [{ field: "first_seen", cmp: "gt", value: since }],
    });
    void navigate({ to: "/hosts", search: { filter }, replace: false });
  }

  function viewStaleHosts() {
    const cutoff = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString().split("T")[0]!;
    const filter = serializeFilter({
      op: "AND",
      conditions: [{ field: "last_seen", cmp: "lt", value: cutoff }],
    });
    void navigate({ to: "/hosts", search: { filter }, replace: false });
  }

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <p className="text-xs font-medium text-text-primary mb-3">
        Quick Actions
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          icon={<ScanLine className="h-3.5 w-3.5" />}
          onClick={onQuickScan}
          className="text-xs h-7 px-3"
        >
          Quick Scan
        </Button>
        <Button
          variant="secondary"
          icon={<ScanSearch className="h-3.5 w-3.5" />}
          onClick={() => void navigate({ to: "/discovery" })}
          className="text-xs h-7 px-3"
        >
          Run Discovery
        </Button>
        <Button
          variant="secondary"
          icon={<Server className="h-3.5 w-3.5" />}
          onClick={viewNewHosts}
          className="text-xs h-7 px-3"
        >
          New Hosts
        </Button>
        {(loading || staleCount > 0) && (
          <button
            type="button"
            onClick={viewStaleHosts}
            className={cn(
              "flex items-center gap-1.5 text-xs px-3 h-7 rounded border transition-colors",
              staleCount > 0
                ? "border-warning/50 text-warning bg-warning/10 hover:bg-warning/15"
                : "border-border text-text-muted",
            )}
          >
            <AlertCircle className="h-3.5 w-3.5" />
            {loading ? (
              <Skeleton className="h-3 w-16 inline-block" />
            ) : (
              `${staleCount} stale host${staleCount !== 1 ? "s" : ""}`
            )}
          </button>
        )}
      </div>
    </div>
  );
}

// ── Avg scan duration widget ──────────────────────────────────────────────────

function AvgDurationCard({
  seconds,
  loading,
}: {
  seconds: number;
  loading: boolean;
}) {
  function format(s: number): string {
    if (s === 0) return "—";
    if (s < 60) return `${Math.round(s)}s`;
    const m = Math.floor(s / 60);
    const rem = Math.round(s % 60);
    return rem > 0 ? `${m}m ${rem}s` : `${m}m`;
  }

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <div className="flex items-center gap-2 mb-1">
        <Clock className="h-3.5 w-3.5 text-text-muted" />
        <p className="text-xs font-medium text-text-primary">Avg Scan Time</p>
      </div>
      <p className="text-xs text-text-muted mb-1">Last 30 days</p>
      {loading ? (
        <Skeleton className="h-6 w-16" />
      ) : (
        <p className="text-lg font-mono font-semibold text-text-primary">
          {format(seconds)}
        </p>
      )}
    </div>
  );
}

// ── Expiring TLS certificates widget ──────────────────────────────────────────

function certUrgencyClass(daysLeft: number): string {
  if (daysLeft <= 7) return "text-danger";
  if (daysLeft <= 14) return "text-[#f97316]"; // orange-500
  return "text-warning";
}

function ExpiringCertsWidget() {
  const { data, isLoading } = useExpiringCerts(30);
  const navigate = useNavigate();

  // Render nothing when there are no expiring certs and we're done loading.
  if (!isLoading && (!data || data.certificates.length === 0)) return null;

  function viewHost(cert: ExpiringCertificate) {
    const filter = serializeFilter({
      op: "AND",
      conditions: [{ field: "hostname", cmp: "contains", value: cert.host_ip }],
    });
    void navigate({ to: "/hosts", search: { filter }, replace: false });
  }

  const certs = data?.certificates ?? [];
  const criticalCount = certs.filter((c) => c.days_left <= 7).length;
  const warningCount = certs.filter(
    (c) => c.days_left > 7 && c.days_left <= 14,
  ).length;

  return (
    <div className="mt-6 bg-surface rounded-lg border border-border p-4">
      <div className="flex items-center gap-2 mb-3">
        <ShieldAlert
          className={cn(
            "h-3.5 w-3.5 shrink-0",
            criticalCount > 0
              ? "text-danger"
              : warningCount > 0
                ? "text-[#f97316]"
                : "text-warning",
          )}
        />
        <p className="text-xs font-medium text-text-primary">
          Expiring TLS Certificates
        </p>
        {!isLoading && (
          <span
            className={cn(
              "ml-auto text-xs font-semibold tabular-nums px-1.5 py-0.5 rounded",
              criticalCount > 0
                ? "bg-danger/15 text-danger"
                : "bg-warning/15 text-warning",
            )}
          >
            {certs.length}
          </span>
        )}
      </div>
      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
        </div>
      ) : (
        <div className="space-y-1.5">
          {certs.slice(0, 6).map((cert) => (
            <button
              key={`${cert.host_id}-${cert.port}`}
              type="button"
              onClick={() => viewHost(cert)}
              className="w-full flex items-center gap-2 text-left hover:bg-surface-raised rounded px-1 py-0.5 transition-colors"
            >
              <span
                className={cn(
                  "text-xs font-mono shrink-0 w-8 text-right",
                  certUrgencyClass(cert.days_left),
                )}
              >
                {cert.days_left}d
              </span>
              <span className="text-xs text-text-secondary font-mono shrink-0">
                {cert.host_ip}
              </span>
              {cert.hostname && (
                <span className="text-xs text-text-muted truncate">
                  {cert.hostname}
                </span>
              )}
              <span className="text-xs text-text-muted shrink-0 ml-auto font-mono">
                :{cert.port}
              </span>
              {cert.subject_cn && (
                <span className="text-xs text-text-muted truncate max-w-[120px]">
                  {cert.subject_cn}
                </span>
              )}
            </button>
          ))}
          {certs.length > 6 && (
            <p className="text-[10px] text-text-muted pt-1">
              +{certs.length - 6} more expiring within 30 days
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// ── Profile recommendations widget ───────────────────────────────────────────

function ProfileRecommendationsWidget() {
  const { data: recs, isLoading } = useProfileRecommendations();
  const triggerBatch = useTriggerSmartScanBatch();

  if (!isLoading && (!recs || recs.length === 0)) return null;

  function queueForFamily(osFamily: string) {
    triggerBatch.mutate({ os_family: osFamily, stage: "port_expansion" });
  }

  return (
    <div className="mt-6 bg-surface rounded-lg border border-border p-4">
      <div className="flex items-center gap-2 mb-3">
        <Layers className="h-3.5 w-3.5 shrink-0 text-accent" />
        <p className="text-xs font-medium text-text-primary">
          Profile Recommendations
        </p>
        {!isLoading && recs && (
          <span className="ml-auto text-xs font-semibold tabular-nums px-1.5 py-0.5 rounded bg-accent/15 text-accent">
            {recs.length}
          </span>
        )}
      </div>
      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
        </div>
      ) : (
        <div className="space-y-1.5">
          {(recs ?? []).map((rec) => {
            const family = rec.os_family ?? "";
            return (
              <div
                key={family}
                className="flex items-center gap-2 px-1 py-0.5"
              >
                <span className="text-xs font-mono text-text-secondary shrink-0 w-6 text-right tabular-nums">
                  {rec.host_count ?? 0}
                </span>
                <span className="text-xs text-text-secondary capitalize flex-1 truncate">
                  {family}
                </span>
                <span className="text-xs text-text-muted truncate max-w-[140px]">
                  {rec.profile_name ?? "—"}
                </span>
                <button
                  type="button"
                  onClick={() => queueForFamily(family)}
                  disabled={triggerBatch.isPending || !family}
                  className="shrink-0 flex items-center gap-1 text-[10px] px-2 h-5 rounded border border-accent/40 text-accent bg-accent/10 hover:bg-accent/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <ScanLine className="h-2.5 w-2.5" />
                  Queue Scan
                </button>
              </div>
            );
          })}
          <p className="text-[10px] text-text-muted pt-1">
            Hosts grouped by OS family — queue a port expansion scan for each.
          </p>
        </div>
      )}
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
  const { data: summary, isLoading: summaryLoading } = useStatsSummary();

  const [selectedScan, setSelectedScan] = useState<ScanResponse | null>(null);
  const [showScanModal, setShowScanModal] = useState(false);

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

      {/* Quick actions + stale hosts */}
      <div className="mb-6">
        <QuickActions
          onQuickScan={() => setShowScanModal(true)}
          staleCount={summary?.stale_host_count ?? 0}
          loading={summaryLoading}
        />
      </div>

      {/* Rich stats row */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <StatusBreakdown
          data={summary?.hosts_by_status}
          loading={summaryLoading}
        />
        <OSDistribution
          data={summary?.hosts_by_os_family}
          loading={summaryLoading}
        />
        <div className="flex flex-col gap-4">
          <TopPorts data={summary?.top_ports} loading={summaryLoading} />
          <AvgDurationCard
            seconds={summary?.avg_scan_duration_s ?? 0}
            loading={summaryLoading}
          />
        </div>
      </div>

      {/* Scan activity chart */}
      <div className="mb-6">
        <ScanActivityChart />
      </div>

      {/* Bottom row: recent scans + activity feed */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
        <div className="lg:col-span-2">
          <RecentScansTable
            scans={recentScans?.data}
            loading={scansLoading}
            onScanClick={(scan) => setSelectedScan(scan as ScanResponse)}
          />
        </div>
        <div>
          <ActivityFeed />
        </div>
      </div>

      {/* Recent discovery changes */}
      <RecentDiscoveryChanges />

      {/* Expiring TLS certificates */}
      <ExpiringCertsWidget />

      {/* Profile recommendations */}
      <ProfileRecommendationsWidget />

      {/* Modals / panels */}
      {selectedScan && (
        <ScanDetailPanel
          scan={selectedScan}
          onClose={() => setSelectedScan(null)}
        />
      )}
      {showScanModal && (
        <RunScanModal onClose={() => setShowScanModal(false)} />
      )}
    </>
  );
}
