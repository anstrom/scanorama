import { useState, useId } from "react";
import { Loader2, CheckCircle2 } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { Button } from "./button";
import { useNetworks, useCreateNetwork } from "../api/hooks/use-networks";
import {
  useRerunDiscovery,
  useDiscoveryJobs,
} from "../api/hooks/use-discovery";
import { cn } from "../lib/utils";

const SKIP_KEY = "scanorama_onboarding_skipped";
const TOTAL_STEPS = 3;

const DISCOVERY_METHODS = [
  { value: "ping", label: "Ping (ICMP echo)" },
  { value: "tcp", label: "TCP" },
  { value: "arp", label: "ARP broadcast" },
] as const;

type DiscoveryMethod = (typeof DISCOVERY_METHODS)[number]["value"];

// ── Step 1: Add a network ─────────────────────────────────────────────────────

interface Step1Props {
  onCreated: (networkId: string) => void;
  onSkip: () => void;
}

function Step1({ onCreated, onSkip }: Step1Props) {
  const id = useId();
  const [name, setName] = useState("");
  const [cidr, setCidr] = useState("");
  const [description, setDescription] = useState("");
  const [discoveryMethod, setDiscoveryMethod] =
    useState<DiscoveryMethod>("ping");
  const [scanEnabled, setScanEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createNetwork, isPending } = useCreateNetwork();

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
      const created = await createNetwork({
        name: trimmedName,
        cidr: trimmedCidr,
        description: description.trim() || undefined,
        discovery_method: discoveryMethod,
        scan_enabled: scanEnabled,
        is_active: true,
      });
      if (created?.id) {
        onCreated(created.id);
      }
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      const message =
        apiErr.message ?? apiErr.error ?? "Failed to create network.";
      setError(message);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="px-5 py-4 space-y-5">
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

      {error && (
        <p role="alert" className="text-xs text-danger">
          {error}
        </p>
      )}

      <div className="flex justify-between gap-2 pt-1">
        <Button variant="ghost" type="button" onClick={onSkip}>
          Skip setup
        </Button>
        <Button type="submit" loading={isPending}>
          {isPending ? (
            <>
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              Creating…
            </>
          ) : (
            "Add network"
          )}
        </Button>
      </div>
    </form>
  );
}

// ── Step 2: Run a discovery scan ──────────────────────────────────────────────

interface Step2Props {
  networkId: string;
  onComplete: (jobId: string, newHostsCount: number) => void;
  onSkip: () => void;
}

function Step2({ networkId, onComplete, onSkip }: Step2Props) {
  const [jobId, setJobId] = useState<string | null>(null);
  const [startError, setStartError] = useState<string | null>(null);

  const { mutateAsync: rerunDiscovery, isPending } = useRerunDiscovery();

  // Fetch all jobs (no status filter) so we can detect completed and failed.
  const { data: jobsData, error: jobsError } = useDiscoveryJobs();

  const allJobs = jobsData?.data ?? [];
  const ourJob = jobId !== null ? allJobs.find((j) => j.id === jobId) : undefined;

  // When jobId is set but the job isn't in the list yet, treat as still pending.
  const jobStatus = ourJob?.status ?? (jobId !== null ? "pending" : null);
  const jobsFailed = jobId !== null && !!jobsError;
  const scanComplete = jobStatus === "completed";
  const scanFailed = jobStatus === "failed";
  const scanRunning = jobId !== null && !scanComplete && !scanFailed && !jobsFailed;

  async function handleStart() {
    setStartError(null);
    try {
      const created = await rerunDiscovery({ networks: [networkId] });
      if (created?.id) {
        setJobId(created.id);
      }
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      setStartError(apiErr.message ?? apiErr.error ?? "Failed to start discovery.");
    }
  }

  function handleRetry() {
    setJobId(null);
  }

  function handleNext() {
    if (jobId && ourJob) {
      onComplete(jobId, ourJob.hosts_found ?? 0);
    }
  }

  return (
    <div className="px-5 py-4 space-y-5">
      <p className="text-sm text-text-primary">
        Run a discovery scan to find all devices on your network.
      </p>

      {!jobId && (
        <div className="space-y-3">
          <p className="text-xs text-text-muted">
            This will probe the CIDR range you just added and list all
            responding hosts.
          </p>
          {startError && (
            <p role="alert" className="text-xs text-danger">
              {startError}
            </p>
          )}
          <div className="flex justify-between gap-2 pt-1">
            <Button variant="ghost" type="button" onClick={onSkip}>
              Skip setup
            </Button>
            <Button onClick={() => void handleStart()} loading={isPending}>
              {isPending ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Starting…
                </>
              ) : (
                "Start discovery scan"
              )}
            </Button>
          </div>
        </div>
      )}

      {scanRunning && (
        <div className="space-y-3">
          <div
            className="flex items-center gap-2 text-xs text-text-muted"
            aria-live="polite"
          >
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>Discovery scan running…</span>
          </div>
          <div className="flex justify-between gap-2 pt-1">
            <Button variant="ghost" type="button" onClick={onSkip}>
              Skip setup
            </Button>
            <Button variant="secondary" disabled>
              Next
            </Button>
          </div>
        </div>
      )}

      {scanFailed && (
        <div className="space-y-3">
          <p role="alert" className="text-xs text-danger">
            Discovery scan failed.
          </p>
          <div className="flex justify-between gap-2 pt-1">
            <Button variant="ghost" type="button" onClick={onSkip}>
              Skip setup
            </Button>
            <Button onClick={handleRetry}>Retry</Button>
          </div>
        </div>
      )}

      {jobsFailed && (
        <div className="space-y-3">
          <p role="alert" className="text-xs text-danger">
            Failed to check scan status. Please try again.
          </p>
          <div className="flex justify-between gap-2 pt-1">
            <Button variant="ghost" type="button" onClick={onSkip}>
              Skip setup
            </Button>
            <Button variant="secondary" onClick={() => setJobId(null)}>
              Retry
            </Button>
          </div>
        </div>
      )}

      {scanComplete && (
        <div className="space-y-3">
          <div
            className="flex items-center gap-2 text-xs text-text-primary"
            aria-live="polite"
          >
            <CheckCircle2 className="h-4 w-4 text-success" />
            <span>Discovery scan complete.</span>
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <Button onClick={handleNext}>View results</Button>
          </div>
        </div>
      )}
    </div>
  );
}

// ── Step 3: Review results ────────────────────────────────────────────────────

interface Step3Props {
  newHostsCount: number;
  onDone: () => void;
}

function Step3({ newHostsCount, onDone }: Step3Props) {
  return (
    <div className="px-5 py-4 space-y-5">
      <p className="text-sm text-text-primary">
        Your network has been scanned. Here is what was found:
      </p>

      <div className="rounded border border-border bg-surface-raised px-4 py-3">
        <p className="text-xs text-text-muted">Hosts discovered</p>
        <p className="text-2xl font-semibold text-text-primary mt-0.5">
          {newHostsCount}
        </p>
      </div>

      <p className="text-xs text-text-muted">
        View all discovered hosts to see details, open ports, and more.
      </p>

      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={onDone}>Go to Hosts</Button>
      </div>
    </div>
  );
}

// ── Progress indicator ─────────────────────────────────────────────────────────

interface ProgressProps {
  step: number;
  total: number;
}

function ProgressIndicator({ step, total }: ProgressProps) {
  return (
    <div className="flex items-center gap-2 px-5 py-3 border-b border-border">
      <span className="text-xs text-text-muted">
        Step {step} of {total}
      </span>
      <div className="flex gap-1.5 ml-1">
        {Array.from({ length: total }, (_, i) => (
          <div
            key={i}
            className={cn(
              "h-1.5 w-1.5 rounded-full transition-colors",
              i + 1 <= step ? "bg-accent" : "bg-border",
            )}
          />
        ))}
      </div>
    </div>
  );
}

// ── Wizard shell ──────────────────────────────────────────────────────────────

export function OnboardingWizard() {
  const { data: networksData, isLoading } = useNetworks();
  const navigate = useNavigate();

  const [dismissed, setDismissed] = useState(
    () => localStorage.getItem(SKIP_KEY) === "true",
  );
  const [step, setStep] = useState(1);
  const [createdNetworkId, setCreatedNetworkId] = useState<string | null>(null);
  const [completedJobId, setCompletedJobId] = useState<string | null>(null);
  const [newHostsCount, setNewHostsCount] = useState(0);

  function handleSkip() {
    localStorage.setItem(SKIP_KEY, "true");
    setDismissed(true);
  }

  function handleNetworkCreated(networkId: string) {
    setCreatedNetworkId(networkId);
    setStep(2);
  }

  function handleScanComplete(jobId: string, hostCount: number) {
    setCompletedJobId(jobId);
    setNewHostsCount(hostCount);
    setStep(3);
  }

  function handleDone() {
    localStorage.setItem(SKIP_KEY, "true");
    setDismissed(true);
    void navigate({ to: "/hosts" });
  }

  // Don't render while loading, when dismissed, or when networks already exist.
  if (isLoading || dismissed) return null;
  const total = networksData?.total ?? 0;
  if (total > 0) return null;

  const titles: Record<number, string> = {
    1: "Add your first network",
    2: "Run a discovery scan",
    3: "Review results",
  };

  return (
    <>
      <div className="fixed inset-0 bg-black/50 z-40" aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="First-run setup wizard"
        className={cn(
          "fixed z-50 inset-0 m-auto",
          "w-full max-w-md h-fit",
          "bg-surface border border-border rounded-lg shadow-xl",
          "flex flex-col",
        )}
      >
        <ProgressIndicator step={step} total={TOTAL_STEPS} />

        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold text-text-primary">
            {titles[step]}
          </h2>
        </div>

        {step === 1 && (
          <Step1 onCreated={handleNetworkCreated} onSkip={handleSkip} />
        )}
        {step === 2 && createdNetworkId && (
          <Step2
            networkId={createdNetworkId}
            onComplete={handleScanComplete}
            onSkip={handleSkip}
          />
        )}
        {step === 3 && completedJobId && (
          <Step3
            newHostsCount={newHostsCount}
            onDone={handleDone}
          />
        )}
      </div>
    </>
  );
}
