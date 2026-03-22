import { useState, useId } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
import {
  useCreateDiscoveryJob,
  useStartDiscovery,
} from "../api/hooks/use-discovery";
import { cn } from "../lib/utils";

const METHODS = [
  { value: "tcp", label: "TCP" },
  { value: "icmp", label: "ICMP" },
  { value: "arp", label: "ARP" },
] as const;

type Method = (typeof METHODS)[number]["value"];

export interface CreateDiscoveryModalProps {
  onClose: () => void;
  onCreated?: () => void;
}

export function CreateDiscoveryModal({
  onClose,
  onCreated,
}: CreateDiscoveryModalProps) {
  const id = useId();

  const [name, setName] = useState("");
  const [network, setNetwork] = useState("");
  const [method, setMethod] = useState<Method>("tcp");
  const [startImmediately, setStartImmediately] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createDiscoveryJob, isPending: isCreating } =
    useCreateDiscoveryJob();
  const { mutateAsync: startDiscovery, isPending: isStarting } =
    useStartDiscovery();
  const isPending = isCreating || isStarting;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedNetwork = network.trim();
    if (!trimmedNetwork) {
      setError("Network / Target is required (e.g. 192.168.1.0/24).");
      return;
    }

    try {
      const result = await createDiscoveryJob({
        name: name.trim() || undefined,
        network: trimmedNetwork,
        method,
      });

      if (startImmediately && result?.id) {
        await startDiscovery(result.id);
      }

      onCreated?.();
      onClose();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to create discovery job.",
      );
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
            New Discovery Job
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
              Name{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              id={`${id}-name`}
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Network Discovery"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Network / Target */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-network`}
              className="block text-xs font-medium text-text-primary"
            >
              Network / Target
            </label>
            <input
              id={`${id}-network`}
              type="text"
              value={network}
              onChange={(e) => setNetwork(e.target.value)}
              placeholder="192.168.1.0/24"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
            <p className="text-xs text-text-muted">
              IPv4 or IPv6 CIDR notation (e.g. 192.168.1.0/24).
            </p>
          </div>

          {/* Method */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-method`}
              className="block text-xs font-medium text-text-primary"
            >
              Method
            </label>
            <select
              id={`${id}-method`}
              value={method}
              onChange={(e) => setMethod(e.target.value as Method)}
              aria-label="Select discovery method"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              {METHODS.map((m) => (
                <option key={m.value} value={m.value}>
                  {m.label}
                </option>
              ))}
            </select>
          </div>

          {/* Start immediately */}
          <div className="flex items-center gap-2">
            <input
              id={`${id}-start-immediately`}
              type="checkbox"
              checked={startImmediately}
              onChange={(e) => setStartImmediately(e.target.checked)}
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            <label
              htmlFor={`${id}-start-immediately`}
              className="text-xs text-text-primary"
            >
              Start immediately
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
              {isPending ? "Creating…" : "Create"}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
