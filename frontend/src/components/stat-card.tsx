import type { ElementType } from "react";
import { cn } from "../lib/utils";

interface StatCardProps {
  label: string;
  value: string | number;
  subtext?: string;
  icon?: ElementType;
  trend?: { value: number; label?: string };
  loading?: boolean;
}

export function StatCard({
  label,
  value,
  subtext,
  icon: Icon,
  trend,
  loading = false,
}: StatCardProps) {
  if (loading) {
    return (
      <div className="bg-surface rounded-lg border border-border p-4">
        <div className="animate-pulse space-y-2">
          <div className="h-3 w-20 rounded bg-surface-raised" />
          <div className="h-7 w-16 rounded bg-surface-raised" />
          <div className="h-3 w-24 rounded bg-surface-raised" />
        </div>
      </div>
    );
  }

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <div className="flex items-center justify-between mb-1">
        <p className="text-xs text-text-muted">{label}</p>
        {Icon && <Icon className="h-4 w-4 text-text-muted" />}
      </div>
      <p className="text-2xl font-semibold text-text-primary">{value}</p>
      {(subtext || trend) && (
        <div className="flex items-center gap-1.5 mt-1">
          {trend && (
            <span
              className={cn(
                "text-xs font-medium",
                trend.value > 0
                  ? "text-success"
                  : trend.value < 0
                    ? "text-danger"
                    : "text-text-muted",
              )}
            >
              {trend.value > 0 ? "+" : ""}
              {trend.value}%{trend.label ? ` ${trend.label}` : ""}
            </span>
          )}
          {subtext && <p className="text-xs text-text-secondary">{subtext}</p>}
        </div>
      )}
    </div>
  );
}
