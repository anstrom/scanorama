import { cn } from "../lib/utils";

interface PaginationBarProps {
  page: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
  className?: string;
}

export function PaginationBar({
  page,
  totalPages,
  onPrev,
  onNext,
  className,
}: PaginationBarProps) {
  const isFirst = page <= 1;
  const isLast = page >= totalPages;

  return (
    <div
      className={cn(
        "flex items-center justify-between gap-4 pt-3 border-t border-border",
        className,
      )}
    >
      <button
        type="button"
        onClick={onPrev}
        disabled={isFirst}
        className={cn(
          "px-3 py-1.5 rounded text-xs font-medium transition-colors",
          "border border-border",
          isFirst
            ? "text-text-muted bg-surface cursor-not-allowed opacity-50"
            : "text-text-secondary bg-surface hover:bg-surface-raised hover:text-text-primary cursor-pointer",
        )}
        aria-label="Previous page"
      >
        Previous
      </button>

      <span className="text-xs text-text-muted tabular-nums">
        Page {page} of {totalPages}
      </span>

      <button
        type="button"
        onClick={onNext}
        disabled={isLast}
        className={cn(
          "px-3 py-1.5 rounded text-xs font-medium transition-colors",
          "border border-border",
          isLast
            ? "text-text-muted bg-surface cursor-not-allowed opacity-50"
            : "text-text-secondary bg-surface hover:bg-surface-raised hover:text-text-primary cursor-pointer",
        )}
        aria-label="Next page"
      >
        Next
      </button>
    </div>
  );
}
