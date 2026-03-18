import { render, act, type RenderOptions } from "@testing-library/react";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  createRoute,
  RouterProvider,
  Outlet,
} from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, act as actHook } from "@testing-library/react";
import type { ReactNode } from "react";

/**
 * Creates a fresh QueryClient for each test — no retries, no caching,
 * so tests are isolated and failures surface immediately.
 */
export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

/**
 * Wrapper that provides a QueryClient context. Pass to renderHook's `wrapper`
 * option, or use renderHookWithQuery below.
 */
export function createQueryWrapper() {
  const queryClient = createTestQueryClient();
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  }
  return { Wrapper, queryClient };
}

/**
 * Renders a hook inside a QueryClientProvider. Returns the renderHook result
 * plus the queryClient so tests can inspect cache state if needed.
 */
export function renderHookWithQuery<T>(hook: () => T) {
  const { Wrapper, queryClient } = createQueryWrapper();
  const result = renderHook(hook, { wrapper: Wrapper });
  return { ...result, queryClient, actHook };
}

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
