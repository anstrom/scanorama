import { useState } from "react";
import { Database, Search, X } from "lucide-react";
import { usePorts, usePortCategories } from "../api/hooks/use-ports";
import type { PortDefinition } from "../api/hooks/use-ports";
import { Skeleton } from "../components";
import { cn } from "../lib/utils";

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function SkeletonRows() {
  return (
    <>
      {Array.from({ length: 10 }).map((_, i) => (
        <tr key={i} className="border-b border-border">
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-12 font-mono" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-10" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-24" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-48" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-16" />
          </td>
        </tr>
      ))}
    </>
  );
}

// ── Category badge ────────────────────────────────────────────────────────────

const categoryColors: Record<string, string> = {
  web: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300",
  database:
    "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300",
  windows:
    "bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300",
  remote:
    "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300",
  email:
    "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300",
  messaging:
    "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300",
  network:
    "bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300",
  monitoring:
    "bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300",
  container:
    "bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300",
  iot: "bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300",
  security:
    "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300",
  linux:
    "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300",
  transfer:
    "bg-lime-100 text-lime-700 dark:bg-lime-900/30 dark:text-lime-300",
  proxy:
    "bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-300",
};

function CategoryBadge({ category }: { category?: string }) {
  if (!category) return null;
  const cls =
    categoryColors[category] ??
    "bg-muted text-muted-foreground";
  return (
    <span
      className={cn(
        "inline-block rounded px-1.5 py-0.5 text-[11px] font-medium capitalize",
        cls,
      )}
    >
      {category}
    </span>
  );
}

// ── Row ───────────────────────────────────────────────────────────────────────

function PortRow({ port }: { port: PortDefinition }) {
  return (
    <tr className="border-b border-border hover:bg-muted/30 transition-colors">
      <td className="px-4 py-2.5 font-mono text-sm font-semibold text-foreground">
        {port.port}
      </td>
      <td className="px-4 py-2.5 text-xs text-muted-foreground uppercase">
        {port.protocol}
      </td>
      <td className="px-4 py-2.5 text-sm font-medium text-foreground">
        {port.service}
      </td>
      <td className="px-4 py-2.5 text-sm text-muted-foreground max-w-xs">
        {port.description ?? "—"}
      </td>
      <td className="px-4 py-2.5">
        <CategoryBadge category={port.category} />
      </td>
      <td className="px-4 py-2.5 text-xs text-muted-foreground">
        {port.os_families && port.os_families.length > 0
          ? port.os_families.join(", ")
          : "—"}
      </td>
    </tr>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

const PAGE_SIZE = 50;

export function PortsPage() {
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [category, setCategory] = useState("");
  const [protocol, setProtocol] = useState("");
  const [page, setPage] = useState(1);
  const [searchTimer, setSearchTimer] = useState<ReturnType<
    typeof setTimeout
  > | null>(null);

  const { data: categories } = usePortCategories();
  const { data, isLoading } = usePorts({
    search: debouncedSearch,
    category,
    protocol,
    page,
    page_size: PAGE_SIZE,
  });

  function handleSearchChange(value: string) {
    setSearch(value);
    if (searchTimer) clearTimeout(searchTimer);
    const t = setTimeout(() => {
      setDebouncedSearch(value);
      setPage(1);
    }, 300);
    setSearchTimer(t);
  }

  function handleCategoryChange(value: string) {
    setCategory(value);
    setPage(1);
  }

  function handleProtocolChange(value: string) {
    setProtocol(value);
    setPage(1);
  }

  function clearFilters() {
    setSearch("");
    setDebouncedSearch("");
    setCategory("");
    setProtocol("");
    setPage(1);
  }

  const hasFilters = search || category || protocol;
  const ports = data?.ports ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.total_pages ?? 1;

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-border">
        <div className="flex items-center gap-2">
          <Database className="h-5 w-5 text-muted-foreground" />
          <h1 className="text-lg font-semibold">Port Database</h1>
          {total > 0 && (
            <span className="text-sm text-muted-foreground ml-1">
              ({total.toLocaleString()} entries)
            </span>
          )}
        </div>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-border flex-wrap">
        {/* Search input */}
        <div className="relative flex-1 min-w-48 max-w-xs">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
          <input
            type="text"
            placeholder="Search port or service…"
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="w-full pl-8 pr-3 py-1.5 text-sm rounded border border-input bg-background focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        {/* Category filter */}
        <select
          value={category}
          onChange={(e) => handleCategoryChange(e.target.value)}
          className="text-sm rounded border border-input bg-background px-2.5 py-1.5 focus:outline-none focus:ring-1 focus:ring-ring"
        >
          <option value="">All categories</option>
          {(categories ?? []).map((c) => (
            <option key={c} value={c}>
              {c.charAt(0).toUpperCase() + c.slice(1)}
            </option>
          ))}
        </select>

        {/* Protocol filter */}
        <select
          value={protocol}
          onChange={(e) => handleProtocolChange(e.target.value)}
          className="text-sm rounded border border-input bg-background px-2.5 py-1.5 focus:outline-none focus:ring-1 focus:ring-ring"
        >
          <option value="">TCP + UDP</option>
          <option value="tcp">TCP only</option>
          <option value="udp">UDP only</option>
        </select>

        {hasFilters && (
          <button
            onClick={clearFilters}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            <X className="h-3 w-3" />
            Clear filters
          </button>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-background border-b border-border">
            <tr>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide w-20">
                Port
              </th>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide w-16">
                Proto
              </th>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide w-36">
                Service
              </th>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide">
                Description
              </th>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide w-28">
                Category
              </th>
              <th className="text-left px-4 py-2.5 font-medium text-muted-foreground text-xs uppercase tracking-wide w-32">
                OS Families
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <SkeletonRows />
            ) : ports.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-4 py-12 text-center text-muted-foreground"
                >
                  {hasFilters
                    ? "No ports match the current filters."
                    : "No port definitions found."}
                </td>
              </tr>
            ) : (
              ports.map((p) => (
                <PortRow key={`${p.port}-${p.protocol}`} port={p} />
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between px-6 py-3 border-t border-border text-sm">
          <span className="text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              className="px-3 py-1 rounded border border-input disabled:opacity-40 hover:bg-muted transition-colors"
            >
              Previous
            </button>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className="px-3 py-1 rounded border border-input disabled:opacity-40 hover:bg-muted transition-colors"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
