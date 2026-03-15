import { render, act, type RenderOptions } from "@testing-library/react";
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
 *
 * Returns a promise so callers can await the fully-rendered output after
 * the router's async initialisation (Transitioner) has completed.
 */
export async function renderWithRouter(
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

  let result!: ReturnType<typeof render>;

  await act(async () => {
    result = render(<RouterProvider router={router} />, options);
  });

  return result;
}
