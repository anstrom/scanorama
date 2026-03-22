import { useState, useId } from "react";
import { X, Loader2 } from "lucide-react";
import { Button } from "./button";
import {
  useCreateNetworkExclusion,
  useCreateGlobalExclusion,
} from "../api/hooks/use-networks";
import { cn } from "../lib/utils";

export interface AddExclusionModalProps {
  /**
   * If provided, the exclusion will be scoped to this network.
   * If omitted, a global exclusion will be created.
   */
  networkId?: string;
  onClose: () => void;
  onCreated?: () => void;
}

export function AddExclusionModal({
  networkId,
  onClose,
  onCreated,
}: AddExclusionModalProps) {
  const id = useId();

  const [cidr, setCidr] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createNetworkExclusion, isPending: isNetworkPending } =
    useCreateNetworkExclusion();
  const { mutateAsync: createGlobalExclusion, isPending: isGlobalPending } =
    useCreateGlobalExclusion();

  const isPending = isNetworkPending || isGlobalPending;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedCidr = cidr.trim();

    if (!trimmedCidr) {
      setError("CIDR block is required (e.g. 192.168.1.128/25).");
      return;
    }

    if (!/^[\da-fA-F:./]+\/\d+$/.test(trimmedCidr)) {
      setError(
        "CIDR block must be in valid notation (e.g. 192.168.1.128/25 or 10.0.1.0/24).",
      );
      return;
    }

    const body = {
      excluded_cidr: trimmedCidr,
      reason: reason.trim() || undefined,
    };

    try {
      if (networkId) {
        await createNetworkExclusion({ networkId, body });
      } else {
        await createGlobalExclusion(body);
      }
      onCreated?.();
      onClose();
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      setError(apiErr.message ?? apiErr.error ?? "Failed to add exclusion.");
    }
  }

  const isGlobal = !networkId;

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
            {isGlobal ? "Add Global Exclusion" : "Add Exclusion"}
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
          {isGlobal && (
            <p className="text-xs text-text-muted leading-relaxed">
              Global exclusions apply to all networks — hosts in these CIDR
              ranges will never be scanned or discovered.
            </p>
          )}

          {/* CIDR */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-cidr`}
              className="block text-xs font-medium text-text-primary"
            >
              CIDR block
            </label>
            <input
              id={`${id}-cidr`}
              type="text"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder="192.168.1.128/25"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
            <p className="text-xs text-text-muted">
              IPv4 or IPv6 CIDR notation (e.g. 192.168.1.128/25).
            </p>
          </div>

          {/* Reason */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-reason`}
              className="block text-xs font-medium text-text-primary"
            >
              Reason{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              id={`${id}-reason`}
              type="text"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Reserved for printers"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
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
              {isPending ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Adding…
                </>
              ) : (
                "Add exclusion"
              )}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
