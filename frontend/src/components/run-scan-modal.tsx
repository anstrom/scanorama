import { useState, useId } from "react";
import { X, Loader2 } from "lucide-react";
import { Button } from "./button";
import { useProfiles } from "../api/hooks/use-profiles";
import { useCreateScan, useStartScan } from "../api/hooks/use-scans";
import { cn, validatePortSpec } from "../lib/utils";

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

  const [target, setTarget] = useState(initialTarget);
  const [mode, setMode] = useState<Mode>("profile");
  const [profileId, setProfileId] = useState("");
  const [ports, setPorts] = useState("");
  const [scanType, setScanType] = useState<ScanType>("connect");
  const [osDetection, setOsDetection] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data: profilesData, isLoading: profilesLoading } = useProfiles({
    page: 1,
    page_size: 100,
  });
  const profiles = profilesData?.data ?? [];

  // Look up the selected profile object so we can read its scan_type / ports.
  const selectedProfile = profiles.find((p) => p.id === profileId) ?? null;

  const { mutateAsync: createScan, isPending: isCreating } = useCreateScan();
  const { mutateAsync: startScan, isPending: isStarting } = useStartScan();
  const isPending = isCreating || isStarting;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedTarget = target.trim();
    if (!trimmedTarget) {
      setError("Please enter at least one target.");
      return;
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

    // Split comma/whitespace-separated targets into an array.
    const targets = trimmedTarget
      .split(/[\s,]+/)
      .map((t) => t.trim())
      .filter(Boolean);

    if (targets.length > 100) {
      setError(`Too many targets (${targets.length}). Maximum is 100.`);
      return;
    }

    // Auto-generate a name from the first target.
    const name = `Ad-hoc scan: ${targets[0]}${targets.length > 1 ? ` +${targets.length - 1}` : ""}`;

    let result: Awaited<ReturnType<typeof createScan>> | undefined;
    try {
      if (mode === "profile") {
        // Expand the profile's settings into the request — the backend
        // expects scan_type at the top level, not a UUID profile_id.
        // Fall back to "1-65535" when the profile has no ports set, because
        // the backend now requires a non-empty ports field.
        result = await createScan({
          name,
          targets,
          scan_type: (selectedProfile?.scan_type ?? "connect") as ScanType,
          ports: selectedProfile?.ports || "1-65535",
          ...(osDetection ? { os_detection: true } : {}),
        });
      } else {
        result = await createScan({
          name,
          targets,
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
        // Scan was created (ID exists) but start failed — it sits as pending.
        setError(
          'Scan was created but could not be started. It will appear as "pending" in the Scans page.',
        );
        onSubmitted?.();
        return; // leave modal open so user sees the message
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
          {/* Target */}
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

          {/* Mode toggle */}
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
              OS detection{" "}
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
              {isPending ? "Starting…" : "Run scan"}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
