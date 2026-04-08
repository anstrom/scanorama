import { useEffect } from "react";
import { Outlet, useRouterState, useNavigate } from "@tanstack/react-router";
import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";
import { ToastProvider, useToast } from "../toast-provider";
import { useWs } from "../../lib/use-ws";
import type { WsMessage } from "../../lib/ws";

const routeTitles: Record<string, string> = {
  "/": "Dashboard",
  "/scans": "Scans",
  "/hosts": "Hosts",
  "/discovery": "Discovery",
  "/networks": "Networks",
  "/profiles": "Profiles",
  "/schedules": "Schedules",
  "/admin": "Admin",
};

// ── WS payload for discovery_update messages ───────────────────────────────────

interface DiscoveryUpdateData {
  job_id: string;
  status: string;
  hosts_found?: number;
  new_hosts?: number;
  gone_hosts?: number;
  changed_hosts?: number;
  message?: string;
}

// ── Discovery completion notifications ────────────────────────────────────────
// Must live inside <ToastProvider> to access useToast().

function DiscoveryNotifications() {
  const { manager } = useWs();
  const { toast } = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    if (!manager) return;

    return manager.on("discovery_update", (msg: WsMessage) => {
      const data = msg.data as DiscoveryUpdateData;
      if (data.status !== "completed") return;

      const parts: string[] = [];
      if (data.new_hosts) parts.push(`${data.new_hosts} new`);
      if (data.gone_hosts) parts.push(`${data.gone_hosts} gone`);
      if (data.changed_hosts) parts.push(`${data.changed_hosts} changed`);
      const summary = parts.length > 0 ? parts.join(", ") : "no changes";
      const message = `Discovery completed: ${summary}. Click to view.`;

      const jobId = data.job_id;
      toast.success(message, () => {
        void navigate({ to: "/discovery", search: { job: jobId } });
      });
    });
  }, [manager, toast, navigate]);

  return null;
}

// ── Root layout ────────────────────────────────────────────────────────────────

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const title = routeTitles[pathname] ?? "Scanorama";

  return (
    <ToastProvider>
      <DiscoveryNotifications />
      <div className="flex h-screen overflow-hidden">
        <Sidebar />
        <div className="flex flex-col flex-1 min-w-0">
          <Topbar title={title} />
          <main className="flex-1 overflow-y-auto p-4">
            <Outlet />
          </main>
        </div>
      </div>
    </ToastProvider>
  );
}
