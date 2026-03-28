import { useState, useId } from "react";
import { X, Loader2 } from "lucide-react";
import { Button } from "./button";
import { useProfiles } from "../api/hooks/use-profiles";
import { useStartNetworkScan } from "../api/hooks/use-networks";
import { useStartScan } from "../api/hooks/use-scans";
import { useHosts } from "../api/hooks/use-hosts";
import { useToast } from "./toast-provider";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type NetworkResponse = components["schemas"]["docs.NetworkResponse"];

export interface ScanNetworkModalProps {
  network: NetworkResponse;
  onClose: () => void;
  onSubmitted?: () => void;
}

export function ScanNetworkModal({
  network,
  onClose,
  onSubmitted,
}: ScanNetworkModalProps) {
  const id = useId();
  const { toast } = useToast();

  const [profileId, setProfileId] = useState("");
  const [osDetection, setOsDetection] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Pre-fetch active host count for the network
  const { data: hostsData, isLoading: hostsLoading } = useHosts({
    network: network.cidr ?? "",
    status: "up",
    page: 1,
    page_size: 1,
  });
  const activeHostCount = hostsData?.pagination?.total_items ?? 0;

  const { data: profilesData, isLoading: profilesLoading } = useProfiles({
    page: 1,
    page_size: 100,
  });
  const profiles = profilesData?.data ?? [];

  const { mutateAsync: createNetworkScan, isPending: isCreating } =
    useStartNetworkScan();
  const { mutateAsync: startScan, isPending: isStarting } = useStartScan();
  const isPending = isCreating || isStarting;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (activeHostCount === 0) {
      setError("No active hosts found in this network.");
      return;
    }

    try {
      const scan = await createNetworkScan({
        networkId: network.id ?? "",
        osDetection,
      });
      if (scan?.id) {
        await startScan(scan.id);
      }
      toast.success(
        `Scan started for ${activeHostCount} active host${activeHostCount !== 1 ? "s" : ""}`,
      );
      onSubmitted?.();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to start scan.";
      setError(msg);
      toast.error(msg);
    }
  }

  return (
    <>
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
        aria-hidden="true"
      />
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
        <div className="flex items-center justify-between px-5 py-4 border-b border-border">
          <h2
            id={`${id}-title`}
            className="text-sm font-semibold text-text-primary"
          >
            Scan Active Hosts
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

        <form onSubmit={handleSubmit} className="px-5 py-4 space-y-5">
          <div className="rounded bg-surface-raised border border-border p-3 space-y-1">
            <p className="text-xs font-medium text-text-primary">
              {network.name}
            </p>
            <p className="text-xs font-mono text-text-muted">{network.cidr}</p>
            {hostsLoading ? (
              <div className="flex items-center gap-1.5 text-xs text-text-muted">
                <Loader2 className="h-3 w-3 animate-spin" />
                Checking active hosts…
              </div>
            ) : (
              <p className="text-xs text-text-secondary">
                <span
                  className={cn(
                    "font-medium",
                    activeHostCount > 0 ? "text-success" : "text-text-muted",
                  )}
                >
                  {activeHostCount}
                </span>{" "}
                active host{activeHostCount !== 1 ? "s" : ""} will be scanned
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-profile`}
              className="block text-xs font-medium text-text-primary"
            >
              Profile{" "}
              <span className="text-text-muted font-normal">
                (optional — defaults to connect scan, ports 1-1024)
              </span>
            </label>
            {profilesLoading ? (
              <div className="flex items-center gap-2 text-xs text-text-muted py-1">
                <Loader2 className="h-3 w-3 animate-spin" />
                Loading profiles…
              </div>
            ) : (
              <select
                id={`${id}-profile`}
                value={profileId}
                onChange={(e) => setProfileId(e.target.value)}
                aria-label="Select profile"
                className={cn(
                  "w-full px-3 py-1.5 text-xs rounded border border-border",
                  "bg-surface text-text-primary",
                  "focus:outline-none focus:ring-1 focus:ring-border",
                )}
              >
                <option value="">
                  — Default (connect scan, ports 1–1024) —
                </option>
                {profiles.map((p) => (
                  <option key={p.id} value={p.id ?? ""}>
                    {p.name ?? p.id}
                  </option>
                ))}
              </select>
            )}
          </div>

          {/* OS detection */}
          <div className="flex items-center gap-2">
            <input
              id={`${id}-os-detection`}
              type="checkbox"
              checked={osDetection}
              onChange={(e) => setOsDetection(e.target.checked)}
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            <label
              htmlFor={`${id}-os-detection`}
              className="text-xs text-text-primary"
            >
              OS fingerprinting{" "}
              <span className="text-text-muted font-normal">
                (-O, requires root)
              </span>
            </label>
          </div>

          {error && (
            <p role="alert" className="text-xs text-danger">
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-1">
            <Button variant="secondary" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              loading={isPending}
              disabled={hostsLoading || activeHostCount === 0}
            >
              {isPending
                ? "Starting…"
                : `Scan ${activeHostCount > 0 ? activeHostCount + " " : ""}host${activeHostCount !== 1 ? "s" : ""}`}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
