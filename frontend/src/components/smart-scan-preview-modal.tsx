import { X, Zap, Loader2 } from "lucide-react";
import { Button } from "./button";
import { cn } from "../lib/utils";
import type { ScanStage, SuggestionSummary } from "../api/hooks/use-smart-scan";

// ── Host-level preview ────────────────────────────────────────────────────────

const STAGE_LABELS: Record<string, string> = {
  os_detection: "OS Detection",
  port_expansion: "Port Expansion",
  service_scan: "Service Scan",
  refresh: "Refresh",
  skip: "No action needed",
};

interface HostSmartScanPreviewProps {
  hostIp: string;
  stage: ScanStage;
  isPending: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

export function HostSmartScanPreviewModal({
  hostIp,
  stage,
  isPending,
  onConfirm,
  onClose,
}: HostSmartScanPreviewProps) {
  const isSkip = stage.stage === "skip";

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Smart Scan preview"
    >
      <div
        className="fixed inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />
      <div className="relative z-10 w-full max-w-sm bg-surface border border-border rounded-lg shadow-xl flex flex-col gap-4 p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-text-primary flex items-center gap-2">
            <Zap className="h-4 w-4 text-accent" />
            Smart Scan Preview
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="text-xs text-text-muted font-mono">{hostIp}</div>

        <div
          className={cn(
            "rounded-md border px-4 py-3 space-y-2",
            isSkip
              ? "border-border bg-surface-raised"
              : "border-accent/30 bg-accent/5",
          )}
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-text-primary">
              {STAGE_LABELS[stage.stage ?? ""] ?? stage.stage}
            </span>
            {!isSkip && stage.scan_type && (
              <span className="text-[11px] text-text-muted font-mono bg-surface-raised px-1.5 py-0.5 rounded">
                {stage.scan_type}
              </span>
            )}
          </div>
          {stage.reason && (
            <p className="text-xs text-text-secondary">{stage.reason}</p>
          )}
          {!isSkip && stage.ports && (
            <div className="flex gap-2 text-xs">
              <span className="text-text-muted w-16 shrink-0">Ports</span>
              <span className="font-mono text-text-secondary">{stage.ports}</span>
            </div>
          )}
          {!isSkip && stage.os_detection && (
            <div className="flex gap-2 text-xs">
              <span className="text-text-muted w-16 shrink-0">OS detect</span>
              <span className="text-text-secondary">enabled</span>
            </div>
          )}
        </div>

        <div className="flex gap-2 justify-end">
          <Button variant="secondary" onClick={onClose} className="text-xs h-7 px-3">
            Cancel
          </Button>
          {!isSkip && (
            <Button
              onClick={onConfirm}
              disabled={isPending}
              className="text-xs h-7 px-3"
            >
              {isPending ? (
                <>
                  <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
                  Queuing…
                </>
              ) : (
                <>
                  <Zap className="h-3 w-3 mr-1.5" />
                  Run Smart Scan
                </>
              )}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Batch (network) preview ───────────────────────────────────────────────────

interface BatchSmartScanPreviewProps {
  networkName: string;
  summary: SuggestionSummary;
  isPending: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

interface SuggestionRowProps {
  label: string;
  count: number | undefined;
  action: string | undefined;
}

function SuggestionRow({ label, count, action }: SuggestionRowProps) {
  if (!count) return null;
  return (
    <div className="flex items-center justify-between text-xs py-1">
      <span className="text-text-secondary">{label}</span>
      <div className="flex items-center gap-3">
        <span className="font-mono text-text-primary tabular-nums">{count}</span>
        <span className="text-[11px] text-text-muted font-mono bg-surface-raised px-1.5 py-0.5 rounded w-24 text-center">
          {action}
        </span>
      </div>
    </div>
  );
}

export function BatchSmartScanPreviewModal({
  networkName,
  summary,
  isPending,
  onConfirm,
  onClose,
}: BatchSmartScanPreviewProps) {
  const groups = [
    { label: "No OS info", ...summary.no_os_info },
    { label: "No open ports", ...summary.no_ports },
    { label: "No service banners", ...summary.no_services },
    { label: "Stale (>30 days)", ...summary.stale },
  ];
  const eligibleCount = groups.reduce((n, g) => n + (g.count ?? 0), 0);

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Smart Scan batch preview"
    >
      <div
        className="fixed inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />
      <div className="relative z-10 w-full max-w-sm bg-surface border border-border rounded-lg shadow-xl flex flex-col gap-4 p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-text-primary flex items-center gap-2">
            <Zap className="h-4 w-4 text-accent" />
            Smart Scan Preview
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="text-xs text-text-muted">{networkName}</div>

        <div className="rounded-md border border-border bg-surface-raised divide-y divide-border/50">
          {groups.map((g) => (
            <div key={g.label} className={cn("px-3", !g.count && "opacity-40")}>
              <SuggestionRow
                label={g.label}
                count={g.count}
                action={g.action}
              />
            </div>
          ))}
        </div>

        {eligibleCount === 0 ? (
          <p className="text-xs text-text-muted text-center">
            All hosts are well-known — no Smart Scan needed.
          </p>
        ) : (
          <p className="text-xs text-text-secondary">
            Up to{" "}
            <span className="font-semibold text-text-primary">
              {eligibleCount}
            </span>{" "}
            hosts will be queued for scanning (max 50 per batch).
          </p>
        )}

        <div className="flex gap-2 justify-end">
          <Button variant="secondary" onClick={onClose} className="text-xs h-7 px-3">
            Cancel
          </Button>
          {eligibleCount > 0 && (
            <Button
              onClick={onConfirm}
              disabled={isPending}
              className="text-xs h-7 px-3"
            >
              {isPending ? (
                <>
                  <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
                  Queuing…
                </>
              ) : (
                <>
                  <Zap className="h-3 w-3 mr-1.5" />
                  Run Smart Scan
                </>
              )}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
