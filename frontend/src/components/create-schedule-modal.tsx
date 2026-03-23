import { useState, useId } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
import { useCreateSchedule } from "../api/hooks/use-schedules";
import { useNetworks } from "../api/hooks/use-networks";
import { cn, describeCron } from "../lib/utils";

export interface CreateScheduleModalProps {
  onClose: () => void;
  onCreated?: () => void;
}

export function CreateScheduleModal({
  onClose,
  onCreated,
}: CreateScheduleModalProps) {
  const id = useId();

  const [name, setName] = useState("");
  const [cronExpr, setCronExpr] = useState("0 2 * * *");
  const [networkId, setNetworkId] = useState("");
  const [type, setType] = useState<"scan" | "discovery">("scan");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createSchedule, isPending } = useCreateSchedule();
  const { data: networksData } = useNetworks({ page: 1, page_size: 100 });
  const networks = networksData?.data ?? [];

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    const trimmedCron = cronExpr.trim();

    if (!trimmedName) {
      setError("Name is required.");
      return;
    }

    if (!trimmedCron) {
      setError("Cron expression is required.");
      return;
    }

    if (!networkId) {
      setError("A network is required.");
      return;
    }

    try {
      await createSchedule({
        name: trimmedName,
        cron_expr: trimmedCron,
        type,
        network_id: networkId,
        enabled,
      });
      onCreated?.();
      onClose();
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      setError(apiErr.message ?? apiErr.error ?? "Failed to create schedule.");
    }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        className={cn(
          "fixed z-50 inset-0 m-auto",
          "w-full max-w-md h-fit",
          "bg-surface border border-border rounded-lg shadow-xl",
          "flex flex-col",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border">
          <h2
            id={`${id}-title`}
            className="text-sm font-semibold text-text-primary"
          >
            Create Schedule
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="px-5 py-4 space-y-5">
          {/* Name */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-name`}
              className="block text-xs font-medium text-text-primary"
            >
              Name
            </label>
            <input
              id={`${id}-name`}
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Daily Security Scan"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Cron expression */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-cron`}
              className="block text-xs font-medium text-text-primary"
            >
              Cron expression
            </label>
            <input
              id={`${id}-cron`}
              type="text"
              value={cronExpr}
              onChange={(e) => setCronExpr(e.target.value)}
              placeholder="0 2 * * *"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
            <p className="text-xs text-text-muted">{describeCron(cronExpr)}</p>
          </div>

          {/* Type */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-type`}
              className="block text-xs font-medium text-text-primary"
            >
              Type
            </label>
            <select
              id={`${id}-type`}
              value={type}
              onChange={(e) => setType(e.target.value as "scan" | "discovery")}
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              <option value="scan">Scan</option>
              <option value="discovery">Discovery</option>
            </select>
          </div>

          {/* Network */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-network`}
              className="block text-xs font-medium text-text-primary"
            >
              Network
            </label>
            <select
              id={`${id}-network`}
              value={networkId}
              onChange={(e) => setNetworkId(e.target.value)}
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              <option value="">— Select a network —</option>
              {networks.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name} ({n.cidr})
                </option>
              ))}
            </select>
            {networks.length === 0 && (
              <p className="text-xs text-text-muted">
                No networks configured yet. Add a network first.
              </p>
            )}
          </div>

          {/* Enabled */}
          <div className="flex items-center gap-2">
            <input
              id={`${id}-enabled`}
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            <label
              htmlFor={`${id}-enabled`}
              className="text-xs text-text-primary"
            >
              Enable this schedule
            </label>
          </div>

          {/* Inline error */}
          {error && (
            <p role="alert" className="text-xs text-danger">
              {error}
            </p>
          )}

          {/* Footer */}
          <div className="flex justify-end gap-2 pt-1">
            <Button variant="secondary" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={isPending}>
              Create schedule
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
