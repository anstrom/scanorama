import { ChevronDown, ChevronUp, ChevronsUpDown } from "lucide-react";
import { cn } from "../lib/utils";

export type SortOrder = "asc" | "desc";

export interface SortHeaderProps {
  label: string;
  column: string;
  sortBy: string;
  sortOrder: SortOrder;
  onSort: (col: string) => void;
  className?: string;
}

export function SortHeader({
  label,
  column,
  sortBy,
  sortOrder,
  onSort,
  className,
}: SortHeaderProps) {
  const active = sortBy === column;
  return (
    <th
      onClick={() => onSort(column)}
      className={cn(
        "text-left font-medium text-text-muted py-3 pr-4",
        "cursor-pointer select-none hover:text-text-secondary transition-colors whitespace-nowrap",
        active && "text-text-secondary",
        className,
      )}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active ? (
          sortOrder === "asc" ? (
            <ChevronUp className="h-3 w-3 shrink-0" />
          ) : (
            <ChevronDown className="h-3 w-3 shrink-0" />
          )
        ) : (
          <ChevronsUpDown className="h-3 w-3 shrink-0 opacity-30" />
        )}
      </span>
    </th>
  );
}
