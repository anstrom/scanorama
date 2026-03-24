import { useState } from "react";
import { X, Plus } from "lucide-react";
import { Button } from "../components/button";
import {
  useDiscoveryJobs,
  useStartDiscovery,
  useStopDiscovery,
} from "../api/hooks/use-discovery";
import { StatusBadge, Skeleton, PaginationBar } from "../components";
import { CreateDiscoveryModal } from "../components/create-discovery-modal";
import { formatRelativeTime } from "../lib/utils";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type DiscoveryJobResponse = components["schemas"]["docs.DiscoveryJobResponse"];

const PAGE_SIZE = 20;

const METHOD_LABELS: Record<string, string> = {
  tcp: "TCP",
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
  const title = job.name ?? `Discovery #${job.id}`;

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
              {job.networks ?? "—"}
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

        {/* Scrollable body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
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
                <p className="text-xs text-text-muted">{job.progress ?? 0}%</p>
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
              <MetaRow label="Network" value={job.networks} />
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
        </div>
      </div>
    </>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function DiscoveryPage() {
  const [page, setPage] = useState(1);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

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

  return (
    <>
      <div className="space-y-4">
        {/* Toolbar */}
        <div className="flex justify-end">
          <Button
            onClick={() => setShowCreate(true)}
            icon={<Plus className="h-3.5 w-3.5" />}
          >
            New discovery job
          </Button>
        </div>

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
                        {job.networks ?? "—"}
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
