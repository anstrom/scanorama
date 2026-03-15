import { render, type RenderOptions } from "@testing-library/react";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  createRoute,
  RouterProvider,
  Outlet,
} from "@tanstack/react-router";
import type { ReactNode } from "react";

/**
 * Wraps the given UI in a minimal TanStack Router context so that any
 * component that calls useRouter / Link / etc. does not throw
 * "useRouter must be used inside a RouterProvider".
 */
export function renderWithRouter(
  ui: ReactNode,
  options?: Omit<RenderOptions, "wrapper">,
) {
  const rootRoute = createRootRoute({ component: Outlet });
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <>{ui}</>,
  });

  const router = createRouter({
    routeTree: rootRoute.addChildren([indexRoute]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });

  return render(<RouterProvider router={router} />, options);
}
