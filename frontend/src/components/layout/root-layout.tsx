import { Outlet, useRouterState } from "@tanstack/react-router";
import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

const routeTitles: Record<string, string> = {
  "/": "Dashboard",
  "/scans": "Scans",
  "/hosts": "Hosts",
  "/networks": "Networks",

  "/profiles": "Profiles",
  "/schedules": "Schedules",
  "/admin": "Admin",
};

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const title = routeTitles[pathname] ?? "Scanorama";

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex flex-col flex-1 min-w-0">
        <Topbar title={title} />
        <main className="flex-1 overflow-y-auto p-4">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
