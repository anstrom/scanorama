import { useState, useMemo } from "react";
import { ShieldOff, Plus, Trash2 } from "lucide-react";
import { SortHeader } from "../components/sort-header";
import type { SortOrder } from "../components/sort-header";
import { Button } from "../components/button";
import {
  useGlobalExclusions,
  useDeleteExclusion,
} from "../api/hooks/use-networks";
import { Skeleton } from "../components";
import { AddExclusionModal } from "../components/add-exclusion-modal";
import { formatAbsoluteTime, formatRelativeTime, cn } from "../lib/utils";
import type { components } from "../api/types";

type NetworkExclusionResponse =
  components["schemas"]["docs.NetworkExclusionResponse"];

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function SkeletonRows() {
  return (
    <>
      {Array.from({ length: 5 }).map((_, i) => (
        <tr key={i} className="border-b border-border">
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-36 font-mono" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-48" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-28" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-6" />
          </td>
        </tr>
      ))}
    </>
  );
}

// ── Exclusions page ───────────────────────────────────────────────────────────

export function ExclusionsPage() {
  const [showAdd, setShowAdd] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState("excluded_cidr");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");

  const { data: exclusions, isLoading } = useGlobalExclusions();
  const { mutate: deleteExclusion, isPending: isDeleting } =
    useDeleteExclusion();

  function handleSort(column: string) {
    if (sortBy === column) {
      setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
    } else {
      setSortBy(column);
      setSortOrder("asc");
    }
  }

  const sortedList = useMemo(() => {
    const raw: NetworkExclusionResponse[] = exclusions ?? [];
    if (!sortBy) return raw;
    return [...raw].sort((a, b) => {
      const aVal = a[sortBy as keyof typeof a] ?? "";
      const bVal = b[sortBy as keyof typeof b] ?? "";
      const cmp = String(aVal).localeCompare(String(bVal));
      return sortOrder === "asc" ? cmp : -cmp;
    });
  }, [exclusions, sortBy, sortOrder]);

  function handleDelete(id: string) {
    if (confirmDeleteId !== id) {
      setConfirmDeleteId(id);
      return;
    }
    deleteExclusion(id, {
      onSettled: () => setConfirmDeleteId(null),
    });
  }

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <ShieldOff className="h-4 w-4 text-text-muted" />
          <p className="text-xs text-text-muted leading-relaxed">
            Global exclusions apply to all networks — hosts in these CIDR ranges
            will never be scanned or discovered.
          </p>
        </div>

        <div className="flex-1" />

        <Button
          onClick={() => setShowAdd(true)}
          className="text-xs h-7 px-3 shrink-0"
        >
          <Plus className="h-3 w-3 mr-1" />
          Add exclusion
        </Button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded border border-border">
        <table className="w-full text-xs border-collapse min-w-140">
          <thead>
            <tr className="bg-surface-raised border-b border-border text-left">
              <SortHeader
                label="CIDR Block"
                column="excluded_cidr"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
                className="px-4 py-2.5"
              />
              <SortHeader
                label="Reason"
                column="reason"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
                className="px-4 py-2.5"
              />
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Created By
              </th>
              <SortHeader
                label="Created At"
                column="created_at"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
                className="px-4 py-2.5"
              />
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap w-16">
                {/* actions */}
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <SkeletonRows />
            ) : sortedList.length === 0 ? (
              <tr>
                <td
                  colSpan={5}
                  className="px-4 py-10 text-center text-text-muted"
                >
                  No global exclusions defined.
                </td>
              </tr>
            ) : (
              sortedList.map((excl) => (
                <tr
                  key={excl.id}
                  className={cn(
                    "border-b border-border group transition-colors",
                    "hover:bg-surface-raised",
                  )}
                >
                  {/* CIDR */}
                  <td className="px-4 py-2.5 font-mono text-text-primary whitespace-nowrap">
                    {excl.excluded_cidr ?? "—"}
                  </td>

                  {/* Reason */}
                  <td className="px-4 py-2.5 text-text-secondary max-w-65 truncate">
                    {excl.reason ?? (
                      <span className="italic text-text-muted">—</span>
                    )}
                  </td>

                  {/* Created by */}
                  <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                    {excl.created_by ?? "—"}
                  </td>

                  {/* Created at */}
                  <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                    {excl.created_at ? (
                      <span title={formatAbsoluteTime(excl.created_at)}>
                        {formatRelativeTime(excl.created_at)}
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>

                  {/* Delete action */}
                  <td className="px-4 py-2.5 text-right whitespace-nowrap">
                    {confirmDeleteId === excl.id ? (
                      <div className="flex items-center justify-end gap-2">
                        <button
                          type="button"
                          onClick={() => setConfirmDeleteId(null)}
                          className="text-[11px] text-text-muted hover:text-text-secondary"
                        >
                          Cancel
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDelete(excl.id ?? "")}
                          disabled={isDeleting}
                          className="text-[11px] text-danger hover:text-danger/80 font-medium"
                        >
                          Confirm
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        onClick={() => handleDelete(excl.id ?? "")}
                        aria-label={`Delete exclusion ${excl.excluded_cidr}`}
                        className="p-1 rounded text-text-muted opacity-0 group-hover:opacity-100 hover:text-danger transition-all"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Add exclusion modal */}
      {showAdd && <AddExclusionModal onClose={() => setShowAdd(false)} />}
    </div>
  );
}
