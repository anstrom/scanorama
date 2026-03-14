import { cn } from "../lib/utils";
import { STATUS_BG_COLORS, type StatusKey } from "../lib/constants";

interface StatusBadgeProps {
  status: string;
  className?: string;
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const normalized = status.toLowerCase();
  const style =
    STATUS_BG_COLORS[normalized as StatusKey] ??
    "bg-text-muted/15 text-text-muted";

  return (
    <span
      className={cn(
        "inline-flex items-center px-2 py-0.5 rounded text-xs font-medium",
        style,
        className,
      )}
    >
      {status}
    </span>
  );
}
