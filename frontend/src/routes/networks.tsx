import { useState, useCallback, useRef, useMemo } from "react";
import {
  Network,
  Plus,
  Trash2,
  X,
  Pencil,
  Check,
  Ban,
  Play,
  RefreshCw,
  ScanSearch,
  Radar,
  TrendingUp,
  Zap,
} from "lucide-react";
import { SortHeader } from "../components/sort-header";
import type { SortOrder } from "../components/sort-header";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { Button } from "../components/button";
import {
  useNetworks,
  useNetworkExclusions,
  useEnableNetwork,
  useDisableNetwork,
  useDeleteNetwork,
  useDeleteExclusion,
  useStartNetworkDiscovery,
  useNetworkDiscoveryJobs,
} from "../api/hooks/use-networks";
import {
  useDiscoveryJobs,
  useStartDiscovery,
  useStopDiscovery,
  useRerunDiscovery,
} from "../api/hooks/use-discovery";
import { Skeleton, PaginationBar, StatusBadge } from "../components";
import { AddNetworkModal } from "../components/add-network-modal";
import { AddExclusionModal } from "../components/add-exclusion-modal";
import { EditNetworkModal } from "../components/edit-network-modal";
import { CreateDiscoveryModal } from "../components/create-discovery-modal";
import { ScanNetworkModal } from "../components/scan-network-modal";
import { useToast } from "../components/toast-provider";
import {
  useSmartScanSuggestions,
  useTriggerSmartScanBatch,
} from "../api/hooks/use-smart-scan";
import { BatchSmartScanPreviewModal } from "../components/smart-scan-preview-modal";
import { formatRelativeTime, formatAbsoluteTime, cn } from "../lib/utils";
import type { components } from "../api/types";

type NetworkResponse = components["schemas"]["docs.NetworkResponse"];
type NetworkExclusionResponse =
  components["schemas"]["docs.NetworkExclusionResponse"];
type DiscoveryJobResponse = components["schemas"]["docs.DiscoveryJobResponse"];

const PAGE_SIZE = 25;
const DISCOVERY_PAGE_SIZE = 10;

// ── Helpers ───────────────────────────────────────────────────────────────────

const DISCOVERY_METHOD_LABELS: Record<string, string> = {
  ping: "Ping",
  tcp: "TCP",
  tcp_connect: "TCP Connect",
  arp: "ARP",
  icmp: "ICMP",
};

function NetworkActiveBadge({ active }: { active: boolean }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium",
        active
          ? "bg-success/15 text-success"
          : "bg-text-muted/15 text-text-muted",
      )}
    >
      {active ? "active" : "inactive"}
    </span>
  );
}

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function NetworkSkeletonRows() {
  return (
    <>
      {Array.from({ length: 8 }).map((_, i) => (
        <tr key={i} className="border-b border-border">
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-32" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-28 font-mono" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-10" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-10" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-14" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-5 w-14 rounded" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
        </tr>
      ))}
    </>
  );
}

function DiscoverySkeletonRows({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="border-b border-border/50">
          <td className="py-3 px-4 pr-4">
            <Skeleton className="h-3 w-32" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-28" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-12" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-16" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-14" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-1 w-20" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-16" />
          </td>
          <td className="py-3" />
        </tr>
      ))}
    </>
  );
}

// ── Meta row helper ───────────────────────────────────────────────────────────

function MetaRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-xs">
      <span className="text-text-muted w-32 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value ?? "—"}</span>
    </div>
  );
}

// ── Exclusions section (used in detail panel) ─────────────────────────────────

interface ExclusionsSectionProps {
  networkId: string;
}

function ExclusionsSection({ networkId }: ExclusionsSectionProps) {
  const [showAdd, setShowAdd] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const { toast } = useToast();

  const { data: exclusions, isLoading } = useNetworkExclusions(networkId);
  const { mutateAsync: deleteExclusion, isPending: isDeleting } =
    useDeleteExclusion();

  const list: NetworkExclusionResponse[] = exclusions ?? [];

  async function handleDelete(id: string) {
    try {
      await deleteExclusion(id);
      toast.success("Exclusion removed");
      setConfirmDeleteId(null);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to remove exclusion.",
      );
      setConfirmDeleteId(null);
    }
  }

  return (
    <section>
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-xs font-medium text-text-primary">Exclusions</h3>
        <button
          type="button"
          onClick={() => setShowAdd(true)}
          className="flex items-center gap-1 text-xs text-accent hover:text-accent/80 transition-colors"
        >
          <Plus className="h-3 w-3" />
          Add
        </button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {[1, 2].map((i) => (
            <Skeleton key={i} className="h-8 w-full rounded" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <p className="text-xs text-text-muted">No exclusions configured.</p>
      ) : (
        <div className="space-y-1.5">
          {list.map((ex) => (
            <div
              key={ex.id}
              className="flex items-center justify-between gap-2 px-3 py-2 rounded bg-surface-raised border border-border/50"
            >
              <div className="min-w-0">
                <p className="text-xs font-mono text-text-primary truncate">
                  {ex.excluded_cidr}
                </p>
                {ex.reason && (
                  <p className="text-[11px] text-text-muted truncate">
                    {ex.reason}
                  </p>
                )}
              </div>
              {confirmDeleteId === ex.id ? (
                <div className="flex items-center gap-1.5 shrink-0">
                  <button
                    type="button"
                    onClick={() => setConfirmDeleteId(null)}
                    className="text-[11px] text-text-muted hover:text-text-secondary"
                  >
                    Cancel
                  </button>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => void handleDelete(ex.id ?? "")}
                    loading={isDeleting}
                    className="h-5 px-2 text-[11px]"
                  >
                    Delete
                  </Button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmDeleteId(ex.id ?? null)}
                  className="shrink-0 text-text-muted hover:text-danger transition-colors"
                  aria-label="Delete exclusion"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {showAdd && (
        <AddExclusionModal
          networkId={networkId}
          onClose={() => setShowAdd(false)}
        />
      )}
    </section>
  );
}

// ── Host count sparkline ──────────────────────────────────────────────────────

interface HostCountChartProps {
  networkId: string;
}

function HostCountChart({ networkId }: HostCountChartProps) {
  const { data, isLoading } = useNetworkDiscoveryJobs(networkId, {
    page_size: 20,
  });
  const jobs = (data?.data ?? [])
    .filter((j) => j.status === "completed" && j.hosts_found != null)
    .slice()
    .reverse(); // oldest first for the chart

  if (isLoading) {
    return <Skeleton className="h-20 w-full rounded" />;
  }

  if (jobs.length < 2) {
    return (
      <p className="text-xs text-text-muted">
        Not enough discovery history to show a trend.
      </p>
    );
  }

  const chartData = jobs.map((j, idx) => ({
    idx: idx + 1,
    hosts: j.hosts_found ?? 0,
    label: j.started_at ? formatRelativeTime(j.started_at) : `Run ${idx + 1}`,
  }));

  const maxHosts = Math.max(...chartData.map((d) => d.hosts), 1);

  return (
    <div>
      <p className="text-xs text-text-muted mb-2">
        Hosts found per discovery run (last {jobs.length})
      </p>
      <ResponsiveContainer width="100%" height={80}>
        <AreaChart
          data={chartData}
          margin={{ top: 4, right: 4, left: -30, bottom: 0 }}
        >
          <defs>
            <linearGradient id="hostCountGrad" x1="0" y1="0" x2="0" y2="1">
              <stop
                offset="5%"
                stopColor="var(--color-accent, #3b82f6)"
                stopOpacity={0.3}
              />
              <stop
                offset="95%"
                stopColor="var(--color-accent, #3b82f6)"
                stopOpacity={0}
              />
            </linearGradient>
          </defs>
          <YAxis
            domain={[0, maxHosts + 1]}
            allowDecimals={false}
            tick={{ fontSize: 9, fill: "var(--color-text-muted, #6b7280)" }}
            axisLine={false}
            tickLine={false}
          />
          <XAxis dataKey="idx" hide />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--color-surface-raised, #1e293b)",
              border: "1px solid var(--color-border, #334155)",
              borderRadius: 6,
              fontSize: 11,
            }}
            labelFormatter={(_, payload) => payload?.[0]?.payload?.label ?? ""}
            formatter={(value) => [value as number, "Hosts found"]}
          />
          <Area
            type="monotone"
            dataKey="hosts"
            stroke="var(--color-accent, #3b82f6)"
            fill="url(#hostCountGrad)"
            strokeWidth={1.5}
            dot={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

// ── Network discoveries tab ───────────────────────────────────────────────────

interface NetworkDiscoveriesTabProps {
  network: NetworkResponse;
}

function NetworkDiscoveriesTab({ network }: NetworkDiscoveriesTabProps) {
  const [page, setPage] = useState(1);
  const { toast } = useToast();

  const { data, isLoading } = useNetworkDiscoveryJobs(network.id ?? "", {
    page,
    page_size: 10,
  });
  const jobs = data?.data ?? [];
  const totalPages = data?.pagination?.total_pages ?? 1;

  const { mutate: startDiscovery } = useStartNetworkDiscovery();
  const { mutate: rerunDiscovery, isPending: isRerunning } =
    useRerunDiscovery();

  function handleRerun(job: DiscoveryJobResponse) {
    rerunDiscovery(
      {
        networks: job.networks ?? [],
        method: job.method ?? "tcp_connect",
        name: job.name ?? undefined,
      },
      {
        onSuccess: () => toast.success("Discovery restarted"),
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : "Failed to rerun"),
      },
    );
  }

  return (
    <div className="space-y-4">
      {/* Host count trend */}
      {network.id && (
        <section>
          <h3 className="text-xs font-medium text-text-primary mb-2 flex items-center gap-1.5">
            <TrendingUp className="h-3.5 w-3.5 text-text-muted" />
            Host count trend
          </h3>
          <HostCountChart networkId={network.id} />
        </section>
      )}

      {/* Discovery history */}
      <section>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-xs font-medium text-text-primary flex items-center gap-1.5">
            <ScanSearch className="h-3.5 w-3.5 text-text-muted" />
            Discovery history
          </h3>
          <Button
            variant="secondary"
            size="sm"
            className="h-6 px-2 text-[11px]"
            onClick={() => startDiscovery(network.id ?? "")}
          >
            <Play className="h-3 w-3 mr-1" />
            Run new
          </Button>
        </div>

        {isLoading ? (
          <div className="space-y-1.5">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-10 w-full rounded" />
            ))}
          </div>
        ) : jobs.length === 0 ? (
          <p className="text-xs text-text-muted">
            No discovery jobs found for this network.
          </p>
        ) : (
          <div className="space-y-1.5">
            {jobs.map((job) => (
              <div
                key={job.id}
                className="flex items-center justify-between gap-2 px-3 py-2 rounded bg-surface-raised border border-border/50"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <StatusBadge status={job.status ?? "unknown"} />
                    <span className="text-xs text-text-muted whitespace-nowrap">
                      {job.started_at
                        ? formatRelativeTime(job.started_at)
                        : "—"}
                    </span>
                  </div>
                  <div className="flex items-center gap-3 mt-0.5">
                    <span className="text-[11px] text-text-muted">
                      {job.method
                        ? (DISCOVERY_METHOD_LABELS[job.method] ?? job.method)
                        : "—"}
                    </span>
                    {job.hosts_found != null && (
                      <span className="text-[11px] text-text-secondary">
                        <span className="font-medium text-text-primary">
                          {job.hosts_found}
                        </span>{" "}
                        hosts found
                      </span>
                    )}
                    {job.status === "running" && (
                      <div className="flex items-center gap-1">
                        <div className="w-12 bg-border rounded-full h-1">
                          <div
                            className="bg-accent h-1 rounded-full"
                            style={{ width: `${job.progress ?? 0}%` }}
                          />
                        </div>
                        <span className="text-[10px] text-text-muted">
                          {job.progress ?? 0}%
                        </span>
                      </div>
                    )}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => handleRerun(job)}
                  disabled={isRerunning}
                  className="shrink-0 flex items-center gap-1 text-[11px] text-text-muted hover:text-accent transition-colors disabled:opacity-50"
                  title="Run again"
                >
                  <RefreshCw className="h-3 w-3" />
                  Run again
                </button>
              </div>
            ))}
          </div>
        )}

        {!isLoading && totalPages > 1 && (
          <div className="mt-3">
            <PaginationBar
              page={page}
              totalPages={totalPages}
              onPrev={() => setPage((p) => Math.max(1, p - 1))}
              onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
            />
          </div>
        )}
      </section>
    </div>
  );
}

// ── Network detail panel ──────────────────────────────────────────────────────

type DetailTab = "overview" | "discoveries" | "exclusions";

interface DetailPanelProps {
  network: NetworkResponse;
  onClose: () => void;
}

function NetworkDetailPanel({
  network: initialNetwork,
  onClose,
}: DetailPanelProps) {
  const [activeTab, setActiveTab] = useState<DetailTab>("overview");
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showScanModal, setShowScanModal] = useState(false);
  const [showSmartScanPreview, setShowSmartScanPreview] = useState(false);

  const { toast } = useToast();

  const { mutateAsync: enableNetwork, isPending: isEnabling } =
    useEnableNetwork();
  const { mutateAsync: disableNetwork, isPending: isDisabling } =
    useDisableNetwork();
  const { mutateAsync: deleteNetwork, isPending: isDeleting } =
    useDeleteNetwork();
  const { mutateAsync: discoverNetwork, isPending: isDiscovering } =
    useStartNetworkDiscovery();
  const { data: smartScanSuggestions, isLoading: isSuggestionsLoading } =
    useSmartScanSuggestions(showSmartScanPreview);
  const { mutateAsync: triggerBatch, isPending: isBatchPending } =
    useTriggerSmartScanBatch();

  async function handleSmartScanBatch() {
    try {
      const result = await triggerBatch({});
      setShowSmartScanPreview(false);
      toast.success(
        result.queued === 0
          ? "No hosts needed scanning."
          : `Smart Scan queued for ${result.queued} host${result.queued === 1 ? "" : "s"}.`,
      );
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to queue Smart Scan batch.",
      );
    }
  }

  const isTogglingActive = isEnabling || isDisabling;
  const n = initialNetwork;

  async function handleToggleActive() {
    setActionError(null);
    try {
      if (n.is_active) {
        await disableNetwork(n.id ?? "");
        toast.success("Network disabled");
      } else {
        await enableNetwork(n.id ?? "");
        toast.success("Network enabled");
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed.");
      setActionError(err instanceof Error ? err.message : "Action failed.");
    }
  }

  async function handleDelete() {
    setActionError(null);
    try {
      await deleteNetwork(n.id ?? "");
      toast.success("Network deleted");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed.");
      setActionError(err instanceof Error ? err.message : "Delete failed.");
      setShowDeleteConfirm(false);
    }
  }

  async function handleDiscover() {
    setActionError(null);
    try {
      await discoverNetwork(n.id ?? "");
      toast.success("Discovery job started");
      setActiveTab("discoveries");
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : "Failed to start discovery.";
      toast.error(msg);
      setActionError(msg);
    }
  }

  const tabs: { id: DetailTab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "discoveries", label: "Discoveries" },
    { id: "exclusions", label: "Exclusions" },
  ];

  return (
    <>
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      <div
        role="dialog"
        aria-label="Network details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-110",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden",
          "shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-xs text-text-muted">Network</p>
            <p className="text-sm font-medium text-text-primary truncate">
              {n.name ?? "—"}
            </p>
            <p className="text-xs font-mono text-text-secondary">
              {n.cidr ?? "—"}
            </p>
            <NetworkActiveBadge active={n.is_active ?? false} />
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

        {/* Action bar */}
        <div className="flex items-center gap-2 px-5 py-3 border-b border-border shrink-0 flex-wrap">
          <Button
            variant="secondary"
            onClick={() => void handleToggleActive()}
            loading={isTogglingActive}
            className="text-xs h-7 px-3"
          >
            {n.is_active ? (
              <>
                <Ban className="h-3 w-3 mr-1" /> Disable
              </>
            ) : (
              <>
                <Check className="h-3 w-3 mr-1" /> Enable
              </>
            )}
          </Button>

          <Button
            variant="secondary"
            onClick={() => setShowEditModal(true)}
            className="text-xs h-7 px-3"
          >
            <Pencil className="h-3 w-3 mr-1" />
            Edit
          </Button>

          <Button
            variant="secondary"
            onClick={() => void handleDiscover()}
            loading={isDiscovering}
            className="text-xs h-7 px-3"
          >
            <Play className="h-3 w-3 mr-1" />
            Discover
          </Button>

          <Button
            variant="secondary"
            onClick={() => setShowScanModal(true)}
            className="text-xs h-7 px-3"
          >
            <Radar className="h-3 w-3 mr-1" />
            Scan hosts
          </Button>

          <Button
            variant="secondary"
            onClick={() => setShowSmartScanPreview(true)}
            disabled={isSuggestionsLoading}
            title="Queue the next recommended scan for all eligible hosts"
            className="text-xs h-7 px-3"
          >
            <Zap className="h-3 w-3 mr-1" />
            Smart Scan
          </Button>

          {showDeleteConfirm ? (
            <div className="flex items-center gap-2 ml-auto">
              <span className="text-xs text-text-muted">
                Delete this network?
              </span>
              <button
                type="button"
                onClick={() => setShowDeleteConfirm(false)}
                className="text-xs text-text-muted hover:text-text-secondary"
              >
                Cancel
              </button>
              <Button
                variant="danger"
                onClick={() => void handleDelete()}
                loading={isDeleting}
                className="text-xs h-7 px-3"
              >
                Delete
              </Button>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="ml-auto flex items-center gap-1 text-xs text-text-muted hover:text-danger transition-colors"
            >
              <Trash2 className="h-3 w-3" />
              Delete
            </button>
          )}

          {actionError && (
            <p className="w-full text-[11px] text-danger mt-0.5">
              {actionError}
            </p>
          )}
        </div>

        {/* Tab bar */}
        <div className="flex border-b border-border shrink-0">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                "px-4 py-2.5 text-xs font-medium transition-colors border-b-2 -mb-px",
                activeTab === tab.id
                  ? "border-accent text-accent"
                  : "border-transparent text-text-muted hover:text-text-secondary",
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {activeTab === "overview" && (
            <section>
              <h3 className="text-xs font-medium text-text-primary mb-3 flex items-center gap-1.5">
                <Network className="h-3.5 w-3.5 text-text-muted" />
                Details
              </h3>
              <div className="space-y-2">
                <MetaRow label="ID" value={n.id} />
                <MetaRow label="Description" value={n.description} />
                <MetaRow
                  label="Discovery method"
                  value={
                    n.discovery_method
                      ? (DISCOVERY_METHOD_LABELS[n.discovery_method] ??
                        n.discovery_method)
                      : undefined
                  }
                />
                <MetaRow
                  label="Scan enabled"
                  value={
                    n.scan_enabled != null
                      ? n.scan_enabled
                        ? "Yes"
                        : "No"
                      : undefined
                  }
                />
                <MetaRow
                  label="Total hosts"
                  value={
                    n.host_count != null ? String(n.host_count) : undefined
                  }
                />
                <MetaRow
                  label="Active hosts"
                  value={
                    n.active_host_count != null
                      ? String(n.active_host_count)
                      : undefined
                  }
                />
                <MetaRow
                  label="Last discovery"
                  value={
                    n.last_discovery
                      ? formatRelativeTime(n.last_discovery)
                      : undefined
                  }
                />
                <MetaRow
                  label="Last scan"
                  value={
                    n.last_scan ? formatRelativeTime(n.last_scan) : undefined
                  }
                />
                <MetaRow label="Created by" value={n.created_by} />
                <MetaRow
                  label="Created at"
                  value={
                    n.created_at ? formatAbsoluteTime(n.created_at) : undefined
                  }
                />
                <MetaRow
                  label="Updated at"
                  value={
                    n.updated_at ? formatAbsoluteTime(n.updated_at) : undefined
                  }
                />
              </div>
            </section>
          )}

          {activeTab === "discoveries" && n.id && (
            <NetworkDiscoveriesTab network={n} />
          )}

          {activeTab === "exclusions" && n.id && (
            <ExclusionsSection networkId={n.id} />
          )}
        </div>
      </div>

      {showEditModal && (
        <EditNetworkModal
          network={n}
          onClose={() => setShowEditModal(false)}
          onSaved={() => setShowEditModal(false)}
        />
      )}

      {showScanModal && (
        <ScanNetworkModal network={n} onClose={() => setShowScanModal(false)} />
      )}

      {showSmartScanPreview && smartScanSuggestions && (
        <BatchSmartScanPreviewModal
          networkName={n.name ?? n.cidr ?? "Network"}
          summary={smartScanSuggestions}
          isPending={isBatchPending}
          onConfirm={() => void handleSmartScanBatch()}
          onClose={() => setShowSmartScanPreview(false)}
        />
      )}
    </>
  );
}

// ── All-networks discovery jobs section ───────────────────────────────────────

function DiscoverySection() {
  const [page, setPage] = useState(1);
  const [showCreate, setShowCreate] = useState(false);
  const { toast } = useToast();

  const queryParams = { page, page_size: DISCOVERY_PAGE_SIZE };
  const { data, isLoading } = useDiscoveryJobs(queryParams);

  const jobs = data?.data ?? [];
  const totalPages = data?.pagination?.total_pages ?? 1;

  const { mutate: startDiscovery } = useStartDiscovery();
  const { mutate: stopDiscovery } = useStopDiscovery();
  const { mutate: rerunDiscovery, isPending: isRerunning } =
    useRerunDiscovery();

  function handleRerun(job: DiscoveryJobResponse) {
    rerunDiscovery(
      {
        networks: job.networks ?? [],
        method: job.method ?? "tcp_connect",
        name: job.name ?? undefined,
      },
      {
        onSuccess: () => toast.success("Discovery restarted"),
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : "Failed to rerun"),
      },
    );
  }

  return (
    <>
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-text-primary flex items-center gap-2">
            <ScanSearch className="h-4 w-4 text-text-muted" />
            Discovery Jobs
          </h2>
          <Button
            onClick={() => setShowCreate(true)}
            variant="secondary"
            className="text-xs h-7 px-3"
          >
            <Plus className="h-3 w-3 mr-1" />
            New discovery
          </Button>
        </div>

        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border bg-surface-raised text-left">
                  <th className="font-medium text-text-muted px-4 py-2.5 pr-4 whitespace-nowrap">
                    Name / Network
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap">
                    Method
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap">
                    Status
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap text-right">
                    Hosts found
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap">
                    Progress
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap">
                    Started
                  </th>
                  <th className="font-medium text-text-muted py-2.5 pr-4 whitespace-nowrap">
                    Created
                  </th>
                  <th className="py-2.5" />
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <DiscoverySkeletonRows count={5} />
                ) : jobs.length === 0 ? (
                  <tr>
                    <td
                      colSpan={8}
                      className="py-8 text-center text-xs text-text-muted"
                    >
                      No discovery jobs found.
                    </td>
                  </tr>
                ) : (
                  jobs.map((job) => (
                    <tr
                      key={job.id}
                      className={cn(
                        "border-b border-border/50 last:border-0",
                        "hover:bg-surface-raised/50 transition-colors",
                      )}
                    >
                      <td className="py-2.5 px-4 pr-4">
                        <div className="text-text-secondary">
                          {job.name ?? "—"}
                        </div>
                        <div className="font-mono text-text-muted">
                          {job.networks?.join(", ") ?? "—"}
                        </div>
                      </td>
                      <td className="py-2.5 pr-4 text-text-secondary">
                        {job.method
                          ? (DISCOVERY_METHOD_LABELS[job.method] ?? job.method)
                          : "—"}
                      </td>
                      <td className="py-2.5 pr-4">
                        <StatusBadge status={job.status ?? "unknown"} />
                      </td>
                      <td className="py-2.5 pr-4 text-right tabular-nums text-text-secondary">
                        {job.hosts_found != null ? job.hosts_found : "—"}
                      </td>
                      <td className="py-2.5 pr-4">
                        {job.status === "running" ? (
                          <div className="w-full bg-border rounded-full h-1 min-w-16">
                            <div
                              className="bg-accent h-1 rounded-full"
                              style={{ width: `${job.progress ?? 0}%` }}
                            />
                          </div>
                        ) : (
                          <span className="text-text-muted">—</span>
                        )}
                      </td>
                      <td className="py-2.5 pr-4 text-text-muted whitespace-nowrap">
                        {job.started_at
                          ? formatRelativeTime(job.started_at)
                          : "—"}
                      </td>
                      <td className="py-2.5 pr-4 text-text-muted whitespace-nowrap">
                        {job.created_at
                          ? formatRelativeTime(job.created_at)
                          : "—"}
                      </td>
                      <td
                        className="py-2.5 pr-4"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <div className="flex items-center gap-2">
                          {job.status === "pending" && (
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => startDiscovery(job.id ?? "")}
                            >
                              ▶ Start
                            </Button>
                          )}
                          {job.status === "running" && (
                            <Button
                              variant="danger"
                              size="sm"
                              onClick={() => stopDiscovery(job.id ?? "")}
                            >
                              ■ Stop
                            </Button>
                          )}
                          {(job.status === "completed" ||
                            job.status === "failed") && (
                            <button
                              type="button"
                              onClick={() => handleRerun(job)}
                              disabled={isRerunning}
                              className="flex items-center gap-1 text-[11px] text-text-muted hover:text-accent transition-colors disabled:opacity-50 whitespace-nowrap"
                            >
                              <RefreshCw className="h-3 w-3" />
                              Run again
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {!isLoading && jobs.length > 0 && totalPages > 1 && (
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

      {showCreate && (
        <CreateDiscoveryModal onClose={() => setShowCreate(false)} />
      )}
    </>
  );
}

// ── Networks page ─────────────────────────────────────────────────────────────

export function NetworksPage() {
  const [page, setPage] = useState(1);
  const [showInactive, setShowInactive] = useState(false);
  const [nameSearch, setNameSearch] = useState("");
  const [debouncedName, setDebouncedName] = useState("");
  const [sortBy, setSortBy] = useState("cidr");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");
  const [selectedNetwork, setSelectedNetwork] =
    useState<NetworkResponse | null>(null);
  const [showAddNetwork, setShowAddNetwork] = useState(false);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSort = useCallback(
    (column: string) => {
      if (sortBy === column) {
        setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
      } else {
        setSortBy(column);
        setSortOrder("asc");
      }
    },
    [sortBy],
  );

  const handleNameInput = useCallback((value: string) => {
    setNameSearch(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedName(value);
      setPage(1);
    }, 300);
  }, []);

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    ...(showInactive ? { show_inactive: true } : {}),
    ...(debouncedName.trim() ? { name: debouncedName.trim() } : {}),
  };

  const { data, isLoading } = useNetworks(queryParams);
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  const sortedNetworks = useMemo(() => {
    const raw = data?.data ?? [];
    if (!sortBy) return raw;
    return [...raw].sort((a, b) => {
      const aVal = a[sortBy as keyof typeof a] ?? "";
      const bVal = b[sortBy as keyof typeof b] ?? "";
      const cmp = String(aVal).localeCompare(String(bVal), undefined, {
        numeric: true,
      });
      return sortOrder === "asc" ? cmp : -cmp;
    });
  }, [data, sortBy, sortOrder]);

  function handleShowInactiveChange(checked: boolean) {
    setShowInactive(checked);
    setPage(1);
  }

  return (
    <div className="flex flex-col gap-6 h-full">
      {/* ── Networks section ─────────────────────────────────────────── */}
      <div className="flex flex-col gap-4">
        {/* Toolbar */}
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative flex-1 min-w-48 max-w-64">
            <input
              type="text"
              value={nameSearch}
              onChange={(e) => handleNameInput(e.target.value)}
              placeholder="Search by name…"
              aria-label="Search networks"
              className={cn(
                "w-full pl-3 pr-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          <label className="flex items-center gap-2 text-xs text-text-secondary cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showInactive}
              onChange={(e) => handleShowInactiveChange(e.target.checked)}
              aria-label="Show inactive networks"
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            Show inactive
          </label>

          <div className="flex-1" />

          <Button
            onClick={() => setShowAddNetwork(true)}
            className="text-xs h-7 px-3"
          >
            <Plus className="h-3 w-3 mr-1" />
            Add network
          </Button>
        </div>

        {/* Table */}
        <div className="overflow-auto rounded border border-border">
          <table className="w-full text-xs border-collapse min-w-160">
            <thead>
              <tr className="bg-surface-raised border-b border-border text-left">
                <SortHeader
                  label="Name"
                  column="name"
                  sortBy={sortBy}
                  sortOrder={sortOrder}
                  onSort={handleSort}
                  className="px-4 py-2.5"
                />
                <SortHeader
                  label="CIDR"
                  column="cidr"
                  sortBy={sortBy}
                  sortOrder={sortOrder}
                  onSort={handleSort}
                  className="px-4 py-2.5"
                />
                <SortHeader
                  label="Hosts"
                  column="host_count"
                  sortBy={sortBy}
                  sortOrder={sortOrder}
                  onSort={handleSort}
                  className="px-4 py-2.5 text-right"
                />
                <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap text-right">
                  Active
                </th>
                <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                  Discovery
                </th>
                <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                  Status
                </th>
                <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                  Last Discovery
                </th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <NetworkSkeletonRows />
              ) : sortedNetworks.length === 0 ? (
                <tr>
                  <td
                    colSpan={7}
                    className="px-4 py-10 text-center text-text-muted"
                  >
                    No networks found.
                  </td>
                </tr>
              ) : (
                sortedNetworks.map((network) => (
                  <tr
                    key={network.id}
                    onClick={() =>
                      setSelectedNetwork(
                        selectedNetwork?.id === network.id ? null : network,
                      )
                    }
                    className={cn(
                      "border-b border-border cursor-pointer transition-colors",
                      "hover:bg-surface-raised",
                      selectedNetwork?.id === network.id && "bg-accent/8",
                    )}
                  >
                    <td className="px-4 py-2.5 text-text-primary font-medium truncate max-w-45">
                      {network.name ?? "—"}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-text-secondary whitespace-nowrap">
                      {network.cidr ?? "—"}
                    </td>
                    <td className="px-4 py-2.5 text-text-secondary text-right tabular-nums">
                      {network.host_count != null ? network.host_count : "—"}
                    </td>
                    <td className="px-4 py-2.5 text-text-secondary text-right tabular-nums">
                      {network.active_host_count != null
                        ? network.active_host_count
                        : "—"}
                    </td>
                    <td className="px-4 py-2.5 text-text-secondary">
                      {network.discovery_method
                        ? (DISCOVERY_METHOD_LABELS[network.discovery_method] ??
                          network.discovery_method)
                        : "—"}
                    </td>
                    <td className="px-4 py-2.5">
                      <NetworkActiveBadge active={network.is_active ?? false} />
                    </td>
                    <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                      {network.last_discovery
                        ? formatRelativeTime(network.last_discovery)
                        : "—"}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {!isLoading && sortedNetworks.length > 0 && totalPages > 1 && (
          <PaginationBar
            page={page}
            totalPages={totalPages}
            onPrev={() => setPage((p) => Math.max(1, p - 1))}
            onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
          />
        )}
      </div>

      {/* ── Discovery section ─────────────────────────────────────────── */}
      <DiscoverySection />

      {/* Network detail panel */}
      {selectedNetwork && (
        <NetworkDetailPanel
          network={selectedNetwork}
          onClose={() => setSelectedNetwork(null)}
        />
      )}

      {/* Add network modal */}
      {showAddNetwork && (
        <AddNetworkModal onClose={() => setShowAddNetwork(false)} />
      )}
    </div>
  );
}
