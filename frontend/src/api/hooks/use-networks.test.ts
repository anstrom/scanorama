import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { useNetworks, useNetworkStats } from "./use-networks";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);

const ok = (data: unknown): ReturnType<typeof mockGet> =>
  Promise.resolve({
    data,
    error: undefined,
    response: new Response(),
  }) as ReturnType<typeof mockGet>;

const fail = (message = "something went wrong"): ReturnType<typeof mockGet> =>
  Promise.resolve({
    data: undefined,
    error: { message },
    response: new Response(),
  }) as ReturnType<typeof mockGet>;

const mockNetworks = [
  {
    id: "net-1",
    name: "Office LAN",
    cidr: "192.168.1.0/24",
    is_active: true,
    created_at: "2024-01-01T00:00:00Z",
  },
  {
    id: "net-2",
    name: "DMZ",
    cidr: "10.0.0.0/8",
    is_active: false,
    created_at: "2024-01-02T00:00:00Z",
  },
];

const mockPagination = {
  page: 1,
  page_size: 20,
  total_items: 2,
  total_pages: 1,
};

const mockStats = {
  networks: { total: 3, active: 2 },
  hosts: { total: 42, active: 30 },
  exclusions: { total: 5 },
};

// ── useNetworks ───────────────────────────────────────────────────────────────

describe("useNetworks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetworks());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();
  });

  it("returns network list and pagination on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockNetworks, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useNetworks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].name).toBe("Office LAN");
    expect(result.current.data?.data?.[1].name).toBe("DMZ");
    expect(result.current.data?.pagination?.total_items).toBe(2);
    expect(result.current.data?.pagination?.total_pages).toBe(1);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal error"));

    const { result } = renderHookWithQuery(() => useNetworks());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useNetworks({ page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/networks",
      expect.objectContaining({
        params: { query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("returns an empty list when the API returns no networks", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: { ...mockPagination, total_items: 0 } }),
    );

    const { result } = renderHookWithQuery(() => useNetworks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(0);
  });

  it("calls api.GET with the /networks path", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockNetworks, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useNetworks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/networks",
      expect.objectContaining({ params: { query: {} } }),
    );
  });

  it("caches the result under the ['networks', params] query key", async () => {
    const params = { page: 1, page_size: 20 };
    mockGet.mockResolvedValue(
      ok({ data: mockNetworks, pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() =>
      useNetworks(params),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["networks", params]);
    expect(cached).toBeDefined();
  });
});

// ── useNetworkStats ───────────────────────────────────────────────────────────

describe("useNetworkStats", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetworkStats());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns stats data on success", async () => {
    mockGet.mockResolvedValue(ok(mockStats));

    const { result } = renderHookWithQuery(() => useNetworkStats());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.networks).toEqual({ total: 3, active: 2 });
    expect(result.current.data?.hosts).toEqual({ total: 42, active: 30 });
    expect(result.current.data?.exclusions).toEqual({ total: 5 });
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("service unavailable"));

    const { result } = renderHookWithQuery(() => useNetworkStats());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the /networks/stats path", async () => {
    mockGet.mockResolvedValue(ok(mockStats));

    const { result } = renderHookWithQuery(() => useNetworkStats());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith("/networks/stats");
  });

  it("caches the result under the ['networks', 'stats'] query key", async () => {
    mockGet.mockResolvedValue(ok(mockStats));

    const { result, queryClient } = renderHookWithQuery(() =>
      useNetworkStats(),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["networks", "stats"]);
    expect(cached).toBeDefined();
    expect((cached as typeof mockStats).networks).toEqual({
      total: 3,
      active: 2,
    });
  });
});
