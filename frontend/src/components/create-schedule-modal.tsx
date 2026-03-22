import { useState, useId } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
import { useCreateSchedule } from "../api/hooks/use-schedules";
import { useProfiles } from "../api/hooks/use-profiles";
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
  const [targets, setTargets] = useState("");
  const [profileId, setProfileId] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createSchedule, isPending } = useCreateSchedule();
  const { data: profilesData } = useProfiles({ page: 1, page_size: 100 });
  const profiles = profilesData?.data ?? [];

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    const trimmedCron = cronExpr.trim();
    const targetList = targets
      .split(/[\s,]+/)
      .map((t) => t.trim())
      .filter(Boolean);

    if (!trimmedName) {
      setError("Name is required.");
      return;
    }

    if (!trimmedCron) {
      setError("Cron expression is required.");
      return;
    }

    if (targetList.length === 0) {
      setError("At least one target is required.");
      return;
    }

    try {
      await createSchedule({
        name: trimmedName,
        cron_expression: trimmedCron,
        targets: targetList,
        profile_id: profileId || undefined,
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

          {/* Targets */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-targets`}
              className="block text-xs font-medium text-text-primary"
            >
              Targets
            </label>
            <textarea
              id={`${id}-targets`}
              value={targets}
              onChange={(e) => setTargets(e.target.value)}
              placeholder="192.168.1.0/24, 10.0.0.0/8"
              rows={3}
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono resize-none",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
            <p className="text-xs text-text-muted">
              Comma or whitespace-separated CIDRs or IPs.
            </p>
          </div>

          {/* Profile */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-profile`}
              className="block text-xs font-medium text-text-primary"
            >
              Profile{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <select
              id={`${id}-profile`}
              value={profileId}
              onChange={(e) => setProfileId(e.target.value)}
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              <option value="">— No profile —</option>
              {profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
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
