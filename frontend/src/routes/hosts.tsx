import { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { isNotFound } from "../api/errors";
import {
  Search,
  ScanLine,
  X,
  Monitor,
  Pencil,
  Check,
  Trash2,
  Activity,
  Play,
  SlidersHorizontal,
  Plus,
  FileText,
  ChevronDown,
  ChevronRight,
  ShieldCheck,
  Zap,
} from "lucide-react";
import { SortHeader } from "../components/sort-header";
import type { SortOrder } from "../components/sort-header";
import { Button } from "../components/button";
import {
  useHosts,
  useHost,
  useHostScans,
  useUpdateHost,
  useDeleteHost,
  useBulkDeleteHosts,
} from "../api/hooks/use-hosts";
import { useToast } from "../components/toast-provider";
import {
  StatusBadge,
  Skeleton,
  PaginationBar,
  RunScanModal,
} from "../components";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import type { components } from "../api/types";
import { ColumnToggle } from "../components/column-toggle";
import type { ColumnDef } from "../components/column-toggle";
import { useTableKeyNav } from "../hooks/use-table-key-nav";
import { FilterBuilder } from "../components/filter-builder";
import { TagInput } from "../components/tag-input";
import type { FilterGroup } from "../lib/filter-expr";
import { deserializeFilter, serializeFilter } from "../lib/filter-expr";
import { useTags, useUpdateHostTags } from "../api/hooks/use-tags";
import { useGroups, useAddHostsToGroup } from "../api/hooks/use-groups";
import {
  useSmartScanStage,
  useTriggerSmartScan,
} from "../api/hooks/use-smart-scan";
import { HostSmartScanPreviewModal } from "../components/smart-scan-preview-modal";

type HostResponse = components["schemas"]["docs.HostResponse"];

interface PortInfo {
  port?: number;
  protocol?: string;
  state?: string;
  service?: string;
  last_seen?: string;
}

/** Extended host shape — includes fields the API returns but the generated schema omits */
type PortBanner = {
  id: string;
  host_id: string;
  port: number;
  protocol: string;
  raw_banner?: string;
  service?: string;
  version?: string;
  scanned_at: string;
};

type TLSCertificate = {
  id: string;
  host_id: string;
  port: number;
  subject_cn?: string;
  sans?: string[];
  issuer?: string;
  not_before?: string;
  not_after?: string;
  key_type?: string;
  tls_version?: string;
  raw_banner?: string;
  scanned_at: string;
};

type HostWithDetails = HostResponse & {
  ports?: PortInfo[];
  /** Open port count from the list query (not populated in detail view). */
  total_ports?: number;
  os_family?: string;
  os_name?: string;
  os_version_detail?: string;
  os_confidence?: number;
  vendor?: string;
  response_time_ms?: number | null;
  response_time_min_ms?: number | null;
  response_time_max_ms?: number | null;
  response_time_avg_ms?: number | null;
  timeout_count?: number;
  groups?: Array<{ id: string; name: string; color?: string }>;
  dns_records?: Array<{
    id: string;
    record_type: string;
    value: string;
    ttl?: number;
    resolved_at: string;
  }>;
  banners?: PortBanner[];
  certificates?: TLSCertificate[];
  snmp_data?: SNMPData | null;
};

interface SNMPInterface {
  name?: string;
  status?: string;
  speed_mbps?: number;
  mac?: string;
  ip?: string;
}

interface SNMPData {
  sys_name?: string;
  sys_descr?: string;
  sys_location?: string;
  sys_contact?: string;
  sys_uptime_cs?: number;
  if_count?: number;
  interfaces?: SNMPInterface[];
  collected_at?: string;
}

const PAGE_SIZE = 25;

// ──────────────────────────────────────────────
// Host detail panel
// ──────────────────────────────────────────────

/** Converts SNMP sysUptime centiseconds to a human-readable string. */
function formatSNMPUptime(cs: number): string {
  const totalSeconds = Math.floor(cs / 100);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  parts.push(`${minutes}m`);
  return parts.join(" ");
}

function MetaRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-xs">
      <span className="text-text-muted w-28 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value ?? "—"}</span>
    </div>
  );
}

function HostDetailPanel({
  host,
  onClose,
  onScan,
  allTags,
  onTagFilter,
  allGroups,
}: {
  host: HostResponse;
  onClose: () => void;
  onScan: (ip: string) => void;
  allTags: string[];
  onTagFilter: (tag: string) => void;
  allGroups: Array<{ id: string; name: string; color?: string }>;
}) {
  const { data: full, isLoading, isError, error } = useHost(host.id ?? "");
  const h = (full ?? host) as HostWithDetails;

  // Expanded port banners: Set of "port-protocol" keys with banner expanded
  const [expandedBanners, setExpandedBanners] = useState<Set<string>>(new Set());

  function toggleBanner(port: number, protocol: string) {
    const key = `${port}-${protocol}`;
    setExpandedBanners((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  // Show-more state for truncated banners
  const [expandedRawBanners, setExpandedRawBanners] = useState<Set<string>>(
    new Set(),
  );

  function toggleRawBanner(key: string) {
    setExpandedRawBanners((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  // Hostname editing
  const [isEditingHostname, setIsEditingHostname] = useState(false);
  const [hostnameInput, setHostnameInput] = useState("");
  const [hostnameError, setHostnameError] = useState<string | null>(null);

  // Tags
  const [localTags, setLocalTags] = useState<string[] | null>(null);
  const { mutateAsync: updateTags } = useUpdateHostTags();
  const displayTags = localTags ?? (h.tags ?? []);

  // Groups
  const { mutateAsync: addToGroup, isPending: isAddingToGroup } = useAddHostsToGroup();
  const [addGroupOpen, setAddGroupOpen] = useState(false);
  const hostGroupIds = new Set((h.groups ?? []).map((g) => g.id));
  const availableGroups = allGroups.filter((g) => !hostGroupIds.has(g.id));

  async function handleTagsChange(tags: string[]) {
    setLocalTags(tags);
    try {
      await updateTags({ hostId: h.id ?? "", tags });
    } catch {
      setLocalTags(null);
      toast.error("Failed to update tags.");
    }
  }

  async function handleAddToGroup(groupId: string) {
    setAddGroupOpen(false);
    try {
      await addToGroup({ groupId, hostIds: [h.id ?? ""] });
      toast.success("Host added to group.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add to group.");
    }
  }

  // Delete
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Scan history pagination
  const [scanHistoryPage, setScanHistoryPage] = useState(1);
  const SCAN_HISTORY_PAGE_SIZE = 5;

  const { toast } = useToast();
  const { mutateAsync: updateHost, isPending: isUpdatingHost } =
    useUpdateHost();
  const { mutateAsync: deleteHost, isPending: isDeletingHost } =
    useDeleteHost();
  const { data: hostScansData, isLoading: hostScansLoading } = useHostScans(
    host.id ?? "",
    { page: scanHistoryPage, page_size: SCAN_HISTORY_PAGE_SIZE },
  );

  // Smart Scan
  const [showSmartScanPreview, setShowSmartScanPreview] = useState(false);
  const { data: smartScanStage, isLoading: smartScanStageLoading } =
    useSmartScanStage(showSmartScanPreview ? (h.id ?? "") : "");
  const { mutateAsync: triggerSmartScan, isPending: isSmartScanPending } =
    useTriggerSmartScan();

  const hasPendingScan = (hostScansData?.data ?? []).some(
    (s) => s.status === "pending" || s.status === "running",
  );

  async function handleSmartScanConfirm() {
    try {
      const result = await triggerSmartScan(h.id ?? "");
      setShowSmartScanPreview(false);
      if (result && "queued" in result && !result.queued) {
        toast.success("No scan needed — host knowledge is sufficient.");
      } else {
        toast.success("Smart Scan queued.");
      }
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to queue Smart Scan.",
      );
    }
  }

  async function handleSaveHostname() {
    setHostnameError(null);
    const trimmed = hostnameInput.trim();
    try {
      await updateHost({
        hostId: h.id ?? "",
        body: { hostname: trimmed || undefined },
      });
      setIsEditingHostname(false);
      toast.success("Hostname updated");
    } catch (err) {
      setHostnameError(err instanceof Error ? err.message : "Update failed.");
      toast.error(
        err instanceof Error ? err.message : "Failed to update hostname.",
      );
    }
  }

  async function handleDeleteHost() {
    setDeleteError(null);
    try {
      await deleteHost(h.id ?? "");
      toast.success("Host deleted");
      onClose();
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Delete failed.");
      setShowDeleteConfirm(false);
      toast.error(
        err instanceof Error ? err.message : "Failed to delete host.",
      );
    }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Panel */}
      <div
        role="dialog"
        aria-label="Host details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-105",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden",
          "shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-xs text-text-muted">Host</p>
            <p className="text-sm font-mono text-text-primary truncate">
              {h.ip_address ?? "—"}
            </p>
            <StatusBadge status={h.status ?? "unknown"} />
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

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
          {isError && (
            <p className="text-xs text-danger">
              {isNotFound(error)
                ? "This host no longer exists."
                : "Failed to load host details."}
            </p>
          )}

          {/* OS Detection */}
          {(isLoading || h.os_family || h.os_name || h.os_version_detail) && (
            <section>
              <h3 className="text-xs font-medium text-text-primary mb-3 flex items-center gap-1.5">
                <Monitor className="h-3.5 w-3.5 text-text-muted" />
                OS Detection
              </h3>
              {isLoading ? (
                <div className="space-y-2">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <div key={i} className="flex gap-2">
                      <Skeleton className="h-3 w-28 shrink-0" />
                      <Skeleton className="h-3 w-40" />
                    </div>
                  ))}
                </div>
              ) : (
                <div className="space-y-2">
                  <MetaRow label="Family" value={h.os_family} />
                  <MetaRow label="Name" value={h.os_name} />
                  <MetaRow label="Version" value={h.os_version_detail} />
                  <MetaRow
                    label="Confidence"
                    value={
                      h.os_confidence != null ? (
                        <span className="flex items-center gap-1.5">
                          <span className="tabular-nums">
                            {h.os_confidence}%
                          </span>
                          <span className="w-20 h-1.5 rounded-full bg-surface-raised overflow-hidden">
                            <span
                              className="block h-full rounded-full bg-accent/70"
                              style={{ width: `${h.os_confidence}%` }}
                            />
                          </span>
                        </span>
                      ) : undefined
                    }
                  />
                </div>
              )}
            </section>
          )}

          {/* Identity */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Identity
            </h3>
            {isLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="flex gap-2">
                    <Skeleton className="h-3 w-28 shrink-0" />
                    <Skeleton className="h-3 w-40" />
                  </div>
                ))}
              </div>
            ) : (
              <div className="space-y-2">
                <MetaRow label="ID" value={h.id} />
                <MetaRow
                  label="IP Address"
                  value={<span className="font-mono">{h.ip_address}</span>}
                />

                {/* Inline hostname editing */}
                <div className="flex gap-2 text-xs">
                  <span className="text-text-muted w-28 shrink-0">
                    Hostname
                  </span>
                  {isEditingHostname ? (
                    <div className="flex items-center gap-1.5 flex-1 min-w-0">
                      <input
                        type="text"
                        value={hostnameInput}
                        onChange={(e) => setHostnameInput(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") void handleSaveHostname();
                          if (e.key === "Escape") setIsEditingHostname(false);
                        }}
                        autoFocus
                        aria-label="Edit hostname"
                        className="flex-1 px-2 py-0.5 text-xs rounded border border-border bg-surface text-text-primary focus:outline-none focus:ring-1 focus:ring-border min-w-0"
                      />
                      <button
                        type="button"
                        onClick={() => void handleSaveHostname()}
                        disabled={isUpdatingHost}
                        aria-label="Save hostname"
                        className="p-0.5 rounded text-success hover:bg-surface-raised"
                      >
                        <Check className="h-3 w-3" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setIsEditingHostname(false)}
                        aria-label="Cancel"
                        className="p-0.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </div>
                  ) : (
                    <div className="flex items-center gap-1.5 min-w-0">
                      <span className="text-text-secondary break-all">
                        {h.hostname ?? "—"}
                      </span>
                      <button
                        type="button"
                        onClick={() => {
                          setIsEditingHostname(true);
                          setHostnameInput(h.hostname ?? "");
                        }}
                        aria-label="Edit hostname"
                        className="p-0.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised shrink-0"
                      >
                        <Pencil className="h-2.5 w-2.5" />
                      </button>
                    </div>
                  )}
                </div>
                {hostnameError && (
                  <p className="text-[11px] text-danger ml-30">
                    {hostnameError}
                  </p>
                )}

                <MetaRow
                  label="MAC Address"
                  value={
                    h.mac_address ? (
                      <span className="font-mono">{h.mac_address}</span>
                    ) : undefined
                  }
                />
                <MetaRow
                  label="Status"
                  value={<StatusBadge status={h.status ?? "unknown"} />}
                />
              </div>
            )}
          </section>

          {/* Tags */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Tags
            </h3>
            {isLoading ? (
              <Skeleton className="h-7 w-full rounded" />
            ) : (
              <div className="space-y-2">
                <TagInput
                  tags={displayTags}
                  allTags={allTags}
                  onChange={(tags) => void handleTagsChange(tags)}
                />
                {displayTags.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1">
                    {displayTags.map((tag) => (
                      <button
                        key={tag}
                        type="button"
                        onClick={() => {
                          onClose();
                          onTagFilter(tag);
                        }}
                        title={`Filter by "${tag}"`}
                        className="text-[11px] text-accent/70 hover:text-accent underline-offset-2 hover:underline transition-colors"
                      >
                        filter by {tag}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
          </section>

          {/* Groups */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs font-medium text-text-primary">Groups</h3>
              {availableGroups.length > 0 && (
                <div className="relative">
                  <button
                    type="button"
                    onClick={() => setAddGroupOpen((o) => !o)}
                    disabled={isAddingToGroup}
                    className="text-[11px] text-text-muted hover:text-text-primary transition-colors flex items-center gap-1"
                  >
                    <Plus className="h-3 w-3" /> Add to group
                  </button>
                  {addGroupOpen && (
                    <div className="absolute right-0 top-full mt-1 z-30 w-44 bg-surface border border-border rounded-md shadow-lg py-1 max-h-40 overflow-y-auto">
                      {availableGroups.map((g) => (
                        <button
                          key={g.id}
                          type="button"
                          onClick={() => void handleAddToGroup(g.id)}
                          className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-raised flex items-center gap-2"
                        >
                          <span
                            className="h-2 w-2 rounded-full shrink-0"
                            style={{ backgroundColor: g.color ?? "#64748b" }}
                          />
                          <span className="truncate">{g.name}</span>
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
            {isLoading ? (
              <Skeleton className="h-5 w-32 rounded" />
            ) : (h.groups ?? []).length === 0 ? (
              <p className="text-xs text-text-muted">Not in any group.</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {(h.groups ?? []).map((g) => (
                  <span
                    key={g.id}
                    className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium border"
                    style={{
                      backgroundColor: g.color ? `${g.color}20` : undefined,
                      borderColor: g.color ? `${g.color}50` : undefined,
                      color: g.color ?? undefined,
                    }}
                  >
                    {g.name}
                  </span>
                ))}
              </div>
            )}
          </section>

          {/* Activity */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Activity
            </h3>
            {isLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="flex gap-2">
                    <Skeleton className="h-3 w-28 shrink-0" />
                    <Skeleton className="h-3 w-32" />
                  </div>
                ))}
              </div>
            ) : (
              <div className="space-y-2">
                <MetaRow
                  label="First seen"
                  value={
                    h.first_seen ? formatRelativeTime(h.first_seen) : undefined
                  }
                />
                <MetaRow
                  label="Last seen"
                  value={
                    h.last_seen ? formatRelativeTime(h.last_seen) : undefined
                  }
                />
                <MetaRow label="Scan count" value={h.scan_count} />
              </div>
            )}
          </section>

          {/* Network / Response Time */}
          {!isLoading &&
            (h.response_time_avg_ms != null || (h.timeout_count ?? 0) > 0) && (
              <section>
                <h3 className="text-xs font-medium text-text-primary mb-3 flex items-center gap-1.5">
                  <Activity className="h-3.5 w-3.5 text-text-muted" />
                  Network
                </h3>
                <div className="space-y-2">
                  {h.response_time_min_ms != null && (
                    <MetaRow
                      label="RTT min"
                      value={`${h.response_time_min_ms} ms`}
                    />
                  )}
                  {h.response_time_avg_ms != null && (
                    <MetaRow
                      label="RTT avg"
                      value={
                        <span className="flex items-center gap-1.5">
                          <span className="tabular-nums">
                            {h.response_time_avg_ms} ms
                          </span>
                          {h.response_time_avg_ms > 100 && (
                            <span className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium bg-warning/10 text-warning border border-warning/20">
                              Slow
                            </span>
                          )}
                        </span>
                      }
                    />
                  )}
                  {h.response_time_max_ms != null && (
                    <MetaRow
                      label="RTT max"
                      value={`${h.response_time_max_ms} ms`}
                    />
                  )}
                  {(h.timeout_count ?? 0) > 0 && (
                    <MetaRow label="Timeouts" value={h.timeout_count} />
                  )}
                </div>
              </section>
            )}

          {/* Ports */}
          <section>
            {(() => {
              const allPorts = h.ports ?? [];
              const openPorts = allPorts.filter((p) => p.state === "open");
              const otherPorts = allPorts.filter((p) => p.state !== "open");
              return (
                <>
                  <h3 className="text-xs font-medium text-text-primary mb-3">
                    {isLoading
                      ? "Open Ports"
                      : `Open Ports (${openPorts.length})`}
                  </h3>
                  {isLoading ? (
                    <div className="space-y-1.5">
                      {Array.from({ length: 4 }).map((_, i) => (
                        <Skeleton key={i} className="h-5 w-full rounded" />
                      ))}
                    </div>
                  ) : openPorts.length === 0 ? (
                    <p className="text-xs text-text-muted">
                      No open ports recorded.
                    </p>
                  ) : (
                    <div className="space-y-1">
                      {openPorts.map((p) => {
                        const portKey = `${p.port}-${p.protocol}`;
                        const banner = (h as HostWithDetails).banners?.find(
                          (b) => b.port === p.port && b.protocol === (p.protocol ?? "tcp"),
                        );
                        const cert = (h as HostWithDetails).certificates?.find(
                          (c) => c.port === p.port,
                        );
                        const hasBanner = !!banner;
                        const hasCert = !!cert;
                        const isExpanded = expandedBanners.has(portKey);
                        const certDaysLeft = cert?.not_after
                          ? Math.ceil(
                              (new Date(cert.not_after).getTime() - Date.now()) /
                                86400000,
                            )
                          : null;
                        const certUrgent =
                          certDaysLeft !== null && certDaysLeft <= 30;
                        const certColor =
                          certDaysLeft === null
                            ? ""
                            : certDaysLeft < 0
                              ? "text-danger"
                              : certDaysLeft <= 7
                                ? "text-danger"
                                : certDaysLeft <= 14
                                  ? "text-[#f97316]"
                                  : "text-warning";

                        return (
                          <div key={portKey}>
                            <div className="flex items-center justify-between gap-2 py-0.5">
                              <div className="flex items-center gap-2 min-w-0">
                                <span className="font-mono text-xs text-text-primary shrink-0">
                                  {p.port}
                                </span>
                                <span className="text-xs text-text-muted uppercase shrink-0">
                                  {p.protocol}
                                </span>
                                {p.service && (
                                  <span className="text-xs text-text-secondary truncate">
                                    {p.service}
                                  </span>
                                )}
                                {hasBanner && (
                                  <button
                                    type="button"
                                    title="Show banner details"
                                    onClick={() => toggleBanner(p.port!, p.protocol ?? "tcp")}
                                    className={cn(
                                      "shrink-0 flex items-center gap-0.5 px-1 py-0.5 rounded text-[10px] font-medium transition-colors",
                                      isExpanded
                                        ? "bg-accent/20 text-accent"
                                        : "bg-surface-raised text-text-muted hover:text-text-secondary",
                                    )}
                                  >
                                    <FileText className="h-2.5 w-2.5" />
                                    {isExpanded ? (
                                      <ChevronDown className="h-2.5 w-2.5" />
                                    ) : (
                                      <ChevronRight className="h-2.5 w-2.5" />
                                    )}
                                  </button>
                                )}
                                {hasCert && !hasBanner && (
                                  <button
                                    type="button"
                                    title="Show certificate details"
                                    onClick={() => toggleBanner(p.port!, p.protocol ?? "tcp")}
                                    className={cn(
                                      "shrink-0 flex items-center gap-0.5 px-1 py-0.5 rounded text-[10px] font-medium transition-colors",
                                      isExpanded
                                        ? "bg-accent/20 text-accent"
                                        : certUrgent
                                          ? "bg-warning/10 text-warning hover:bg-warning/20"
                                          : "bg-surface-raised text-text-muted hover:text-text-secondary",
                                    )}
                                  >
                                    <ShieldCheck className="h-2.5 w-2.5" />
                                    {isExpanded ? (
                                      <ChevronDown className="h-2.5 w-2.5" />
                                    ) : (
                                      <ChevronRight className="h-2.5 w-2.5" />
                                    )}
                                  </button>
                                )}
                              </div>
                              <span className="text-xs text-text-muted whitespace-nowrap shrink-0">
                                {p.last_seen
                                  ? formatRelativeTime(p.last_seen)
                                  : "—"}
                              </span>
                            </div>

                            {/* Inline banner/cert expansion */}
                            {isExpanded && (hasBanner || hasCert) && (
                              <div className="ml-4 mb-1 mt-0.5 text-xs bg-surface-raised rounded p-2 border border-border/50 space-y-1">
                                {banner && (
                                  <>
                                    {(banner.service || banner.version) && (
                                      <div className="flex items-center gap-2 flex-wrap">
                                        {banner.service && (
                                          <span className="text-text-secondary font-medium">
                                            {banner.service}
                                          </span>
                                        )}
                                        {banner.version && (
                                          <span className="text-text-muted">
                                            {banner.version}
                                          </span>
                                        )}
                                      </div>
                                    )}
                                    {banner.raw_banner && (() => {
                                      const rawKey = `raw-${portKey}`;
                                      const isRawExpanded = expandedRawBanners.has(rawKey);
                                      const maxLen = 200;
                                      const truncated =
                                        banner.raw_banner.length > maxLen &&
                                        !isRawExpanded;
                                      return (
                                        <div>
                                          <p className="font-mono text-[11px] text-text-muted break-all whitespace-pre-wrap">
                                            {truncated
                                              ? banner.raw_banner.slice(0, maxLen) + "…"
                                              : banner.raw_banner}
                                          </p>
                                          {banner.raw_banner.length > maxLen && (
                                            <button
                                              type="button"
                                              onClick={() => toggleRawBanner(rawKey)}
                                              className="text-[10px] text-accent hover:underline mt-0.5"
                                            >
                                              {isRawExpanded ? "show less" : "show more"}
                                            </button>
                                          )}
                                        </div>
                                      );
                                    })()}
                                  </>
                                )}
                                {cert && (
                                  <div className="space-y-0.5">
                                    {cert.subject_cn && (
                                      <div className="flex gap-2">
                                        <span className="text-text-muted w-14 shrink-0">CN</span>
                                        <span className="text-text-secondary break-all">
                                          {cert.subject_cn}
                                        </span>
                                      </div>
                                    )}
                                    {cert.not_after && (
                                      <div className="flex gap-2">
                                        <span className="text-text-muted w-14 shrink-0">
                                          Expires
                                        </span>
                                        <span
                                          className={cn(
                                            certDaysLeft !== null && certDaysLeft <= 30
                                              ? certColor
                                              : "text-text-secondary",
                                          )}
                                        >
                                          {new Date(cert.not_after).toLocaleDateString()}
                                          {certDaysLeft !== null &&
                                            (certDaysLeft < 0
                                              ? " (expired)"
                                              : ` (${certDaysLeft}d)`)}
                                        </span>
                                      </div>
                                    )}
                                    {cert.tls_version && (
                                      <div className="flex gap-2">
                                        <span className="text-text-muted w-14 shrink-0">
                                          TLS
                                        </span>
                                        <span className="text-text-secondary">
                                          {cert.tls_version}
                                        </span>
                                      </div>
                                    )}
                                  </div>
                                )}
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}

                  {!isLoading && otherPorts.length > 0 && (
                    <>
                      <h3 className="text-xs font-medium text-text-primary mt-5 mb-3">
                        {`Closed / Filtered (${otherPorts.length})`}
                      </h3>
                      <div className="flex flex-wrap gap-1.5">
                        {otherPorts.map((p) => (
                          <span
                            key={`${p.port}-${p.protocol}`}
                            title={`${p.protocol} · ${p.state} · last seen ${p.last_seen ? formatRelativeTime(p.last_seen) : "unknown"}`}
                            className="inline-block px-2 py-0.5 rounded bg-surface-raised text-xs font-mono text-text-muted border border-border"
                          >
                            {p.port}
                          </span>
                        ))}
                      </div>
                    </>
                  )}
                </>
              );
            })()}
          </section>

          {/* DNS Records */}
          {(h.dns_records ?? []).length > 0 && (
            <section>
              <h3 className="text-xs font-medium text-text-primary mb-3">
                DNS Records
              </h3>
              <div className="space-y-1">
                {(h.dns_records ?? []).map((rec) => (
                  <div
                    key={rec.id}
                    className="flex items-start gap-2 text-xs py-0.5"
                  >
                    <span className="text-text-muted w-10 shrink-0 font-mono">
                      {rec.record_type}
                    </span>
                    <span className="text-text-secondary font-mono break-all">
                      {rec.value}
                    </span>
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* Port Banners */}
          {(h as HostWithDetails).banners &&
            (h as HostWithDetails).banners!.length > 0 && (
              <section>
                <h3 className="text-xs font-medium text-text-primary mb-3">
                  Port Banners
                </h3>
                <div className="space-y-1">
                  {(h as HostWithDetails).banners!.map((b) => (
                    <div
                      key={b.id}
                      className="text-xs border border-border/50 rounded p-2 space-y-0.5"
                    >
                      <div className="flex items-center gap-2">
                        <span className="font-mono font-semibold text-foreground">
                          {b.port}/{b.protocol}
                        </span>
                        {b.service && (
                          <span className="text-text-secondary">{b.service}</span>
                        )}
                        {b.version && (
                          <span className="text-text-muted">{b.version}</span>
                        )}
                      </div>
                      {b.raw_banner && (
                        <p className="text-text-muted font-mono break-all whitespace-pre-wrap text-[11px]">
                          {b.raw_banner}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </section>
            )}

          {/* TLS Certificates */}
          {(h as HostWithDetails).certificates &&
            (h as HostWithDetails).certificates!.length > 0 && (
              <section>
                <h3 className="text-xs font-medium text-text-primary mb-3">
                  TLS Certificates
                </h3>
                <div className="space-y-2">
                  {(h as HostWithDetails).certificates!.map((cert) => {
                    const expiry = cert.not_after
                      ? new Date(cert.not_after)
                      : null;
                    const daysLeft = expiry
                      ? Math.ceil(
                          (expiry.getTime() - Date.now()) / 86400000,
                        )
                      : null;
                    const expiryClass =
                      daysLeft === null
                        ? ""
                        : daysLeft < 0
                          ? "text-danger"
                          : daysLeft < 30
                            ? "text-warning"
                            : "text-text-secondary";

                    return (
                      <div
                        key={cert.id}
                        className="text-xs border border-border/50 rounded p-2 space-y-1"
                      >
                        <div className="flex items-center justify-between">
                          <span className="font-mono font-semibold text-foreground">
                            Port {cert.port}
                          </span>
                          {cert.tls_version && (
                            <span className="text-text-muted">
                              {cert.tls_version}
                            </span>
                          )}
                        </div>
                        {cert.subject_cn && (
                          <div className="text-text-secondary">
                            CN: {cert.subject_cn}
                          </div>
                        )}
                        {cert.issuer && (
                          <div className="text-text-muted">
                            Issuer: {cert.issuer}
                          </div>
                        )}
                        {cert.not_after && (
                          <div className={expiryClass}>
                            Expires:{" "}
                            {new Date(cert.not_after).toLocaleDateString()}
                            {daysLeft !== null &&
                              (daysLeft < 0
                                ? " (expired)"
                                : ` (${daysLeft}d)`)}
                          </div>
                        )}
                        {cert.sans && cert.sans.length > 0 && (
                          <div className="text-text-muted break-all">
                            SANs: {cert.sans.join(", ")}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </section>
            )}

          {/* SNMP Data */}
          {h.snmp_data && (
            <section>
              <h3 className="text-xs font-medium text-text-primary mb-3">
                SNMP Device Info
              </h3>
              <div className="space-y-1.5">
                {h.snmp_data.sys_name && (
                  <MetaRow label="System Name" value={h.snmp_data.sys_name} />
                )}
                {h.snmp_data.sys_descr && (
                  <MetaRow label="Description" value={h.snmp_data.sys_descr} />
                )}
                {h.snmp_data.sys_location && (
                  <MetaRow label="Location" value={h.snmp_data.sys_location} />
                )}
                {h.snmp_data.sys_contact && (
                  <MetaRow label="Contact" value={h.snmp_data.sys_contact} />
                )}
                {h.snmp_data.sys_uptime_cs != null && (
                  <MetaRow
                    label="Uptime"
                    value={formatSNMPUptime(h.snmp_data.sys_uptime_cs)}
                  />
                )}
              </div>
              {h.snmp_data.interfaces && h.snmp_data.interfaces.length > 0 && (
                <div className="mt-3">
                  <p className="text-xs text-text-muted mb-2">
                    Interfaces ({h.snmp_data.if_count ?? h.snmp_data.interfaces.length})
                  </p>
                  <div className="space-y-1">
                    {h.snmp_data.interfaces.map((iface, idx) => (
                      <div
                        key={idx}
                        className="flex items-center justify-between gap-2 py-0.5 text-xs"
                      >
                        <div className="flex items-center gap-2 min-w-0">
                          <span
                            className={`shrink-0 w-1.5 h-1.5 rounded-full ${
                              iface.status === "up"
                                ? "bg-green-500"
                                : "bg-surface-raised border border-border"
                            }`}
                          />
                          <span className="font-mono text-text-primary truncate">
                            {iface.name || `if${idx + 1}`}
                          </span>
                          {iface.mac && (
                            <span className="font-mono text-text-muted shrink-0">
                              {iface.mac}
                            </span>
                          )}
                        </div>
                        {iface.speed_mbps != null && iface.speed_mbps > 0 && (
                          <span className="text-text-muted shrink-0">
                            {iface.speed_mbps >= 1000
                              ? `${iface.speed_mbps / 1000} Gbps`
                              : `${iface.speed_mbps} Mbps`}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </section>
          )}

          {/* Scan History */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Scan History
            </h3>
            {hostScansLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="flex gap-2">
                    <Skeleton className="h-3 w-20 shrink-0" />
                    <Skeleton className="h-3 w-full" />
                  </div>
                ))}
              </div>
            ) : (hostScansData?.data ?? []).length === 0 ? (
              <p className="text-xs text-text-muted">
                No scan history for this host.
              </p>
            ) : (
              <div className="space-y-1">
                {(hostScansData?.data ?? []).map((scan) => (
                  <div
                    key={scan.id}
                    className="flex items-center justify-between gap-2 py-1 border-b border-border/40 last:border-0"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <StatusBadge status={scan.status ?? "unknown"} />
                      <span className="text-xs font-mono text-text-muted truncate">
                        {(scan.targets as string[] | undefined)?.join(", ") ??
                          "—"}
                      </span>
                    </div>
                    <span className="text-xs text-text-muted whitespace-nowrap shrink-0">
                      {scan.started_at
                        ? formatRelativeTime(scan.started_at)
                        : "—"}
                    </span>
                  </div>
                ))}
              </div>
            )}

            {/* Pagination */}
            {!hostScansLoading &&
              (hostScansData?.pagination?.total_pages ?? 0) > 1 && (
                <div className="flex justify-between items-center mt-2 pt-2">
                  <button
                    type="button"
                    disabled={scanHistoryPage <= 1}
                    onClick={() =>
                      setScanHistoryPage((p) => Math.max(1, p - 1))
                    }
                    className="text-xs text-text-muted hover:text-text-primary disabled:opacity-40"
                  >
                    ← Prev
                  </button>
                  <span className="text-xs text-text-muted">
                    {scanHistoryPage} /{" "}
                    {hostScansData?.pagination?.total_pages ?? 1}
                  </span>
                  <button
                    type="button"
                    disabled={
                      scanHistoryPage >=
                      (hostScansData?.pagination?.total_pages ?? 1)
                    }
                    onClick={() => setScanHistoryPage((p) => p + 1)}
                    className="text-xs text-text-muted hover:text-text-primary disabled:opacity-40"
                  >
                    Next →
                  </button>
                </div>
              )}
          </section>
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-border shrink-0 space-y-2">
          <div className="flex gap-2">
            <Button
              icon={<ScanLine className="h-3.5 w-3.5" />}
              onClick={() => {
                onClose();
                onScan(h.ip_address ?? "");
              }}
              className="flex-1 justify-center"
            >
              Scan
            </Button>
            <Button
              variant="secondary"
              icon={<Zap className="h-3.5 w-3.5" />}
              onClick={() => setShowSmartScanPreview(true)}
              disabled={hasPendingScan || smartScanStageLoading}
              title={
                hasPendingScan
                  ? "A scan is already pending for this host"
                  : "Run the next recommended scan for this host"
              }
              className="flex-1 justify-center"
            >
              Smart Scan
            </Button>
          </div>

          {deleteError && (
            <p className="text-[11px] text-danger">{deleteError}</p>
          )}

          {!showDeleteConfirm ? (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="w-full flex items-center justify-center gap-1.5 text-xs text-text-muted hover:text-danger transition-colors py-1"
            >
              <Trash2 className="h-3 w-3" />
              Delete host
            </button>
          ) : (
            <div className="flex items-center justify-center gap-2">
              <span className="text-xs text-text-muted">
                Permanently delete?
              </span>
              <Button
                variant="danger"
                onClick={() => void handleDeleteHost()}
                loading={isDeletingHost}
                className="text-xs h-6 px-2"
              >
                Confirm
              </Button>
              <Button
                variant="secondary"
                onClick={() => setShowDeleteConfirm(false)}
                className="text-xs h-6 px-2"
              >
                Cancel
              </Button>
            </div>
          )}
        </div>
      </div>

      {showSmartScanPreview && smartScanStage && (
        <HostSmartScanPreviewModal
          hostIp={h.ip_address ?? ""}
          stage={smartScanStage}
          isPending={isSmartScanPending}
          onConfirm={() => void handleSmartScanConfirm()}
          onClose={() => setShowSmartScanPreview(false)}
        />
      )}
    </>
  );
}

type HostStatus = "all" | "up" | "down" | "unknown";

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
          <td className="py-3 pl-4 pr-2 w-8">
            <Skeleton className="h-3 w-3 rounded" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3.5 w-28" />
          </td>
          {colVis.hostname && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-36" />
            </td>
          )}
          <td className="py-3 pr-4">
            <Skeleton className="h-5 w-14" />
          </td>
          {colVis.os && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-20" />
            </td>
          )}
          {colVis.mac && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-32" />
            </td>
          )}
          {colVis.vendor && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-24" />
            </td>
          )}
          {colVis.ports && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-20" />
            </td>
          )}
          {colVis.tags && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-24" />
            </td>
          )}
          {colVis.last_seen && (
            <td className="py-3 pr-4">
              <Skeleton className="h-3.5 w-8" />
            </td>
          )}
          {colVis.scans && (
            <td className="py-3">
              <Skeleton className="h-3.5 w-8" />
            </td>
          )}
        </tr>
      ))}
    </>
  );
}

// ── OS family filter options ───────────────────────────────────────────────

const OS_FAMILIES = ["Linux", "Windows", "macOS", "FreeBSD", "iOS", "Android"];

const HOST_COLUMNS: ColumnDef[] = [
  { key: "ip", label: "IP Address", alwaysVisible: true },
  { key: "hostname", label: "Hostname" },
  { key: "status", label: "Status", alwaysVisible: true },
  { key: "os", label: "OS" },
  { key: "mac", label: "MAC Address" },
  { key: "vendor", label: "Vendor" },
  { key: "ports", label: "Open Ports" },
  { key: "tags", label: "Tags" },
  { key: "last_seen", label: "Last Seen" },
  { key: "scans", label: "Scans" },
];

export function HostsPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<HostStatus>("all");
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [sortBy, setSortBy] = useState("last_seen");
  const [sortOrder, setSortOrder] = useState<SortOrder>("desc");
  const [osFilter, setOsFilter] = useState("");
  const [vendorFilter, setVendorFilter] = useState("");
  const [scanIP, setScanIP] = useState<string | null>(null);
  const [selectedHost, setSelectedHost] = useState<HostResponse | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkScanIPs, setBulkScanIPs] = useState<string[] | null>(null);
  const [bulkGroupOpen, setBulkGroupOpen] = useState(false);
  const [colVis, setColVis] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(HOST_COLUMNS.map((c) => [c.key, true])),
  );
  const [showFilterBuilder, setShowFilterBuilder] = useState(false);
  const [activeFilter, setActiveFilter] = useState<FilterGroup | null>(null);
  const { mutateAsync: bulkDeleteHosts, isPending: isBulkDeleting } =
    useBulkDeleteHosts();
  const { mutateAsync: bulkAddToGroup, isPending: isBulkAddingToGroup } =
    useAddHostsToGroup();
  const { data: allTags = [] } = useTags();
  const { data: allGroups = [] } = useGroups();
  const { toast } = useToast();
  const navigate = useNavigate();
  const search = useSearch({ from: "/hosts" });

  // Initialise active filter from the URL ?filter= param on first render
  useEffect(() => {
    const encoded = search.filter;
    if (encoded) {
      const expr = deserializeFilter(encoded);
      if (expr && "op" in expr && "conditions" in expr) {
        setActiveFilter(expr as FilterGroup);
        setShowFilterBuilder(true);
      }
    }
    // Only run once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Debounce search input ~300ms
  // Use the functional updater form to compare against the previous debounced
  // value — this prevents the timer from clearing selection on mount (when both
  // searchInput and debouncedSearch are still "").
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch((prev) => {
        if (prev !== searchInput) {
          setSelectedIds(new Set());
        }
        return searchInput;
      });
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Reset page when filter changes
  const handleStatusChange = useCallback((value: HostStatus) => {
    setStatusFilter(value);
    setPage(1);
    setSelectedIds(new Set());
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

  const toggleSelect = useCallback((id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  const toggleCol = useCallback((key: string) => {
    const col = HOST_COLUMNS.find((c) => c.key === key);
    if (col?.alwaysVisible) return;
    setColVis((prev) => ({ ...prev, [key]: !prev[key] }));
  }, []);

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    sort_by: sortBy,
    sort_order: sortOrder,
    ...(statusFilter !== "all" ? { status: statusFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
    ...(osFilter ? { os: osFilter } : {}),
    ...(vendorFilter ? { vendor: vendorFilter } : {}),
    // Advanced filter — send the raw JSON expression to the backend
    ...(activeFilter ? { filter: JSON.stringify(activeFilter) } : {}),
  };

  const handleApplyFilter = useCallback(
    (filter: FilterGroup | null) => {
      setActiveFilter(filter);
      setPage(1);
      setSelectedIds(new Set());
      // Persist to URL so the filter is shareable
      void navigate({
        to: "/hosts",
        search: { filter: serializeFilter(filter) },
        replace: true,
      });
    },
    [navigate],
  );

  const { data, isLoading, isError } = useHosts(queryParams);

  const hosts = useMemo(() => data?.data ?? [], [data]);
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 0;

  const { containerProps, isFocused, setFocusedIndex } = useTableKeyNav({
    items: hosts,
    onActivate: (host) => setSelectedHost(host),
    onSelect: (host) => {
      const id = host.id ?? "";
      toggleSelect(id, !selectedIds.has(id));
    },
    onEscape: () => setSelectedHost(null),
  });

  const visibleColCount =
    HOST_COLUMNS.filter((c) => colVis[c.key] !== false).length + 2;

  // Reset keyboard focus when page/filters change
  useEffect(() => {
    setFocusedIndex(-1);
  }, [
    page,
    statusFilter,
    debouncedSearch,
    sortBy,
    sortOrder,
    osFilter,
    vendorFilter,
    setFocusedIndex,
  ]);

  // Clamp page back when a filter/search change reduces total_pages below current page.
  if (!isLoading && totalPages > 0 && page > totalPages) {
    setPage(totalPages);
  }

  const toggleSelectAll = useCallback(() => {
    if (selectedIds.size === hosts.length && hosts.length > 0) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(hosts.map((h) => h.id ?? "")));
    }
  }, [selectedIds, hosts]);

  function handleScanSelected() {
    const ips = hosts
      .filter((h) => selectedIds.has(h.id ?? ""))
      .map((h) => h.ip_address ?? "")
      .filter(Boolean);
    if (ips.length === 0) return;
    setBulkScanIPs(ips);
  }

  async function handleBulkDelete() {
    const ids = Array.from(selectedIds);
    try {
      const result = await bulkDeleteHosts(ids);
      setSelectedIds(new Set());
      toast.success(
        `Deleted ${result?.deleted ?? ids.length} host${(result?.deleted ?? ids.length) !== 1 ? "s" : ""}`,
      );
      // Close the detail panel if the selected host was deleted
      if (selectedHost && selectedIds.has(selectedHost.id ?? "")) {
        setSelectedHost(null);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Bulk delete failed.");
    }
  }

  async function handleBulkAddToGroup(groupId: string) {
    setBulkGroupOpen(false);
    const ids = Array.from(selectedIds);
    try {
      const result = await bulkAddToGroup({ groupId, hostIds: ids });
      toast.success(`Added ${result?.added ?? ids.length} host${ids.length !== 1 ? "s" : ""} to group.`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add hosts to group.");
    }
  }

  return (
    <>
      <div className="space-y-4">
        {/* Filter bar */}
        <div className="flex flex-col sm:flex-row gap-3 flex-wrap">
          {/* Search input */}
          <div className="relative flex-1 min-w-40 max-w-sm">
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

          {/* OS family select */}
          <select
            value={osFilter}
            onChange={(e) => {
              setOsFilter(e.target.value);
              setPage(1);
            }}
            className={cn(
              "px-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
            aria-label="Filter by OS"
          >
            <option value="">All OS</option>
            {OS_FAMILIES.map((os) => (
              <option key={os} value={os}>
                {os}
              </option>
            ))}
          </select>

          {/* Vendor filter */}
          <input
            type="text"
            placeholder="Filter by vendor…"
            value={vendorFilter}
            onChange={(e) => {
              setVendorFilter(e.target.value);
              setPage(1);
            }}
            className={cn(
              "px-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary placeholder:text-text-muted",
              "focus:outline-none focus:ring-1 focus:ring-border",
              "min-w-36",
            )}
            aria-label="Filter by vendor"
          />

          <div className="flex items-center gap-2 sm:ml-auto">
            {/* Advanced filter toggle */}
            <button
              type="button"
              aria-label="Advanced filter"
              aria-pressed={showFilterBuilder}
              onClick={() => setShowFilterBuilder((s) => !s)}
              className={cn(
                "flex items-center gap-1 px-2 py-1.5 rounded border text-xs transition-colors",
                showFilterBuilder || activeFilter
                  ? "border-accent text-accent bg-accent/10"
                  : "border-border text-text-muted hover:text-text-primary hover:bg-surface-raised",
              )}
            >
              <SlidersHorizontal className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">Filter</span>
              {activeFilter && (
                <span className="ml-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-accent text-white text-[10px] font-bold leading-none">
                  {activeFilter.conditions.length}
                </span>
              )}
            </button>

            <Button
              onClick={() => setScanIP("")}
              icon={<ScanLine className="h-3.5 w-3.5" />}
            >
              New scan
            </Button>
            <ColumnToggle
              columns={HOST_COLUMNS}
              visibility={colVis}
              onToggle={toggleCol}
            />
          </div>
        </div>

        {/* Active filter chip */}
        {activeFilter && !showFilterBuilder && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-xs text-text-muted">Active filter:</span>
            <div className="flex items-center gap-1 px-2 py-0.5 rounded-full bg-accent/15 border border-accent/30 text-xs text-accent">
              <span>
                {activeFilter.op} — {activeFilter.conditions.length} condition
                {activeFilter.conditions.length !== 1 ? "s" : ""}
              </span>
              <button
                type="button"
                aria-label="Clear filter"
                onClick={() => handleApplyFilter(null)}
                className="ml-1 hover:text-accent/60 transition-colors"
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          </div>
        )}

        {/* Filter builder panel */}
        {showFilterBuilder && (
          <FilterBuilder
            value={activeFilter}
            onApply={handleApplyFilter}
            tagSuggestions={allTags}
            groupOptions={allGroups.map((g) => ({ id: g.id, name: g.name }))}
          />
        )}

        {/* Bulk action bar */}
        {selectedIds.size > 0 && (
          <div className="flex items-center gap-3 px-4 py-2 rounded-lg border border-border bg-surface-raised text-xs flex-wrap">
            <span className="text-text-secondary font-medium">
              {selectedIds.size} selected
            </span>
            <Button
              icon={<Play className="h-3.5 w-3.5" />}
              onClick={handleScanSelected}
              className="text-xs h-7 px-2"
            >
              Scan selected
            </Button>
            {allGroups.length > 0 && (
              <div className="relative">
                <Button
                  icon={<Plus className="h-3.5 w-3.5" />}
                  onClick={() => setBulkGroupOpen((o) => !o)}
                  loading={isBulkAddingToGroup}
                  className="text-xs h-7 px-2"
                >
                  Add to group
                </Button>
                {bulkGroupOpen && (
                  <div className="absolute left-0 top-full mt-1 z-30 w-44 bg-surface border border-border rounded-md shadow-lg py-1 max-h-48 overflow-y-auto">
                    {allGroups.map((g) => (
                      <button
                        key={g.id}
                        type="button"
                        onClick={() => void handleBulkAddToGroup(g.id)}
                        className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-raised flex items-center gap-2"
                      >
                        <span
                          className="h-2 w-2 rounded-full shrink-0"
                          style={{ backgroundColor: g.color ?? "#64748b" }}
                        />
                        <span className="truncate">{g.name}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
            <Button
              variant="danger"
              icon={<Trash2 className="h-3.5 w-3.5" />}
              loading={isBulkDeleting}
              onClick={() => void handleBulkDelete()}
              className="text-xs h-7 px-2"
            >
              Delete selected
            </Button>
            <button
              type="button"
              onClick={() => setSelectedIds(new Set())}
              className="text-text-muted hover:text-text-primary transition-colors"
            >
              Clear
            </button>
          </div>
        )}

        {/* Table card */}
        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          {/* Keyboard-navigable container */}
          <div
            {...containerProps}
            role="region"
            className="focus:outline-none"
            aria-label="Hosts table"
          >
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-border bg-surface">
                    <th className="py-3 pl-4 pr-2 w-8">
                      <input
                        type="checkbox"
                        aria-label="Select all hosts"
                        checked={
                          hosts.length > 0 && selectedIds.size === hosts.length
                        }
                        ref={(el) => {
                          if (el)
                            el.indeterminate =
                              selectedIds.size > 0 &&
                              selectedIds.size < hosts.length;
                        }}
                        onChange={toggleSelectAll}
                        className="rounded border-border cursor-pointer accent-accent"
                      />
                    </th>
                    <SortHeader
                      label="IP Address"
                      column="ip_address"
                      sortBy={sortBy}
                      sortOrder={sortOrder}
                      onSort={handleSort}
                      className="px-4"
                    />
                    {colVis.hostname && (
                      <SortHeader
                        label="Hostname"
                        column="hostname"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    <SortHeader
                      label="Status"
                      column="status"
                      sortBy={sortBy}
                      sortOrder={sortOrder}
                      onSort={handleSort}
                    />
                    {colVis.os && (
                      <SortHeader
                        label="OS"
                        column="os_family"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    {colVis.mac && (
                      <th className="text-left font-medium text-text-muted py-3 pr-4 whitespace-nowrap">
                        MAC Address
                      </th>
                    )}
                    {colVis.vendor && (
                      <SortHeader
                        label="Vendor"
                        column="vendor"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    {colVis.ports && (
                      <SortHeader
                        label="Open Ports"
                        column="open_ports"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    {colVis.tags && (
                      <th className="text-left font-medium text-text-muted py-3 pr-4 whitespace-nowrap">
                        Tags
                      </th>
                    )}
                    {colVis.last_seen && (
                      <SortHeader
                        label="Last Seen"
                        column="last_seen"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    {colVis.scans && (
                      <SortHeader
                        label="Scans"
                        column="scan_count"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      />
                    )}
                    <th className="py-3" />
                  </tr>
                </thead>
                <tbody>
                  {isError ? (
                    <tr>
                      <td
                        colSpan={visibleColCount}
                        className="py-10 text-center text-xs text-danger"
                      >
                        Failed to load hosts.
                      </td>
                    </tr>
                  ) : isLoading ? (
                    <SkeletonRows count={8} colVis={colVis} />
                  ) : hosts.length === 0 ? (
                    <tr>
                      <td
                        colSpan={visibleColCount}
                        className="py-10 text-center text-xs text-text-muted"
                      >
                        No hosts found.
                      </td>
                    </tr>
                  ) : (
                    hosts.map((host, idx) => (
                      <tr
                        key={host.id}
                        onClick={() => {
                          setSelectedHost(host);
                          setFocusedIndex(idx);
                        }}
                        className={cn(
                          "border-b border-border/50 last:border-0 hover:bg-surface-raised/50 transition-colors cursor-pointer",
                          isFocused(idx) &&
                            "ring-1 ring-inset ring-accent/60 bg-surface-raised/40",
                        )}
                      >
                        <td
                          className="py-3 pl-4 pr-2"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <input
                            type="checkbox"
                            aria-label={`Select ${host.ip_address ?? "host"}`}
                            checked={selectedIds.has(host.id ?? "")}
                            onChange={(e) =>
                              toggleSelect(host.id ?? "", e.target.checked)
                            }
                            className="rounded border-border cursor-pointer accent-accent"
                          />
                        </td>
                        <td className="py-3 px-4 pr-4 font-mono text-text-primary whitespace-nowrap">
                          {host.ip_address ?? "—"}
                        </td>
                        {colVis.hostname && (
                          <td className="py-3 pr-4 text-text-secondary">
                            {host.hostname ?? "—"}
                          </td>
                        )}
                        <td className="py-3 pr-4">
                          <StatusBadge status={host.status ?? "unknown"} />
                        </td>
                        {colVis.os && (
                          <td className="py-3 pr-4 text-text-secondary whitespace-nowrap">
                            {(host as HostWithDetails).os_family ? (
                              <span
                                title={
                                  (host as HostWithDetails).os_name ?? undefined
                                }
                              >
                                {(host as HostWithDetails).os_family}
                              </span>
                            ) : (
                              <span className="text-text-muted">—</span>
                            )}
                          </td>
                        )}
                        {colVis.mac && (
                          <td className="py-3 pr-4 font-mono text-text-muted whitespace-nowrap">
                            {host.mac_address ?? "—"}
                          </td>
                        )}
                        {colVis.vendor && (
                          <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                            {(host as HostWithDetails).vendor ?? "—"}
                          </td>
                        )}
                        {colVis.ports && (
                          <td className="py-3 pr-4 tabular-nums text-text-secondary">
                            {(() => {
                              const count = (host as HostWithDetails)
                                .total_ports;
                              return count != null && count > 0 ? (
                                count
                              ) : (
                                <span className="text-text-muted">—</span>
                              );
                            })()}
                          </td>
                        )}
                        {colVis.tags && (
                          <td
                            className="py-3 pr-4"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <div className="flex flex-wrap gap-1">
                              {((host as HostWithDetails).tags ?? []).length === 0 ? (
                                <span className="text-text-muted">—</span>
                              ) : (
                                ((host as HostWithDetails).tags ?? []).map((tag) => (
                                  <button
                                    key={tag}
                                    type="button"
                                    onClick={() =>
                                      handleApplyFilter({
                                        op: "AND",
                                        conditions: [
                                          {
                                            field: "tags",
                                            cmp: "contains",
                                            value: tag,
                                          },
                                        ],
                                      })
                                    }
                                    className="inline-flex items-center px-1.5 py-0.5 rounded-full text-[11px] font-medium bg-accent/15 text-accent border border-accent/20 hover:bg-accent/25 transition-colors cursor-pointer"
                                  >
                                    {tag}
                                  </button>
                                ))
                              )}
                            </div>
                          </td>
                        )}
                        {colVis.last_seen && (
                          <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                            {host.last_seen
                              ? formatRelativeTime(host.last_seen)
                              : "—"}
                          </td>
                        )}
                        {colVis.scans && (
                          <td className="py-3 pr-4 text-text-secondary tabular-nums">
                            {host.scan_count ?? "—"}
                          </td>
                        )}
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

      {bulkScanIPs !== null && (
        <RunScanModal
          initialTargets={bulkScanIPs}
          onClose={() => setBulkScanIPs(null)}
          onSubmitted={() => {
            setBulkScanIPs(null);
            setSelectedIds(new Set());
            void navigate({ to: "/scans" });
          }}
        />
      )}

      {selectedHost && (
        <HostDetailPanel
          host={selectedHost}
          onClose={() => setSelectedHost(null)}
          onScan={(ip) => setScanIP(ip)}
          allTags={allTags}
          onTagFilter={(tag) => {
            handleApplyFilter({
              op: "AND",
              conditions: [{ field: "tags", cmp: "contains", value: tag }],
            });
            setShowFilterBuilder(true);
          }}
          allGroups={allGroups}
        />
      )}
    </>
  );
}
