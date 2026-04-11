import { useEffect, useState } from "react";
import { cn } from "../../lib/utils";
import {
  LayoutDashboard,
  Radar,
  Server,
  Network,
  ScanSearch,
  SlidersHorizontal,
  Clock,
  Shield,
  ShieldOff,
  Layers,
  Database,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";

interface NavItem {
  label: string;
  href: string;
  icon: React.ElementType;
}

const mainNav: NavItem[] = [
  { label: "Dashboard", href: "#/", icon: LayoutDashboard },
  { label: "Scans", href: "#/scans", icon: Radar },
  { label: "Hosts", href: "#/hosts", icon: Server },
  { label: "Discovery", href: "#/discovery", icon: ScanSearch },
  { label: "Networks", href: "#/networks", icon: Network },
  { label: "Exclusions", href: "#/exclusions", icon: ShieldOff },
  { label: "Groups", href: "#/groups", icon: Layers },
  { label: "Profiles", href: "#/profiles", icon: SlidersHorizontal },
  { label: "Schedules", href: "#/schedules", icon: Clock },
  { label: "Port Database", href: "#/ports", icon: Database },
];

const adminNav: NavItem[] = [{ label: "Admin", href: "#/admin", icon: Shield }];

function useHashPath(): string {
  const [path, setPath] = useState(
    () => window.location.hash.replace(/^#/, "") || "/",
  );

  useEffect(() => {
    const onHashChange = () => {
      setPath(window.location.hash.replace(/^#/, "") || "/");
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  return path;
}

function NavLink({
  item,
  collapsed,
  active,
}: {
  item: NavItem;
  collapsed: boolean;
  active: boolean;
}) {
  const Icon = item.icon;
  return (
    <a
      href={item.href}
      className={cn(
        "flex items-center gap-2 px-2 py-1.5 rounded text-sm transition-colors",
        active
          ? "bg-accent/15 text-accent"
          : "text-text-secondary hover:text-text-primary hover:bg-surface-raised",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && <span className="truncate">{item.label}</span>}
    </a>
  );
}

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false);
  const currentPath = useHashPath();

  function renderNavItems(items: NavItem[]) {
    return items.map((item) => {
      const routePath = item.href.replace(/^#/, "");
      const active =
        routePath === "/"
          ? currentPath === "/"
          : currentPath.startsWith(routePath);
      return (
        <NavLink
          key={item.href}
          item={item}
          collapsed={collapsed}
          active={active}
        />
      );
    });
  }

  return (
    <aside
      className={cn(
        "flex flex-col h-full bg-surface border-r border-border transition-all duration-200",
        collapsed ? "w-14" : "w-56",
      )}
    >
      {/* Logo area */}
      <div className="flex items-center h-12 px-3 border-b border-border gap-2">
        <Radar className="h-5 w-5 text-accent shrink-0" />
        {!collapsed && (
          <span className="font-semibold text-sm text-text-primary truncate">
            Scanorama
          </span>
        )}
      </div>

      {/* Main navigation */}
      <nav className="flex-1 py-2 px-2 space-y-0.5 overflow-y-auto">
        {renderNavItems(mainNav)}
      </nav>

      {/* Divider + admin nav */}
      <div className="px-2 pb-2 space-y-0.5">
        <div className="border-t border-border my-1" />
        {renderNavItems(adminNav)}

        {/* Collapse toggle */}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="flex items-center gap-2 px-2 py-1.5 rounded text-sm text-text-muted hover:text-text-secondary transition-colors w-full"
        >
          {collapsed ? (
            <PanelLeftOpen className="h-4 w-4 shrink-0" />
          ) : (
            <>
              <PanelLeftClose className="h-4 w-4 shrink-0" />
              <span className="truncate">Collapse</span>
            </>
          )}
        </button>
      </div>
    </aside>
  );
}
