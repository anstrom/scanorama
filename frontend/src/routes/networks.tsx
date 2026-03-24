import { useState, useCallback } from "react";
import {
  Network,
  Plus,
  Trash2,
  X,
  Pencil,
  Check,
  Ban,
  Play,
} from "lucide-react";
import { Button } from "../components/button";
import {
  useNetworks,
  useNetworkExclusions,
  useEnableNetwork,
  useDisableNetwork,
  useDeleteNetwork,
  useDeleteExclusion,
  useStartNetworkDiscovery,
} from "../api/hooks/use-networks";
import { Skeleton, PaginationBar } from "../components";
import { AddNetworkModal } from "../components/add-network-modal";
import { AddExclusionModal } from "../components/add-exclusion-modal";
import { EditNetworkModal } from "../components/edit-network-modal";
import { useToast } from "../components/toast-provider";
import { formatRelativeTime, formatAbsoluteTime, cn } from "../lib/utils";
import type { components } from "../api/types";

type NetworkResponse = components["schemas"]["docs.NetworkResponse"];
type NetworkExclusionResponse =
  components["schemas"]["docs.NetworkExclusionResponse"];

const PAGE_SIZE = 25;

// ── Helpers ───────────────────────────────────────────────────────────────────

const DISCOVERY_METHOD_LABELS: Record<string, string> = {
  ping: "Ping",
  tcp: "TCP",
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

function SkeletonRows() {
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

// ── Meta row helper ───────────────────────────────────────────────────────────

function MetaRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-xs">
      <span className="text-text-muted w-32 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value ?? "—"}</span>
    </div>
  );
}

// ── Exclusions sub-section ────────────────────────────────────────────────────

function ExclusionsSection({ networkId }: { networkId: string }) {
  const [showAdd, setShowAdd] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const { toast } = useToast();

  const { data: exclusions, isLoading } = useNetworkExclusions(networkId);
  const { mutate: deleteExclusion, isPending: isDeleting } =
    useDeleteExclusion();

  const list = exclusions ?? [];

  function handleDelete(id: string) {
    if (confirmDeleteId !== id) {
      setConfirmDeleteId(id);
      return;
    }
    deleteExclusion(id, {
      onSuccess: () => toast.success("Exclusion removed"),
      onError: (err) =>
        toast.error(err instanceof Error ? err.message : "Action failed."),
      onSettled: () => setConfirmDeleteId(null),
    });
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
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-full rounded" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <p className="text-xs text-text-muted italic">No exclusions defined.</p>
      ) : (
        <div className="space-y-1">
          {list.map((excl: NetworkExclusionResponse) => (
            <div
              key={excl.id}
              className="flex items-start gap-2 p-2 rounded bg-surface-raised group"
            >
              <div className="flex-1 min-w-0">
                <p className="text-xs font-mono text-text-primary truncate">
                  {excl.excluded_cidr ?? "—"}
                </p>
                {excl.reason && (
                  <p className="text-[11px] text-text-muted truncate mt-0.5">
                    {excl.reason}
                  </p>
                )}
              </div>

              {confirmDeleteId === excl.id ? (
                <div className="flex items-center gap-1 shrink-0">
                  <button
                    type="button"
                    onClick={() => setConfirmDeleteId(null)}
                    className="text-[11px] text-text-muted hover:text-text-secondary px-1"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(excl.id ?? "")}
                    disabled={isDeleting}
                    className="text-[11px] text-danger hover:text-danger/80 px-1"
                  >
                    Confirm
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => handleDelete(excl.id ?? "")}
                  aria-label={`Delete exclusion ${excl.excluded_cidr}`}
                  className="shrink-0 p-0.5 rounded text-text-muted opacity-0 group-hover:opacity-100 hover:text-danger transition-all"
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

// ── Network detail panel ──────────────────────────────────────────────────────

interface DetailPanelProps {
  network: NetworkResponse;
  onClose: () => void;
}

function NetworkDetailPanel({
  network: initialNetwork,
  onClose,
}: DetailPanelProps) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);

  const { toast } = useToast();

  const { mutateAsync: enableNetwork, isPending: isEnabling } =
    useEnableNetwork();
  const { mutateAsync: disableNetwork, isPending: isDisabling } =
    useDisableNetwork();
  const { mutateAsync: deleteNetwork, isPending: isDeleting } =
    useDeleteNetwork();
  const { mutateAsync: discoverNetwork, isPending: isDiscovering } =
    useStartNetworkDiscovery();

  const isTogglingActive = isEnabling || isDisabling;

  // Use initial network data; live updates happen via query cache invalidation
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
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : "Failed to start discovery.";
      toast.error(msg);
      setActionError(msg);
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

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
          {/* Network info */}
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
                value={n.host_count != null ? String(n.host_count) : undefined}
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

          {/* Exclusions */}
          {n.id && <ExclusionsSection networkId={n.id} />}
        </div>
      </div>

      {showEditModal && (
        <EditNetworkModal
          network={n}
          onClose={() => setShowEditModal(false)}
          onSaved={() => setShowEditModal(false)}
        />
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
  const [selectedNetwork, setSelectedNetwork] =
    useState<NetworkResponse | null>(null);
  const [showAddNetwork, setShowAddNetwork] = useState(false);

  // Debounce name search
  const debounceRef = {
    current: 0 as unknown as ReturnType<typeof setTimeout>,
  };
  const handleNameInput = useCallback(
    (value: string) => {
      setNameSearch(value);
      clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        setDebouncedName(value);
        setPage(1);
      }, 300);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    ...(showInactive ? { show_inactive: true } : {}),
    ...(debouncedName.trim() ? { name: debouncedName.trim() } : {}),
  };

  const { data, isLoading } = useNetworks(queryParams);

  const networks = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  function handleShowInactiveChange(checked: boolean) {
    setShowInactive(checked);
    setPage(1);
  }

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-3 flex-wrap">
        {/* Name search */}
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

        {/* Show inactive toggle */}
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

        {/* Spacer */}
        <div className="flex-1" />

        {/* Add network */}
        <Button
          onClick={() => setShowAddNetwork(true)}
          className="text-xs h-7 px-3"
        >
          <Plus className="h-3 w-3 mr-1" />
          Add network
        </Button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded border border-border">
        <table className="w-full text-xs border-collapse min-w-[640px]">
          <thead>
            <tr className="bg-surface-raised border-b border-border text-left">
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Name
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                CIDR
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap text-right">
                Hosts
              </th>
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
              <SkeletonRows />
            ) : networks.length === 0 ? (
              <tr>
                <td
                  colSpan={7}
                  className="px-4 py-10 text-center text-text-muted"
                >
                  No networks found.
                </td>
              </tr>
            ) : (
              networks.map((network) => (
                <tr
                  key={network.id}
                  onClick={() => setSelectedNetwork(network)}
                  className={cn(
                    "border-b border-border cursor-pointer transition-colors",
                    "hover:bg-surface-raised",
                    selectedNetwork?.id === network.id && "bg-accent/8",
                  )}
                >
                  <td className="px-4 py-2.5 text-text-primary font-medium truncate max-w-[180px]">
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
      {!isLoading && networks.length > 0 && totalPages > 1 && (
        <PaginationBar
          page={page}
          totalPages={totalPages}
          onPrev={() => setPage((p) => Math.max(1, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
        />
      )}

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
