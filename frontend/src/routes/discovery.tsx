import { useState } from "react";
import { X, Plus, ArrowRight, GitCompare } from "lucide-react";
import { useSearch } from "@tanstack/react-router";
import { Button } from "../components/button";
import {
  useDiscoveryJobs,
  useDiscoveryDiff,
  useDiscoveryCompare,
  useStartDiscovery,
  useStopDiscovery,
} from "../api/hooks/use-discovery";
import type {
  DiscoveryDiff,
  DiscoveryDiffHost,
  DiscoveryCompareDiff,
} from "../api/hooks/use-discovery";
import { StatusBadge, Skeleton, PaginationBar } from "../components";
import { CreateDiscoveryModal } from "../components/create-discovery-modal";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type DiscoveryJobResponse = components["schemas"]["docs.DiscoveryJobResponse"];

const PAGE_SIZE = 20;

const METHOD_LABELS: Record<string, string> = {
  ping: "Ping",
  tcp: "TCP",
  tcp_connect: "TCP Connect",
  icmp: "ICMP",
  arp: "ARP",
};

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function SkeletonRows({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <tr key={i} className="border-b border-border/50">
          <td className="py-3 px-4 pr-4">
            <Skeleton className="h-3 w-32" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-28" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-12" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-16" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-1 w-20" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-16" />
          </td>
          <td className="py-3 pr-4">
            <Skeleton className="h-3 w-16" />
          </td>
          <td className="py-3" />
        </tr>
      ))}
    </>
  );
}

// ── Changes tab components ────────────────────────────────────────────────────

function DiffHostRow({
  host,
  showStatusChange,
}: {
  host: DiscoveryDiffHost;
  showStatusChange?: boolean;
}) {
  return (
    <div className="flex items-center gap-3 text-xs py-1.5">
      <span className="font-mono text-text-secondary w-28 shrink-0">
        {host.ip_address}
      </span>
      <span className="text-text-muted w-20 shrink-0 truncate">
        {host.hostname ?? "—"}
      </span>
      <span className="text-text-muted flex-1 truncate">
        {host.vendor ?? "—"}
      </span>
      {showStatusChange && host.previous_status && (
        <span className="text-text-muted shrink-0">
          {host.previous_status} → {host.status}
        </span>
      )}
      <span className="text-text-muted shrink-0">
        {formatRelativeTime(host.last_seen)}
      </span>
    </div>
  );
}

interface DiffSectionProps {
  title: string;
  count: number;
  hosts: DiscoveryDiffHost[];
  headerClass: string;
  showStatusChange?: boolean;
}

function DiffSection({
  title,
  count,
  hosts,
  headerClass,
  showStatusChange,
}: DiffSectionProps) {
  return (
    <section>
      <h4 className={cn("text-xs font-medium mb-2", headerClass)}>
        {title} ({count})
      </h4>
      {hosts.length === 0 ? (
        <p className="text-xs text-text-muted">None</p>
      ) : (
        <div className="space-y-0.5">
          {hosts.map((host) => (
            <DiffHostRow
              key={host.id}
              host={host}
              showStatusChange={showStatusChange}
            />
          ))}
        </div>
      )}
    </section>
  );
}

interface ChangesTabProps {
  diff?: DiscoveryDiff;
  isLoading: boolean;
  isError: boolean;
}

function ChangesTab({ diff, isLoading, isError }: ChangesTabProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    );
  }

  if (isError) {
    return <p className="text-xs text-danger">Failed to load changes.</p>;
  }

  if (!diff) return null;

  const isEmpty =
    diff.new_hosts.length === 0 &&
    diff.gone_hosts.length === 0 &&
    diff.changed_hosts.length === 0 &&
    diff.unchanged_count === 0;

  if (isEmpty) {
    return (
      <p className="text-xs text-text-muted">
        No changes detected in this run.
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <DiffSection
        title="New"
        count={diff.new_hosts.length}
        hosts={diff.new_hosts}
        headerClass="text-success"
      />
      <DiffSection
        title="Gone"
        count={diff.gone_hosts.length}
        hosts={diff.gone_hosts}
        headerClass="text-danger"
      />
      <DiffSection
        title="Changed"
        count={diff.changed_hosts.length}
        hosts={diff.changed_hosts}
        headerClass="text-warning"
        showStatusChange
      />
      <section>
        <h4 className="text-xs font-medium text-text-muted mb-2">Unchanged</h4>
        <p className="text-xs text-text-muted">
          {diff.unchanged_count} hosts unchanged
        </p>
      </section>
    </div>
  );
}

// ── Detail panel ──────────────────────────────────────────────────────────────

interface DetailPanelProps {
  job: DiscoveryJobResponse;
  onClose: () => void;
}

function MetaRow({ label, value }: { label: string; value?: React.ReactNode }) {
  if (value === undefined || value === null || value === "") return null;
  return (
    <div className="flex gap-3 text-xs">
      <span className="text-text-muted w-24 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value}</span>
    </div>
  );
}

function DiscoveryDetailPanel({ job, onClose }: DetailPanelProps) {
  const title = job.name || `Discovery #${job.id}`;
  const [tab, setTab] = useState<"overview" | "changes">("overview");

  const isCompleted = job.status === "completed";

  const {
    data: diff,
    isLoading: diffLoading,
    isError: diffError,
  } = useDiscoveryDiff(job.id ?? "", isCompleted);

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Panel */}
      <div
        role="dialog"
        aria-label="Discovery job details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-110",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden",
          "shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-sm font-medium text-text-primary truncate">
              {title}
            </p>
            <p className="text-xs font-mono text-text-secondary">
              {job.networks?.join(", ") ?? "—"}
            </p>
            <StatusBadge status={job.status ?? "unknown"} />
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close panel"
            className="shrink-0 p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Tab bar */}
        <div className="border-b border-border px-5 shrink-0 flex gap-4">
          <button
            type="button"
            onClick={() => setTab("overview")}
            className={cn(
              "text-xs py-2 border-b-2 -mb-px transition-colors",
              tab === "overview"
                ? "border-accent text-text-primary font-medium"
                : "border-transparent text-text-muted hover:text-text-secondary",
            )}
          >
            Overview
          </button>
          <button
            type="button"
            onClick={() => isCompleted && setTab("changes")}
            disabled={!isCompleted}
            className={cn(
              "text-xs py-2 border-b-2 -mb-px transition-colors",
              tab === "changes"
                ? "border-accent text-text-primary font-medium"
                : "border-transparent text-text-muted",
              !isCompleted && "opacity-40 cursor-not-allowed",
            )}
          >
            Changes
          </button>
        </div>

        {/* Scrollable body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
          {tab === "overview" ? (
            <>
              {/* Progress (only when running) */}
              {job.status === "running" && (
                <section>
                  <h3 className="text-xs font-medium text-text-primary mb-3">
                    Progress
                  </h3>
                  <div className="space-y-1.5">
                    <div className="w-full bg-border rounded-full h-1">
                      <div
                        className="bg-accent h-1 rounded-full"
                        style={{ width: `${job.progress ?? 0}%` }}
                      />
                    </div>
                    <p className="text-xs text-text-muted">
                      {job.progress ?? 0}%
                    </p>
                  </div>
                </section>
              )}

              {/* Details */}
              <section>
                <h3 className="text-xs font-medium text-text-primary mb-3">
                  Details
                </h3>
                <div className="space-y-2">
                  <MetaRow label="ID" value={job.id} />
                  <MetaRow label="Network" value={job.networks?.join(", ")} />
                  <MetaRow
                    label="Method"
                    value={
                      job.method
                        ? (METHOD_LABELS[job.method] ?? job.method)
                        : undefined
                    }
                  />
                  <MetaRow label="Status" value={job.status} />
                </div>
              </section>

              {/* Timestamps */}
              <section>
                <h3 className="text-xs font-medium text-text-primary mb-3">
                  Timestamps
                </h3>
                <div className="space-y-2">
                  <MetaRow
                    label="Started"
                    value={
                      job.started_at
                        ? formatRelativeTime(job.started_at)
                        : undefined
                    }
                  />
                  <MetaRow
                    label="Created"
                    value={
                      job.created_at
                        ? formatRelativeTime(job.created_at)
                        : undefined
                    }
                  />
                </div>
              </section>
            </>
          ) : (
            <ChangesTab
              diff={diff}
              isLoading={diffLoading}
              isError={diffError}
            />
          )}
        </div>
      </div>
    </>
  );
}

// ── Compare panel ─────────────────────────────────────────────────────────────

interface ComparePanelProps {
  jobs: DiscoveryJobResponse[];
  runA: string;
  runB: string;
  onRunAChange: (id: string) => void;
  onRunBChange: (id: string) => void;
  result: DiscoveryCompareDiff | undefined;
  isLoading: boolean;
  error: unknown;
  onClose: () => void;
}

function ComparePanel({
  jobs,
  runA,
  runB,
  onRunAChange,
  onRunBChange,
  result,
  isLoading,
  error,
  onClose,
}: ComparePanelProps) {
  const completedJobs = jobs.filter((j) => j.status === "completed");

  const renderOption = (j: DiscoveryJobResponse) => {
    const label: string = j.name ?? j.networks?.join(", ") ?? j.id ?? "";
    const date = j.started_at
      ? new Date(j.started_at).toLocaleDateString()
      : "—";
    return (
      <option key={j.id} value={j.id ?? ""}>
        {label} · {date}
      </option>
    );
  };

  return (
    <div className="bg-surface border border-border rounded-lg p-4 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-text-primary">
          Compare Discovery Runs
        </h3>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close compare panel"
          className="text-text-muted hover:text-text-primary transition-colors"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Run selectors */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex flex-col gap-1 flex-1 min-w-36">
          <label className="text-xs text-text-muted">Baseline (A)</label>
          <select
            value={runA}
            onChange={(e) => onRunAChange(e.target.value)}
            aria-label="Baseline run A"
            className="bg-surface-raised border border-border rounded px-2 py-1.5 text-xs text-text-secondary focus:outline-none focus:border-accent"
          >
            <option value="">Select a run…</option>
            {completedJobs.map(renderOption)}
          </select>
        </div>

        <ArrowRight className="h-4 w-4 text-text-muted shrink-0 mt-4" />

        <div className="flex flex-col gap-1 flex-1 min-w-36">
          <label className="text-xs text-text-muted">Current (B)</label>
          <select
            value={runB}
            onChange={(e) => onRunBChange(e.target.value)}
            aria-label="Current run B"
            className="bg-surface-raised border border-border rounded px-2 py-1.5 text-xs text-text-secondary focus:outline-none focus:border-accent"
          >
            <option value="">Select a run…</option>
            {completedJobs.map(renderOption)}
          </select>
        </div>
      </div>

      {/* Results */}
      {isLoading && (
        <p className="text-xs text-text-muted">Loading comparison…</p>
      )}
      {!!error && !isLoading && (
        <p className="text-xs text-danger">
          {error instanceof Error ? error.message : "Failed to compare runs."}
        </p>
      )}
      {result && !isLoading && (
        <div className="space-y-3">
          {/* Summary counts */}
          <div className="flex items-center gap-4 text-xs flex-wrap">
            <span className="text-success">
              ● {result.new_hosts.length} new
            </span>
            <span className="text-danger">
              ● {result.gone_hosts.length} gone
            </span>
            <span
              className={
                result.changed_hosts.length > 0
                  ? "text-warning"
                  : "text-text-muted"
              }
            >
              ○ {result.changed_hosts.length} changed
            </span>
            <span className="text-text-muted">
              {result.unchanged_count} unchanged
            </span>
          </div>

          {/* Diff sections */}
          {result.new_hosts.length > 0 && (
            <DiffSection
              title="New"
              count={result.new_hosts.length}
              hosts={result.new_hosts}
              headerClass="text-success"
            />
          )}
          {result.gone_hosts.length > 0 && (
            <DiffSection
              title="Gone"
              count={result.gone_hosts.length}
              hosts={result.gone_hosts}
              headerClass="text-danger"
            />
          )}
          {result.changed_hosts.length > 0 && (
            <DiffSection
              title="Changed"
              count={result.changed_hosts.length}
              hosts={result.changed_hosts}
              headerClass="text-warning"
              showStatusChange
            />
          )}
          {result.new_hosts.length === 0 &&
            result.gone_hosts.length === 0 &&
            result.changed_hosts.length === 0 && (
              <p className="text-xs text-text-muted">
                No differences between these two runs.
              </p>
            )}
        </div>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function DiscoveryPage() {
  const search = useSearch({ from: "/discovery" });

  const [page, setPage] = useState(1);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(
    search.job ?? null,
  );
  const [showCreate, setShowCreate] = useState(false);

  // Compare state
  const [showCompare, setShowCompare] = useState(false);
  const [compareRunA, setCompareRunA] = useState("");
  const [compareRunB, setCompareRunB] = useState("");

  const queryParams = { page, page_size: PAGE_SIZE };
  const { data, isLoading } = useDiscoveryJobs(queryParams);

  const jobs = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  const { mutate: startDiscovery } = useStartDiscovery();
  const { mutate: stopDiscovery } = useStopDiscovery();

  const selectedJob = selectedJobId
    ? (jobs.find((j) => j.id === selectedJobId) ?? null)
    : null;

  const compareEnabled = showCompare && !!compareRunA && !!compareRunB;
  const {
    data: compareResult,
    isLoading: compareLoading,
    error: compareError,
  } = useDiscoveryCompare(compareRunA, compareRunB, compareEnabled);

  return (
    <>
      <div className="space-y-4">
        {/* Toolbar */}
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="secondary"
            onClick={() => setShowCompare((v) => !v)}
            icon={<GitCompare className="h-3.5 w-3.5" />}
          >
            Compare runs
          </Button>
          <Button
            onClick={() => setShowCreate(true)}
            icon={<Plus className="h-3.5 w-3.5" />}
          >
            New discovery job
          </Button>
        </div>

        {/* Compare panel */}
        {showCompare && (
          <ComparePanel
            jobs={jobs}
            runA={compareRunA}
            runB={compareRunB}
            onRunAChange={setCompareRunA}
            onRunBChange={setCompareRunB}
            result={compareResult}
            isLoading={compareLoading}
            error={compareError}
            onClose={() => {
              setShowCompare(false);
              setCompareRunA("");
              setCompareRunB("");
            }}
          />
        )}

        {/* Table card */}
        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border bg-surface">
                  <th className="text-left font-medium text-text-muted px-4 py-3 pr-4">
                    Name
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Network
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Method
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Status
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Progress
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Started
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Created
                  </th>
                  <th className="py-3" />
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <SkeletonRows count={5} />
                ) : jobs.length === 0 ? (
                  <tr>
                    <td
                      colSpan={8}
                      className="py-10 text-center text-xs text-text-muted"
                    >
                      No discovery jobs found.
                    </td>
                  </tr>
                ) : (
                  jobs.map((job) => (
                    <tr
                      key={job.id}
                      onClick={() => setSelectedJobId(job.id ?? null)}
                      className={cn(
                        "border-b border-border/50 last:border-0",
                        "hover:bg-surface-raised/50 transition-colors cursor-pointer",
                      )}
                    >
                      <td className="py-3 px-4 pr-4 text-text-secondary">
                        {job.name ?? "—"}
                      </td>
                      <td className="py-3 pr-4 font-mono text-text-secondary">
                        {job.networks?.join(", ") ?? "—"}
                      </td>
                      <td className="py-3 pr-4 text-text-secondary">
                        {job.method
                          ? (METHOD_LABELS[job.method] ?? job.method)
                          : "—"}
                      </td>
                      <td className="py-3 pr-4">
                        <StatusBadge status={job.status ?? "unknown"} />
                      </td>
                      <td className="py-3 pr-4">
                        {job.status === "running" ? (
                          <div className="w-full bg-border rounded-full h-1 min-w-16">
                            <div
                              className="bg-accent h-1 rounded-full"
                              style={{ width: `${job.progress ?? 0}%` }}
                            />
                          </div>
                        ) : (
                          <span className="text-text-muted">—</span>
                        )}
                      </td>
                      <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                        {job.started_at
                          ? formatRelativeTime(job.started_at)
                          : "—"}
                      </td>
                      <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                        {job.created_at
                          ? formatRelativeTime(job.created_at)
                          : "—"}
                      </td>
                      <td
                        className="py-3 pr-4"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {job.status === "pending" && (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => startDiscovery(job.id ?? "")}
                          >
                            ▶ Start
                          </Button>
                        )}
                        {job.status === "running" && (
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={() => stopDiscovery(job.id ?? "")}
                          >
                            ■ Stop
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {!isLoading && jobs.length > 0 && totalPages > 1 && (
            <div className="px-4 pb-3">
              <PaginationBar
                page={page}
                totalPages={totalPages}
                onPrev={() => setPage((p) => Math.max(1, p - 1))}
                onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
              />
            </div>
          )}
        </div>
      </div>

      {/* Detail panel */}
      {selectedJob && (
        <DiscoveryDetailPanel
          job={selectedJob}
          onClose={() => setSelectedJobId(null)}
        />
      )}

      {/* Create modal */}
      {showCreate && (
        <CreateDiscoveryModal onClose={() => setShowCreate(false)} />
      )}
    </>
  );
}
