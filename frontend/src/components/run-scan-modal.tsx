import { useState, useId } from "react";
import { X, Loader2, Network } from "lucide-react";
import { Button } from "./button";
import { useProfiles } from "../api/hooks/use-profiles";
import { useNetworks } from "../api/hooks/use-networks";
import { useCreateScan, useStartScan } from "../api/hooks/use-scans";
import { cn, validatePortSpec } from "../lib/utils";

type Source = "manual" | "network";
type Mode = "profile" | "custom";

type ScanType =
  | "connect"
  | "syn"
  | "ack"
  | "udp"
  | "aggressive"
  | "comprehensive";

const SCAN_TYPES: { value: ScanType; label: string }[] = [
  { value: "connect", label: "Connect (-sT)" },
  { value: "syn", label: "SYN stealth (-sS)" },
  { value: "ack", label: "ACK (-sA)" },
  { value: "udp", label: "UDP (-sU)" },
  { value: "aggressive", label: "Aggressive (-sS -sV -A)" },
  { value: "comprehensive", label: "Comprehensive (-sS -sV --script=default)" },
] as const;

export interface RunScanModalProps {
  /** Pre-fill the target field (e.g. a host IP from the Hosts page). */
  initialTarget?: string;
  onClose: () => void;
  /** Called after the scan is successfully submitted. */
  onSubmitted?: () => void;
}

export function RunScanModal({
  initialTarget = "",
  onClose,
  onSubmitted,
}: RunScanModalProps) {
  const id = useId();

  // Source: manual free-text entry vs. picking a registered network
  const [source, setSource] = useState<Source>("manual");
  const [selectedNetworkId, setSelectedNetworkId] = useState("");

  // Manual target text
  const [target, setTarget] = useState(initialTarget);

  // Scan configuration
  const [mode, setMode] = useState<Mode>("profile");
  const [profileId, setProfileId] = useState("");
  const [ports, setPorts] = useState("");
  const [scanType, setScanType] = useState<ScanType>("connect");
  const [osDetection, setOsDetection] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Data
  const { data: profilesData, isLoading: profilesLoading } = useProfiles({
    page: 1,
    page_size: 100,
  });
  const profiles = profilesData?.data ?? [];
  const selectedProfile = profiles.find((p) => p.id === profileId) ?? null;

  const { data: networksData, isLoading: networksLoading } = useNetworks({
    page: 1,
    page_size: 200,
  });
  const networks = networksData?.data ?? [];
  const selectedNetwork =
    networks.find((n) => n.id === selectedNetworkId) ?? null;

  // Mutations
  const { mutateAsync: createScan, isPending: isCreating } = useCreateScan();
  const { mutateAsync: startScan, isPending: isStarting } = useStartScan();
  const isPending = isCreating || isStarting;

  function handleSourceChange(s: Source) {
    setSource(s);
    setError(null);
    if (s === "manual") {
      setSelectedNetworkId("");
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    // Resolve targets and auto-generated name based on source mode
    let effectiveTargets: string[];
    let scanName: string;

    if (source === "network") {
      if (!selectedNetwork) {
        setError("Please select a network.");
        return;
      }
      if (!selectedNetwork.cidr) {
        setError("The selected network has no CIDR configured.");
        return;
      }
      effectiveTargets = [selectedNetwork.cidr];
      const label = selectedNetwork.name
        ? selectedNetwork.name + " (" + selectedNetwork.cidr + ")"
        : selectedNetwork.cidr;
      scanName = "Scan: " + label;
    } else {
      const trimmedTarget = target.trim();
      if (!trimmedTarget) {
        setError("Please enter at least one target.");
        return;
      }
      effectiveTargets = trimmedTarget
        .split(/[\s,]+/)
        .map((t) => t.trim())
        .filter(Boolean);
      if (effectiveTargets.length > 100) {
        setError(
          "Too many targets (" + effectiveTargets.length + "). Maximum is 100.",
        );
        return;
      }
      scanName =
        "Ad-hoc scan: " +
        effectiveTargets[0] +
        (effectiveTargets.length > 1
          ? " +" + (effectiveTargets.length - 1)
          : "");
    }

    if (mode === "profile" && !profileId) {
      setError("Please select a profile.");
      return;
    }

    if (mode === "custom" && !ports.trim()) {
      setError(
        "Ports are required. Enter a port number, range, or list (e.g. 22,80,443 or 1-1024).",
      );
      return;
    }

    if (mode === "custom" && ports.trim()) {
      const portError = validatePortSpec(ports.trim());
      if (portError) {
        setError(portError);
        return;
      }
    }

    let result: Awaited<ReturnType<typeof createScan>> | undefined;
    try {
      if (mode === "profile") {
        // Expand profile settings — backend expects scan_type at the top level.
        // Fall back to "1-65535" when the profile has no ports set.
        result = await createScan({
          name: scanName,
          targets: effectiveTargets,
          scan_type: (selectedProfile?.scan_type ?? "connect") as ScanType,
          ports: selectedProfile?.ports || "1-65535",
          ...(osDetection ? { os_detection: true } : {}),
        });
      } else {
        result = await createScan({
          name: scanName,
          targets: effectiveTargets,
          scan_type: scanType,
          ports: ports.trim(),
          ...(osDetection ? { os_detection: true } : {}),
        });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create scan.");
      return;
    }

    const scanId = result?.id;
    if (scanId) {
      try {
        await startScan(scanId);
      } catch {
        // Scan was created but start failed — it sits as pending.
        setError(
          'Scan was created but could not be started. It will appear as "pending" in the Scans page.',
        );
        onSubmitted?.();
        return;
      }
    }

    onSubmitted?.();
    onClose();
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
          "w-full max-w-md h-fit max-h-[90vh] overflow-y-auto",
          "bg-surface border border-border rounded-lg shadow-xl",
          "flex flex-col",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border sticky top-0 bg-surface z-10">
          <h2
            id={`${id}-title`}
            className="text-sm font-semibold text-text-primary"
          >
            Run Scan
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
          {/* Target source toggle */}
          <div className="space-y-1.5">
            <span className="block text-xs font-medium text-text-primary">
              Target source
            </span>
            <div
              role="radiogroup"
              aria-label="Target source"
              className="flex rounded border border-border overflow-hidden text-xs"
            >
              {(["manual", "network"] as Source[]).map((s) => (
                <button
                  key={s}
                  type="button"
                  role="radio"
                  aria-checked={source === s}
                  onClick={() => handleSourceChange(s)}
                  className={cn(
                    "flex-1 py-1.5 text-center transition-colors",
                    source === s
                      ? "bg-accent text-white font-medium"
                      : "text-text-secondary hover:bg-surface-raised",
                  )}
                >
                  {s === "manual" ? "Manual" : "Network"}
                </button>
              ))}
            </div>
          </div>

          {/* Manual: free-text target */}
          {source === "manual" && (
            <div className="space-y-1.5">
              <label
                htmlFor={`${id}-target`}
                className="block text-xs font-medium text-text-primary"
              >
                Target
              </label>
              <input
                id={`${id}-target`}
                type="text"
                value={target}
                onChange={(e) => setTarget(e.target.value)}
                placeholder="192.168.1.1, 10.0.0.0/24…"
                autoFocus
                className={cn(
                  "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                  "bg-surface text-text-primary placeholder:text-text-muted",
                  "focus:outline-none focus:ring-1 focus:ring-border",
                )}
              />
              <p className="text-xs text-text-muted">
                Comma-separated IPs, ranges, or CIDR blocks.
              </p>
            </div>
          )}

          {/* Network: registered network picker */}
          {source === "network" && (
            <div className="space-y-2">
              <label
                htmlFor={`${id}-network`}
                className="block text-xs font-medium text-text-primary"
              >
                Network
              </label>

              {networksLoading ? (
                <div className="flex items-center gap-2 text-xs text-text-muted py-1">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Loading networks…
                </div>
              ) : networks.length === 0 ? (
                <p className="text-xs text-text-muted py-1">
                  No networks found. Add one on the Networks page first.
                </p>
              ) : (
                <select
                  id={`${id}-network`}
                  value={selectedNetworkId}
                  onChange={(e) => setSelectedNetworkId(e.target.value)}
                  aria-label="Select network"
                  className={cn(
                    "w-full px-3 py-1.5 text-xs rounded border border-border",
                    "bg-surface text-text-primary",
                    "focus:outline-none focus:ring-1 focus:ring-border",
                  )}
                >
                  <option value="">— Select a network —</option>
                  {networks.map((n) => (
                    <option key={n.id} value={n.id ?? ""}>
                      {n.name
                        ? n.name + (n.cidr ? " — " + n.cidr : "")
                        : (n.cidr ?? "")}
                    </option>
                  ))}
                </select>
              )}

              {/* Selected network info pill */}
              {selectedNetwork && (
                <div className="flex items-center gap-2 px-3 py-2 rounded bg-surface-raised border border-border/50 text-xs">
                  <Network className="h-3.5 w-3.5 text-text-muted shrink-0" />
                  <div className="min-w-0 flex-1">
                    <span className="font-mono text-text-primary">
                      {selectedNetwork.cidr}
                    </span>
                    {selectedNetwork.active_host_count != null && (
                      <span className="ml-2 text-text-muted">
                        {selectedNetwork.active_host_count}
                        {" active host"}
                        {selectedNetwork.active_host_count !== 1 ? "s" : ""}
                      </span>
                    )}
                  </div>
                  <span
                    className={cn(
                      "shrink-0 px-1.5 py-0.5 rounded text-[11px] font-medium",
                      selectedNetwork.is_active
                        ? "bg-success/15 text-success"
                        : "bg-text-muted/15 text-text-muted",
                    )}
                  >
                    {selectedNetwork.is_active ? "active" : "inactive"}
                  </span>
                </div>
              )}

              <p className="text-xs text-text-muted">
                The network's full CIDR range will be used as the scan target.
              </p>
            </div>
          )}

          {/* Scan configuration toggle */}
          <div className="space-y-1.5">
            <span className="block text-xs font-medium text-text-primary">
              Scan configuration
            </span>
            <div
              role="radiogroup"
              aria-label="Scan configuration mode"
              className="flex rounded border border-border overflow-hidden text-xs"
            >
              {(["profile", "custom"] as Mode[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  role="radio"
                  aria-checked={mode === m}
                  onClick={() => setMode(m)}
                  className={cn(
                    "flex-1 py-1.5 text-center transition-colors",
                    mode === m
                      ? "bg-accent text-white font-medium"
                      : "text-text-secondary hover:bg-surface-raised",
                  )}
                >
                  {m === "profile" ? "Profile" : "Custom ports"}
                </button>
              ))}
            </div>
          </div>

          {/* Profile selector */}
          {mode === "profile" && (
            <div className="space-y-1.5">
              <label
                htmlFor={`${id}-profile`}
                className="block text-xs font-medium text-text-primary"
              >
                Profile
              </label>
              {profilesLoading ? (
                <div className="flex items-center gap-2 text-xs text-text-muted py-1">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Loading profiles…
                </div>
              ) : profiles.length === 0 ? (
                <p className="text-xs text-text-muted py-1">
                  No profiles found. Create one under Profiles first.
                </p>
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
                  <option value="">— Select a profile —</option>
                  {profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name ?? p.id}
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}

          {/* OS fingerprinting */}
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

          {/* Custom ports */}
          {mode === "custom" && (
            <div className="space-y-4">
              <div className="space-y-1.5">
                <label
                  htmlFor={`${id}-ports`}
                  className="block text-xs font-medium text-text-primary"
                >
                  Ports
                  <span className="text-text-muted font-normal ml-1">
                    (required — e.g. 22,80,443 or 1-1024)
                  </span>
                </label>
                <input
                  id={`${id}-ports`}
                  type="text"
                  value={ports}
                  onChange={(e) => setPorts(e.target.value)}
                  placeholder="22,80,443,8080-8090"
                  className={cn(
                    "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                    "bg-surface text-text-primary placeholder:text-text-muted",
                    "focus:outline-none focus:ring-1 focus:ring-border",
                  )}
                />
              </div>

              <div className="space-y-1.5">
                <label
                  htmlFor={`${id}-scan-type`}
                  className="block text-xs font-medium text-text-primary"
                >
                  Scan type
                </label>
                <select
                  id={`${id}-scan-type`}
                  value={scanType}
                  onChange={(e) => setScanType(e.target.value as ScanType)}
                  aria-label="Select scan type"
                  className={cn(
                    "w-full px-3 py-1.5 text-xs rounded border border-border",
                    "bg-surface text-text-primary",
                    "focus:outline-none focus:ring-1 focus:ring-border",
                  )}
                >
                  {SCAN_TYPES.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          )}

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
              {isPending ? "Starting\u2026" : "Run scan"}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
