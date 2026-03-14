import { cn } from "../../lib/utils";
import { Activity } from "lucide-react";
import { useHealth } from "../../api/hooks/use-system";

interface TopbarProps {
  title: string;
}

export function Topbar({ title }: TopbarProps) {
  const { data: health } = useHealth();
  const isHealthy = health?.status === "healthy";

  return (
    <header className="flex items-center justify-between h-12 px-4 border-b border-border bg-surface">
      <h1 className="text-sm font-medium text-text-primary">{title}</h1>
      <div className="flex items-center gap-2 text-xs">
        <Activity
          className={cn(
            "h-3.5 w-3.5",
            isHealthy ? "text-success" : "text-danger"
          )}
        />
        <span className="text-text-muted">
          {isHealthy ? "Healthy" : "Unhealthy"}
        </span>
      </div>
    </header>
  );
}
