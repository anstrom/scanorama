import {
  createHashHistory,
  createRouter,
  createRoute,
  createRootRoute,
} from "@tanstack/react-router";
import { z } from "zod";
import { RootLayout } from "./components/layout/root-layout";
import { DashboardPage } from "./routes/dashboard";
import { ScansPage } from "./routes/scans";
import { HostsPage } from "./routes/hosts";
import { NetworksPage } from "./routes/networks";
import { ExclusionsPage } from "./routes/exclusions";
import { DiscoveryPage } from "./routes/discovery";
import { ProfilesPage } from "./routes/profiles";
import { SchedulesPage } from "./routes/schedules";
import { AdminPage } from "./routes/admin";

const rootRoute = createRootRoute({
  component: RootLayout,
});

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: DashboardPage,
});

const scansRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/scans",
  component: ScansPage,
});

const hostsSearchSchema = z.object({
  filter: z.string().optional(),
});

const hostsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/hosts",
  component: HostsPage,
  validateSearch: hostsSearchSchema,
});

const networksRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/networks",
  component: NetworksPage,
});

const exclusionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/exclusions",
  component: ExclusionsPage,
});

const discoverySearchSchema = z.object({
  job: z.string().optional(),
});

const discoveryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/discovery",
  component: DiscoveryPage,
  validateSearch: discoverySearchSchema,
});

const profilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/profiles",
  component: ProfilesPage,
});

const schedulesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/schedules",
  component: SchedulesPage,
});

const adminRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  component: AdminPage,
});

const routeTree = rootRoute.addChildren([
  dashboardRoute,
  scansRoute,
  hostsRoute,
  networksRoute,
  exclusionsRoute,
  discoveryRoute,
  profilesRoute,
  schedulesRoute,
  adminRoute,
]);

const hashHistory = createHashHistory();

export const router = createRouter({
  routeTree,
  history: hashHistory,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
