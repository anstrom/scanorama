import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useHosts,
  useHost,
  useActiveHostCount,
  useHostScans,
  useUpdateHost,
  useDeleteHost,
} from "./use-hosts";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
    PUT: vi.fn(),
    DELETE: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);
const mockPut = vi.mocked(api.PUT);
const mockDelete = vi.mocked(api.DELETE);

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

const okPut = (data: unknown): ReturnType<typeof mockPut> =>
  Promise.resolve({
    data,
    error: undefined,
    response: new Response(),
  }) as ReturnType<typeof mockPut>;

const failPut = (
  message = "something went wrong",
): ReturnType<typeof mockPut> =>
  Promise.resolve({
    data: undefined,
    error: { message },
    response: new Response(),
  }) as ReturnType<typeof mockPut>;

const okDelete = (): ReturnType<typeof mockDelete> =>
  Promise.resolve({
    data: undefined,
    error: undefined,
    response: new Response(),
  }) as ReturnType<typeof mockDelete>;

const failDelete = (
  message = "something went wrong",
): ReturnType<typeof mockDelete> =>
  Promise.resolve({
    data: undefined,
    error: { message },
    response: new Response(),
  }) as ReturnType<typeof mockDelete>;

const mockHost = {
  id: "host-1",
  ip_address: "192.168.1.100",
  hostname: "server01.local",
  status: "up",
  mac_address: "00:1B:44:11:3A:B7",
  os_name: "Linux",
  scan_count: 3,
  open_ports: [22, 80, 443],
  first_seen: "2024-01-01T00:00:00Z",
  last_seen: "2024-06-01T12:00:00Z",
};

const mockHost2 = {
  id: "host-2",
  ip_address: "192.168.1.101",
  hostname: "desktop.local",
  status: "down",
  mac_address: undefined,
  os_name: undefined,
  scan_count: 1,
  open_ports: [],
  first_seen: "2024-02-01T00:00:00Z",
  last_seen: "2024-05-01T00:00:00Z",
};

const mockPagination = {
  page: 1,
  page_size: 20,
  total_items: 2,
  total_pages: 1,
};

// ── useHosts ──────────────────────────────────────────────────────────────────

describe("useHosts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useHosts());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns host list and pagination on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [mockHost, mockHost2], pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useHosts());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].ip_address).toBe("192.168.1.100");
    expect(result.current.data?.data?.[1].ip_address).toBe("192.168.1.101");
    expect(result.current.data?.pagination?.total_items).toBe(2);
    expect(result.current.data?.pagination?.total_pages).toBe(1);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal error"));

    const { result } = renderHookWithQuery(() => useHosts());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("forwards status filter as a query param", async () => {
    mockGet.mockResolvedValue(
      ok({
        data: [mockHost],
        pagination: { ...mockPagination, total_items: 1 },
      }),
    );

    const { result } = renderHookWithQuery(() => useHosts({ status: "up" }));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts",
      expect.objectContaining({ params: { query: { status: "up" } } }),
    );
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useHosts({ page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts",
      expect.objectContaining({
        params: { query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("forwards search as a query param", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useHosts({ search: "server01" }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts",
      expect.objectContaining({ params: { query: { search: "server01" } } }),
    );
  });

  it("caches the result under the ['hosts', params] query key", async () => {
    const params = { page: 1, page_size: 20 };
    mockGet.mockResolvedValue(
      ok({ data: [mockHost], pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() => useHosts(params));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["hosts", params]);
    expect(cached).toBeDefined();
  });
});

// ── useHost ───────────────────────────────────────────────────────────────────

describe("useHost", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when an id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useHost("host-1"));
    expect(result.current.isLoading).toBe(true);
  });

  it("returns the host data on success", async () => {
    mockGet.mockResolvedValue(ok(mockHost));

    const { result } = renderHookWithQuery(() => useHost("host-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe("host-1");
    expect(result.current.data?.ip_address).toBe("192.168.1.100");
    expect(result.current.data?.hostname).toBe("server01.local");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() => useHost("host-unknown"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled and does not fetch when id is empty string", () => {
    const { result } = renderHookWithQuery(() => useHost(""));
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("forwards the host id as a path param", async () => {
    mockGet.mockResolvedValue(ok(mockHost));

    const { result } = renderHookWithQuery(() => useHost("host-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts/{hostId}",
      expect.objectContaining({ params: { path: { hostId: "host-1" } } }),
    );
  });

  it("caches the result under the ['hosts', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockHost));

    const { result, queryClient } = renderHookWithQuery(() =>
      useHost("host-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["hosts", "host-1"]);
    expect(cached).toBeDefined();
    expect((cached as typeof mockHost).ip_address).toBe("192.168.1.100");
  });
});

// ── useActiveHostCount ────────────────────────────────────────────────────────

describe("useActiveHostCount", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns the total_items value from the pagination field", async () => {
    mockGet.mockResolvedValue(
      ok({
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 42, total_pages: 42 },
      }),
    );

    const { result } = renderHookWithQuery(() => useActiveHostCount());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBe(42);
  });

  it("calls api.GET with status=up, page=1, page_size=1", async () => {
    mockGet.mockResolvedValue(
      ok({
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 5, total_pages: 5 },
      }),
    );

    const { result } = renderHookWithQuery(() => useActiveHostCount());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts",
      expect.objectContaining({
        params: { query: { status: "up", page: 1, page_size: 1 } },
      }),
    );
  });

  it("returns 0 when pagination is absent", async () => {
    mockGet.mockResolvedValue(ok({}));

    const { result } = renderHookWithQuery(() => useActiveHostCount());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBe(0);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("forbidden"));

    const { result } = renderHookWithQuery(() => useActiveHostCount());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("caches result under the ['hosts', 'active-count'] query key", async () => {
    mockGet.mockResolvedValue(
      ok({
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 7, total_pages: 7 },
      }),
    );

    const { result, queryClient } = renderHookWithQuery(() =>
      useActiveHostCount(),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["hosts", "active-count"]);
    expect(cached).toBe(7);
  });
});

// ── useHostScans ──────────────────────────────────────────────────────────────

describe("useHostScans", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls GET /hosts/{hostId}/scans with the correct hostId", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useHostScans("host-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts/{hostId}/scans",
      expect.objectContaining({
        params: { path: { hostId: "host-1" }, query: {} },
      }),
    );
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() =>
      useHostScans("host-1", { page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/hosts/{hostId}/scans",
      expect.objectContaining({
        params: { path: { hostId: "host-1" }, query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("is disabled and does not fetch when hostId is empty", () => {
    const { result } = renderHookWithQuery(() => useHostScans(""));
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });
});

// ── useUpdateHost ─────────────────────────────────────────────────────────────

describe("useUpdateHost", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useUpdateHost());
    expect(result.current.isPending).toBe(false);
  });

  it("calls PUT /hosts/{hostId} with the correct params and body", async () => {
    mockPut.mockResolvedValue(okPut(mockHost));

    const { result, actHook } = renderHookWithQuery(() => useUpdateHost());

    await actHook(async () => {
      await result.current.mutateAsync({
        hostId: "host-1",
        body: { hostname: "newname.local" },
      });
    });

    expect(mockPut).toHaveBeenCalledWith(
      "/hosts/{hostId}",
      expect.objectContaining({
        params: { path: { hostId: "host-1" } },
        body: { hostname: "newname.local" },
      }),
    );
  });

  it("invalidates ['hosts'] queries on success", async () => {
    mockPut.mockResolvedValue(okPut(mockHost));

    const { result, actHook, queryClient } = renderHookWithQuery(() =>
      useUpdateHost(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync({
        hostId: "host-1",
        body: { hostname: "updated.local" },
      });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["hosts"] }),
    );
  });
});

// ── useDeleteHost ─────────────────────────────────────────────────────────────

describe("useDeleteHost", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useDeleteHost());
    expect(result.current.isPending).toBe(false);
  });

  it("calls DELETE /hosts/{hostId} with the correct hostId", async () => {
    mockDelete.mockResolvedValue(okDelete());

    const { result, actHook } = renderHookWithQuery(() => useDeleteHost());

    await actHook(async () => {
      await result.current.mutateAsync("host-1");
    });

    expect(mockDelete).toHaveBeenCalledWith(
      "/hosts/{hostId}",
      expect.objectContaining({ params: { path: { hostId: "host-1" } } }),
    );
  });

  it("invalidates ['hosts'] queries on success", async () => {
    mockDelete.mockResolvedValue(okDelete());

    const { result, actHook, queryClient } = renderHookWithQuery(() =>
      useDeleteHost(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync("host-1");
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["hosts"] }),
    );
  });
});
