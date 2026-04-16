import { useState, useEffect } from "react";
import { RefreshCw, Info } from "lucide-react";

import { cn } from "../lib/utils";
import { Button } from "./button";
import { useUpdateCustomName, useRefreshIdentity } from "../api/hooks/use-hosts";
import { useToast } from "./toast-provider";
import type { components } from "../api/types";

type HostResponse = components["schemas"]["docs.HostResponse"];
type NameCandidate = components["schemas"]["docs.NameCandidateResponse"];

type Mode = "compact" | "full";

interface HostIdentityPanelProps {
  host: HostResponse;
  mode: Mode;
  // Compact mode shows a "Manage in Identity tab" link; full mode renders
  // the custom-name editor + refresh button. onManageClick is only consulted
  // in compact mode.
  onManageClick?: () => void;
}

// HostIdentityPanel renders either the compact read-only "top candidates"
// panel used by the Overview tab's chevron expand, or the full management
// surface used by the Identity tab. One component, two layouts — the mode
// prop gates the write controls.
export function HostIdentityPanel({
  host,
  mode,
  onManageClick,
}: HostIdentityPanelProps) {
  const candidates = host.name_candidates ?? [];
  const displayName = host.display_name ?? host.ip_address ?? "—";
  const source = host.display_name_source ?? "ip";

  if (mode === "compact") {
    return (
      <CompactPanel
        displayName={displayName}
        source={source}
        candidates={candidates}
        onManageClick={onManageClick}
      />
    );
  }

  return (
    <FullPanel
      host={host}
      candidates={candidates}
      displayName={displayName}
      source={source}
    />
  );
}

function CompactPanel({
  displayName,
  source,
  candidates,
  onManageClick,
}: {
  displayName: string;
  source: string;
  candidates: NameCandidate[];
  onManageClick?: () => void;
}) {
  // Show at most 4 rows; keep usable candidates first so the best signals
  // are visible without scrolling. Unusable rows with reasons are still
  // included so users see why a cert/PTR isn't winning.
  const top = [...candidates]
    .sort((a, b) => Number(b.usable ?? false) - Number(a.usable ?? false))
    .slice(0, 4);

  return (
    <div className="rounded-md border border-border bg-surface-raised p-3 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] uppercase tracking-wide text-text-muted">
          Chosen: <SourceBadge source={source} /> {displayName}
        </span>
        {onManageClick ? (
          <button
            type="button"
            onClick={onManageClick}
            className="text-[11px] text-accent hover:underline"
          >
            Manage in Identity tab →
          </button>
        ) : null}
      </div>
      {top.length === 0 ? (
        <p className="text-[11px] text-text-muted">
          No alternative names observed yet.
        </p>
      ) : (
        <table className="w-full text-[11px]">
          <tbody>
            {top.map((c, i) => (
              <tr
                key={`${c.source ?? "src"}-${c.name ?? ""}-${i}`}
                className={cn(!c.usable && "text-text-muted")}
              >
                <td className="py-0.5 pr-2 break-all">
                  {c.name ?? "—"}
                  {c.usable === false && c.not_usable_reason ? (
                    <span className="ml-1 text-danger/70">
                      ({c.not_usable_reason})
                    </span>
                  ) : null}
                </td>
                <td className="py-0.5 text-right text-text-muted whitespace-nowrap">
                  <SourceBadge source={c.source ?? ""} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function FullPanel({
  host,
  candidates,
  displayName,
  source,
}: {
  host: HostResponse;
  candidates: NameCandidate[];
  displayName: string;
  source: string;
}) {
  const [input, setInput] = useState(host.custom_name ?? "");
  const { toast } = useToast();
  const update = useUpdateCustomName();
  const refresh = useRefreshIdentity();

  // Keep the input in sync when the host prop changes underneath us (e.g.
  // the refresh-identity mutation invalidates the query and fetches fresh
  // data with a new custom_name).
  useEffect(() => {
    setInput(host.custom_name ?? "");
  }, [host.id, host.custom_name]);

  const hostId = host.id ?? "";

  async function save(name: string | null) {
    if (!hostId) return;
    try {
      await update.mutateAsync({ hostId, customName: name });
      toast.success(name == null ? "Custom name cleared" : "Custom name saved");
    } catch (e) {
      toast.error(
        e instanceof Error
          ? `Failed to save custom name: ${e.message}`
          : "Failed to save custom name",
      );
    }
  }

  async function onSave() {
    const trimmed = input.trim();
    await save(trimmed === "" ? null : trimmed);
  }

  async function onClear() {
    setInput("");
    await save(null);
  }

  async function onRefresh() {
    if (!hostId) return;
    try {
      await refresh.mutateAsync(hostId);
      toast.success(
        "Identity refresh queued — candidates update when the scan completes.",
      );
    } catch (e) {
      toast.error(
        e instanceof Error
          ? `Failed to queue identity refresh: ${e.message}`
          : "Failed to queue identity refresh",
      );
    }
  }

  async function onUseCandidate(candidate: NameCandidate) {
    if (!candidate.name) return;
    setInput(candidate.name);
    await save(candidate.name);
  }

  return (
    <div className="space-y-5">
      <section>
        <h4 className="text-[10px] uppercase tracking-wide text-text-muted mb-2">
          Custom name (user override)
        </h4>
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="e.g. office-router"
            maxLength={255}
            aria-label="Custom name"
            className="flex-1 px-2 py-1 text-xs rounded border border-border bg-surface text-text-primary focus:outline-none focus:ring-1 focus:ring-border min-w-0"
          />
          <Button
            size="sm"
            onClick={() => void onSave()}
            loading={update.isPending}
          >
            Save
          </Button>
          {host.custom_name ? (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => void onClear()}
              loading={update.isPending}
            >
              Clear
            </Button>
          ) : null}
        </div>
      </section>

      <section>
        <h4 className="text-[10px] uppercase tracking-wide text-text-muted mb-2">
          Name sources (ranked)
        </h4>
        <div className="rounded-md border border-border overflow-hidden">
          <table className="w-full text-xs">
            <thead className="bg-surface-raised text-[10px] uppercase text-text-muted">
              <tr>
                <th className="text-left px-3 py-1.5 font-medium">Name</th>
                <th className="text-left px-3 py-1.5 font-medium">Source</th>
                <th className="text-left px-3 py-1.5 font-medium">Observed</th>
                <th className="px-3 py-1.5" />
              </tr>
            </thead>
            <tbody>
              {candidates.length === 0 ? (
                <tr>
                  <td
                    colSpan={4}
                    className="px-3 py-4 text-center text-text-muted italic"
                  >
                    No name candidates have been observed for this host yet.
                  </td>
                </tr>
              ) : (
                candidates.map((c, i) => (
                  <CandidateRow
                    key={`${c.source ?? "src"}-${c.name ?? ""}-${i}`}
                    candidate={c}
                    winning={
                      c.usable === true &&
                      c.source === source &&
                      c.name === displayName
                    }
                    onUse={() => void onUseCandidate(c)}
                    busy={update.isPending}
                  />
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="flex items-center justify-between">
        <Button
          size="sm"
          variant="secondary"
          onClick={() => void onRefresh()}
          loading={refresh.isPending}
          icon={<RefreshCw className="h-3 w-3" />}
        >
          Refresh identity now
        </Button>
        <span className="text-[11px] text-text-muted flex items-center gap-1">
          <Info className="h-3 w-3" />
          Auto-runs via SmartScan when a host lacks a name.
        </span>
      </section>
    </div>
  );
}

function CandidateRow({
  candidate,
  winning,
  onUse,
  busy,
}: {
  candidate: NameCandidate;
  winning: boolean;
  onUse: () => void;
  busy: boolean;
}) {
  const observedAt = candidate.observed_at;
  return (
    <tr
      className={cn(
        "border-t border-border",
        candidate.usable === false && "text-text-muted",
      )}
    >
      <td className="px-3 py-2 break-all">
        {winning ? <span className="text-success mr-1">✓</span> : null}
        <span className={winning ? "font-semibold" : ""}>
          {candidate.name ?? "—"}
        </span>
      </td>
      <td className="px-3 py-2">
        <SourceBadge source={candidate.source ?? ""} />
      </td>
      <td className="px-3 py-2 text-text-muted whitespace-nowrap">
        {observedAt ? formatRelative(observedAt) : "—"}
      </td>
      <td className="px-3 py-2 text-right">
        {candidate.usable === false ? (
          <span className="text-danger/70 text-[11px]">
            {candidate.not_usable_reason ?? "unusable"}
          </span>
        ) : winning ? (
          <span className="text-success text-[11px]">selected</span>
        ) : (
          <button
            type="button"
            onClick={onUse}
            disabled={busy}
            className="text-[11px] text-accent hover:underline disabled:text-text-muted"
          >
            use
          </button>
        )}
      </td>
    </tr>
  );
}

function SourceBadge({ source }: { source: string }) {
  if (!source) return null;
  return (
    <span className="inline-block px-1.5 py-0.5 rounded bg-surface text-[10px] uppercase tracking-wide text-text-muted">
      {source}
    </span>
  );
}

function formatRelative(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "—";
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}
