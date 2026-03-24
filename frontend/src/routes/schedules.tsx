import { useState } from "react";
import { Plus, X, Pencil } from "lucide-react";
import { Button } from "../components/button";
import {
  useSchedules,
  useEnableSchedule,
  useDisableSchedule,
  useDeleteSchedule,
} from "../api/hooks/use-schedules";
import { Skeleton, PaginationBar } from "../components";
import { ScheduleFormModal } from "../components/create-schedule-modal";
import { useToast } from "../components/toast-provider";
import {
  formatRelativeTime,
  formatAbsoluteTime,
  cn,
  describeCron,
} from "../lib/utils";
import type { components } from "../api/types";

type ScheduleResponse = components["schemas"]["docs.ScheduleResponse"];

const PAGE_SIZE = 25;

type StatusFilter = "all" | "enabled" | "disabled";

// ── Badge ─────────────────────────────────────────────────────────────────────

function ScheduleEnabledBadge({ enabled }: { enabled: boolean }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium",
        enabled
          ? "bg-success/15 text-success"
          : "bg-text-muted/15 text-text-muted",
      )}
    >
      {enabled ? "enabled" : "disabled"}
    </span>
  );
}

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function SkeletonRows() {
  return (
    <>
      {Array.from({ length: 6 }).map((_, i) => (
        <tr key={i} className="border-b border-border">
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-32" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-40" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-5 w-16 rounded" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-28" />
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

// ── Detail panel ──────────────────────────────────────────────────────────────

interface ScheduleDetailPanelProps {
  schedule: ScheduleResponse;
  onClose: () => void;
}

function ScheduleDetailPanel({
  schedule: s,
  onClose,
}: ScheduleDetailPanelProps) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);

  const { toast } = useToast();

  const { mutateAsync: enableSchedule, isPending: isEnabling } =
    useEnableSchedule();
  const { mutateAsync: disableSchedule, isPending: isDisabling } =
    useDisableSchedule();
  const { mutateAsync: deleteSchedule, isPending: isDeleting } =
    useDeleteSchedule();

  const isToggling = isEnabling || isDisabling;
  const targetList = (s as unknown as { targets?: string[] }).targets ?? [];

  async function handleToggleEnabled() {
    setActionError(null);
    try {
      if (s.enabled) {
        await disableSchedule(s.id ?? "");
        toast.success("Schedule disabled");
      } else {
        await enableSchedule(s.id ?? "");
        toast.success("Schedule enabled");
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Action failed.";
      setActionError(msg);
      toast.error(msg);
    }
  }

  async function handleDelete() {
    setActionError(null);
    try {
      await deleteSchedule(s.id ?? "");
      toast.success("Schedule deleted");
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed.";
      setActionError(msg);
      setShowDeleteConfirm(false);
      toast.error(msg);
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
        aria-label="Schedule details"
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
            <p className="text-xs text-text-muted">Schedule</p>
            <div className="flex items-center gap-2 min-w-0">
              <h2 className="text-sm font-semibold text-text-primary truncate">
                {s.name ?? "—"}
              </h2>
              <ScheduleEnabledBadge enabled={s.enabled ?? false} />
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close panel"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors shrink-0 mt-0.5"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Action bar */}
        <div className="flex items-center gap-2 px-5 py-3 border-b border-border shrink-0">
          <Button
            variant="secondary"
            onClick={() => void handleToggleEnabled()}
            loading={isToggling}
            className="text-xs h-7 px-3"
          >
            {s.enabled ? "Disable" : "Enable"}
          </Button>

          <Button
            variant="secondary"
            onClick={() => setShowEditModal(true)}
            className="text-xs h-7 px-3"
          >
            <Pencil className="h-3 w-3 mr-1" />
            Edit
          </Button>

          {!showDeleteConfirm ? (
            <Button
              variant="danger"
              onClick={() => setShowDeleteConfirm(true)}
              className="text-xs h-7 px-3"
            >
              Delete
            </Button>
          ) : (
            <div className="flex items-center gap-2">
              <span className="text-xs text-text-muted">Confirm delete?</span>
              <Button
                variant="danger"
                onClick={() => void handleDelete()}
                loading={isDeleting}
                className="text-xs h-7 px-3"
              >
                Yes, delete
              </Button>
              <Button
                variant="secondary"
                onClick={() => setShowDeleteConfirm(false)}
                className="text-xs h-7 px-3"
              >
                Cancel
              </Button>
            </div>
          )}
        </div>

        {/* Inline error */}
        {actionError && (
          <div className="px-5 py-2 shrink-0">
            <p role="alert" className="text-xs text-danger">
              {actionError}
            </p>
          </div>
        )}

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {/* Cron */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-text-primary uppercase tracking-wide">
              Cron
            </h3>
            <MetaRow
              label="Expression"
              value={
                s.cron_expr ? (
                  <span title={s.cron_expr} className="font-mono">
                    {describeCron(s.cron_expr)}
                  </span>
                ) : (
                  "—"
                )
              }
            />
            <MetaRow
              label="Raw"
              value={
                s.cron_expr ? (
                  <span className="font-mono">{s.cron_expr}</span>
                ) : (
                  "—"
                )
              }
            />
          </section>

          {/* Schedule timings */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-text-primary uppercase tracking-wide">
              Schedule
            </h3>
            <MetaRow
              label="Next run"
              value={s.next_run ? formatRelativeTime(s.next_run) : "—"}
            />
            <MetaRow
              label="Last run"
              value={s.last_run ? formatRelativeTime(s.last_run) : "—"}
            />
          </section>

          {/* Targets */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-text-primary uppercase tracking-wide">
              Targets
            </h3>
            {targetList.length === 0 ? (
              <MetaRow label="Targets" value="—" />
            ) : (
              <div className="text-xs text-text-secondary font-mono space-y-1">
                {targetList.map((t, i) => (
                  <div key={i}>{t}</div>
                ))}
              </div>
            )}
          </section>

          {/* Profile */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-text-primary uppercase tracking-wide">
              Profile
            </h3>
            <MetaRow
              label="Profile ID"
              value={
                (s as unknown as { profile_id?: string }).profile_id ?? "None"
              }
            />
          </section>

          {/* Timestamps */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-text-primary uppercase tracking-wide">
              Timestamps
            </h3>
            <MetaRow
              label="Created"
              value={s.created_at ? formatAbsoluteTime(s.created_at) : "—"}
            />
            <MetaRow
              label="Updated"
              value={s.updated_at ? formatAbsoluteTime(s.updated_at) : "—"}
            />
          </section>
        </div>
      </div>

      {/* Edit modal */}
      {showEditModal && (
        <ScheduleFormModal
          mode="edit"
          initial={{
            id: s.id,
            name: s.name,
            cron_expr: s.cron_expr,
            network_id: s.network_id,
            type: s.type as "scan" | "discovery",
            enabled: s.enabled,
          }}
          onClose={() => setShowEditModal(false)}
          onSaved={() => setShowEditModal(false)}
        />
      )}
    </>
  );
}

// ── Schedules page ────────────────────────────────────────────────────────────

export function SchedulesPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [selectedSchedule, setSelectedSchedule] =
    useState<ScheduleResponse | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  const queryParams = {
    page,
    page_size: PAGE_SIZE,
    ...(statusFilter === "enabled" ? { enabled: true } : {}),
    ...(statusFilter === "disabled" ? { enabled: false } : {}),
  };

  const { data, isLoading } = useSchedules(queryParams);

  const schedules = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  function handleStatusFilterChange(value: string) {
    setStatusFilter(value as StatusFilter);
    setPage(1);
  }

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-3 flex-wrap">
        <select
          value={statusFilter}
          onChange={(e) => handleStatusFilterChange(e.target.value)}
          aria-label="Filter by status"
          className={cn(
            "px-3 py-1.5 text-xs rounded border border-border",
            "bg-surface text-text-primary",
            "focus:outline-none focus:ring-1 focus:ring-border",
          )}
        >
          <option value="all">All</option>
          <option value="enabled">Enabled</option>
          <option value="disabled">Disabled</option>
        </select>

        <div className="flex-1" />

        <Button
          onClick={() => setShowCreateModal(true)}
          className="text-xs h-7 px-3"
        >
          <Plus className="h-3 w-3 mr-1" />
          Create schedule
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
                Cron
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Next Run
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Last Run
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Status
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Network
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <SkeletonRows />
            ) : schedules.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-4 py-10 text-center text-text-muted"
                >
                  No schedules found.
                </td>
              </tr>
            ) : (
              schedules.map((schedule) => {
                const targetDisplay = schedule.network_name ?? "—";

                return (
                  <tr
                    key={schedule.id}
                    onClick={() => setSelectedSchedule(schedule)}
                    className={cn(
                      "border-b border-border cursor-pointer transition-colors",
                      "hover:bg-surface-raised",
                      selectedSchedule?.id === schedule.id && "bg-accent/8",
                    )}
                  >
                    <td className="px-4 py-2.5 text-text-primary font-medium truncate max-w-[180px]">
                      {schedule.name ?? "—"}
                    </td>
                    <td
                      className="px-4 py-2.5 text-text-secondary font-mono whitespace-nowrap"
                      title={schedule.cron_expr}
                    >
                      {schedule.cron_expr
                        ? describeCron(schedule.cron_expr)
                        : "—"}
                    </td>
                    <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                      {schedule.next_run
                        ? formatRelativeTime(schedule.next_run)
                        : "—"}
                    </td>
                    <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                      {schedule.last_run
                        ? formatRelativeTime(schedule.last_run)
                        : "—"}
                    </td>
                    <td className="px-4 py-2.5">
                      <ScheduleEnabledBadge
                        enabled={schedule.enabled ?? false}
                      />
                    </td>
                    <td className="px-4 py-2.5 text-text-secondary font-mono whitespace-nowrap">
                      {targetDisplay}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {!isLoading && schedules.length > 0 && totalPages > 1 && (
        <PaginationBar
          page={page}
          totalPages={totalPages}
          onPrev={() => setPage((p) => Math.max(1, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
        />
      )}

      {/* Schedule detail panel */}
      {selectedSchedule && (
        <ScheduleDetailPanel
          schedule={selectedSchedule}
          onClose={() => setSelectedSchedule(null)}
        />
      )}

      {/* Create schedule modal */}
      {showCreateModal && (
        <ScheduleFormModal
          mode="create"
          onClose={() => setShowCreateModal(false)}
        />
      )}
    </div>
  );
}
