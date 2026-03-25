import { cn } from "../../lib/utils";
import { Activity } from "lucide-react";
import { useHealth } from "../../api/hooks/use-system";
import { useWsStatus } from "../../lib/use-ws";
import type { WsStatus } from "../../lib/ws";

interface TopbarProps {
  title: string;
}

function wsDotClass(status: WsStatus): string {
  switch (status) {
    case "connected":
      return "bg-success";
    case "connecting":
      return "bg-warning animate-pulse";
    case "error":
      return "bg-danger";
    default:
      return "bg-text-muted";
  }
}

function wsLabel(status: WsStatus): string {
  switch (status) {
    case "connected":
      return "Live";
    case "connecting":
      return "Connecting…";
    case "error":
      return "WS Error";
    default:
      return "Offline";
  }
}

export function Topbar({ title }: TopbarProps) {
  const { data: health } = useHealth();
  const isHealthy = health?.status === "healthy";
  const wsStatus = useWsStatus();

  return (
    <header className="flex items-center justify-between h-12 px-4 border-b border-border bg-surface">
      <h1 className="text-sm font-medium text-text-primary">{title}</h1>

      <div className="flex items-center gap-3 text-xs">
        {/* WebSocket status */}
        <div
          className="flex items-center gap-1.5"
          title={`WebSocket: ${wsStatus}`}
        >
          <span
            className={cn(
              "w-2 h-2 rounded-full shrink-0",
              wsDotClass(wsStatus),
            )}
          />
          <span
            className={cn(
              "text-text-muted",
              wsStatus === "connected" && "text-success",
              wsStatus === "error" && "text-danger",
            )}
          >
            {wsLabel(wsStatus)}
          </span>
        </div>

        {/* Separator */}
        <span className="w-px h-3 bg-border" />

        {/* API health */}
        <div className="flex items-center gap-1.5">
          <Activity
            className={cn(
              "h-3.5 w-3.5",
              isHealthy ? "text-success" : "text-danger",
            )}
          />
          <span className="text-text-muted">
            {isHealthy ? "Healthy" : "Unhealthy"}
          </span>
        </div>
      </div>
    </header>
  );
}
