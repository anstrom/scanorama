import { useState, useId } from "react";
import { X, Loader2 } from "lucide-react";
import { Button } from "./button";
import { useUpdateNetwork } from "../api/hooks/use-networks";
import { useToast } from "./toast-provider";
import { cn } from "../lib/utils";

const DISCOVERY_METHODS = [
  { value: "ping", label: "Ping (ICMP echo)" },
  { value: "tcp", label: "TCP" },
  { value: "arp", label: "ARP broadcast" },
] as const;

type DiscoveryMethod = (typeof DISCOVERY_METHODS)[number]["value"];

export interface EditNetworkModalProps {
  network: {
    id?: string;
    name?: string;
    cidr?: string;
    description?: string;
    discovery_method?: string;
    scan_enabled?: boolean;
    is_active?: boolean;
  };
  onClose: () => void;
  onSaved?: () => void;
}

export function EditNetworkModal({
  network,
  onClose,
  onSaved,
}: EditNetworkModalProps) {
  const id = useId();
  const { toast } = useToast();

  const [name, setName] = useState(network.name ?? "");
  const [cidr, setCidr] = useState(network.cidr ?? "");
  const [description, setDescription] = useState(network.description ?? "");
  const [discoveryMethod, setDiscoveryMethod] = useState<DiscoveryMethod>(
    (network.discovery_method as DiscoveryMethod) ?? "ping",
  );
  const [scanEnabled, setScanEnabled] = useState(network.scan_enabled ?? true);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: updateNetwork, isPending } = useUpdateNetwork();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    const trimmedCidr = cidr.trim();

    if (!trimmedName) {
      setError("Network name is required.");
      return;
    }

    if (!trimmedCidr) {
      setError("CIDR block is required (e.g. 192.168.1.0/24).");
      return;
    }

    if (!/^[\da-fA-F:./]+\/\d+$/.test(trimmedCidr)) {
      setError(
        "CIDR block must be in valid notation (e.g. 192.168.1.0/24 or 10.0.0.0/8).",
      );
      return;
    }

    try {
      await updateNetwork({
        networkId: network.id ?? "",
        body: {
          name: trimmedName,
          cidr: trimmedCidr,
          description: description.trim() || undefined,
          discovery_method: discoveryMethod,
          scan_enabled: scanEnabled,
          is_active: network.is_active ?? true,
        },
      });
      toast.success("Network updated");
      onSaved?.();
      onClose();
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      const message =
        apiErr.message ?? apiErr.error ?? "Failed to update network.";
      toast.error(err instanceof Error ? err.message : message);
      setError(message);
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
            Edit Network
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
              placeholder="Office LAN"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

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
              placeholder="192.168.1.0/24"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
            <p className="text-xs text-text-muted">
              IPv4 or IPv6 CIDR notation (e.g. 10.0.0.0/8).
            </p>
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-description`}
              className="block text-xs font-medium text-text-primary"
            >
              Description{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              id={`${id}-description`}
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Main office network"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Discovery method */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-discovery`}
              className="block text-xs font-medium text-text-primary"
            >
              Discovery method
            </label>
            <select
              id={`${id}-discovery`}
              value={discoveryMethod}
              onChange={(e) =>
                setDiscoveryMethod(e.target.value as DiscoveryMethod)
              }
              aria-label="Select discovery method"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              {DISCOVERY_METHODS.map((m) => (
                <option key={m.value} value={m.value}>
                  {m.label}
                </option>
              ))}
            </select>
          </div>

          {/* Scan enabled */}
          <div className="flex items-center gap-2">
            <input
              id={`${id}-scan-enabled`}
              type="checkbox"
              checked={scanEnabled}
              onChange={(e) => setScanEnabled(e.target.checked)}
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            <label
              htmlFor={`${id}-scan-enabled`}
              className="text-xs text-text-primary"
            >
              Enable scanning for this network
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
              {isPending ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Saving…
                </>
              ) : (
                "Save changes"
              )}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
