import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useScans,
  useScan,
  useRecentScans,
  useCreateScan,
  useStartScan,
  useScanResults,
} from "./use-scans";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);
const mockPost = vi.mocked(api.POST);

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

const mockPagination = {
  page: 1,
  page_size: 20,
  total_items: 2,
  total_pages: 1,
};

const mockScans = [
  {
    id: "scan-1",
    status: "completed",
    targets: ["192.168.1.0/24"],
    hosts_discovered: 10,
    ports_scanned: 100,
    created_at: "2024-06-01T12:00:00Z",
    scan_type: "connect",
  },
  {
    id: "scan-2",
    status: "running",
    targets: ["10.0.0.0/8"],
    hosts_discovered: 2,
    ports_scanned: 20,
    created_at: "2024-06-02T08:00:00Z",
    scan_type: "syn",
  },
];

const mockScanResults = {
  scan_id: "scan-1",
  total_hosts: 2,
  total_ports: 4,
  open_ports: 4,
  closed_ports: 0,
  generated_at: "2024-06-01T12:05:00Z",
  results: [
    {
      id: "r-1",
      host_ip: "192.168.1.1",
      port: 80,
      state: "open",
      service: "http",
    },
  ],
};

// ── useScans ──────────────────────────────────────────────────────────────────

describe("useScans", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useScans());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns scan list and pagination on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockScans, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useScans());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].id).toBe("scan-1");
    expect(result.current.data?.data?.[1].id).toBe("scan-2");
    expect(result.current.data?.pagination?.total_items).toBe(2);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("unauthorized"));

    const { result } = renderHookWithQuery(() => useScans());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("forwards status filter as a query param", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useScans({ status: "completed" }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans",
      expect.objectContaining({
        params: { query: { status: "completed" } },
      }),
    );
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useScans({ page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans",
      expect.objectContaining({
        params: { query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("caches the result under the ['scans', 'list', params] query key", async () => {
    const params = { page: 1, page_size: 20 };
    mockGet.mockResolvedValue(
      ok({ data: mockScans, pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() => useScans(params));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["scans", "list", params]);
    expect(cached).toBeDefined();
  });
});

// ── useScan ───────────────────────────────────────────────────────────────────

describe("useScan", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when an id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useScan("scan-1"));
    expect(result.current.isLoading).toBe(true);
  });

  it("returns the scan data on success", async () => {
    mockGet.mockResolvedValue(ok(mockScans[0]));

    const { result } = renderHookWithQuery(() => useScan("scan-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe("scan-1");
    expect(result.current.data?.status).toBe("completed");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() => useScan("scan-unknown"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled and does not fetch when id is empty string", () => {
    const { result } = renderHookWithQuery(() => useScan(""));
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("forwards the scan id as a path param", async () => {
    mockGet.mockResolvedValue(ok(mockScans[0]));

    const { result } = renderHookWithQuery(() => useScan("scan-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans/{scanId}",
      expect.objectContaining({ params: { path: { scanId: "scan-1" } } }),
    );
  });

  it("caches the result under the ['scans', 'detail', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockScans[0]));

    const { result, queryClient } = renderHookWithQuery(() =>
      useScan("scan-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["scans", "detail", "scan-1"]);
    expect(cached).toBeDefined();
    expect((cached as (typeof mockScans)[0]).id).toBe("scan-1");
  });
});

// ── useRecentScans ────────────────────────────────────────────────────────────

describe("useRecentScans", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useRecentScans());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns scan data on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockScans, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useRecentScans());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].id).toBe("scan-1");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("server error"));

    const { result } = renderHookWithQuery(() => useRecentScans());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("defaults to page_size=5", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() => useRecentScans());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans",
      expect.objectContaining({
        params: { query: { page: 1, page_size: 5 } },
      }),
    );
  });

  it("forwards a custom limit as page_size", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() => useRecentScans(3));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans",
      expect.objectContaining({
        params: { query: { page: 1, page_size: 3 } },
      }),
    );
  });

  it("caches the result under the ['scans', 'recent', limit] query key", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockScans, pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() =>
      useRecentScans(5),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["scans", "recent", 5]);
    expect(cached).toBeDefined();
  });
});

// ── useScanResults ────────────────────────────────────────────────────────────

describe("useScanResults", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when a scanId is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() =>
      useScanResults("scan-1", "completed"),
    );
    expect(result.current.isLoading).toBe(true);
  });

  it("returns results data on success", async () => {
    mockGet.mockResolvedValue(ok(mockScanResults));

    const { result } = renderHookWithQuery(() =>
      useScanResults("scan-1", "completed"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.scan_id).toBe("scan-1");
    expect(result.current.data?.results).toHaveLength(1);
    expect(result.current.data?.total_hosts).toBe(2);
    expect(result.current.data?.open_ports).toBe(4);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() =>
      useScanResults("scan-unknown", "completed"),
    );
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled and does not fetch when scanId is empty string", () => {
    const { result } = renderHookWithQuery(() =>
      useScanResults("", "completed"),
    );
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("forwards the scan id as a path param", async () => {
    mockGet.mockResolvedValue(ok(mockScanResults));

    const { result } = renderHookWithQuery(() =>
      useScanResults("scan-1", "completed"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/scans/{scanId}/results",
      expect.objectContaining({ params: { path: { scanId: "scan-1" } } }),
    );
  });

  it("caches the result under the ['scans', 'results', scanId] query key", async () => {
    mockGet.mockResolvedValue(ok(mockScanResults));

    const { result, queryClient } = renderHookWithQuery(() =>
      useScanResults("scan-1", "completed"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["scans", "results", "scan-1"]);
    expect(cached).toBeDefined();
  });
});

// ── useCreateScan ─────────────────────────────────────────────────────────────

describe("useCreateScan", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("creates a scan and returns the new scan object", async () => {
    const newScan = { id: "new-scan-1", status: "pending", name: "Test scan" };
    mockPost.mockResolvedValue(ok(newScan));

    const { result, actHook } = renderHookWithQuery(() => useCreateScan());

    let data: unknown;
    await actHook(async () => {
      data = await result.current.mutateAsync({
        name: "Test scan",
        targets: ["192.168.1.1"],
        scan_type: "connect",
      });
    });

    expect(data).toMatchObject({ id: "new-scan-1", status: "pending" });
  });

  it("calls api.POST with the /scans path and request body", async () => {
    mockPost.mockResolvedValue(ok({ id: "new-scan-2", status: "pending" }));

    const { result, actHook } = renderHookWithQuery(() => useCreateScan());
    const body = {
      name: "Custom scan",
      targets: ["10.0.0.1", "10.0.0.2"],
      scan_type: "syn",
      ports: "22,80,443",
      os_detection: true,
    };

    await actHook(async () => {
      await result.current.mutateAsync(body);
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/scans",
      expect.objectContaining({ body }),
    );
  });

  it("throws when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(fail("validation failed"));

    const { result, actHook } = renderHookWithQuery(() => useCreateScan());

    await actHook(async () => {
      await expect(
        result.current.mutateAsync({
          name: "Bad scan",
          targets: [],
          scan_type: "connect",
        }),
      ).rejects.toThrow();
    });
  });

  it("invalidates the ['scans'] query key on success", async () => {
    mockPost.mockResolvedValue(ok({ id: "new-scan-3", status: "pending" }));
    // Seed a stale scans entry so we can verify it gets invalidated
    mockGet.mockResolvedValue(
      ok({ data: mockScans, pagination: mockPagination }),
    );

    const { result, actHook, queryClient } = renderHookWithQuery(() =>
      useCreateScan(),
    );

    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync({
        name: "Invalidating scan",
        targets: ["1.2.3.4"],
        scan_type: "connect",
      });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["scans"] }),
    );
  });
});

// ── useStartScan ──────────────────────────────────────────────────────────────

describe("useStartScan", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts a scan and returns the updated scan object", async () => {
    const started = { id: "scan-1", status: "running" };
    mockPost.mockResolvedValue(ok(started));

    const { result, actHook } = renderHookWithQuery(() => useStartScan());

    let data: unknown;
    await actHook(async () => {
      data = await result.current.mutateAsync("scan-1");
    });

    expect(data).toMatchObject({ id: "scan-1", status: "running" });
  });

  it("calls api.POST with the /scans/{scanId}/start path", async () => {
    mockPost.mockResolvedValue(ok({ id: "scan-1", status: "running" }));

    const { result, actHook } = renderHookWithQuery(() => useStartScan());

    await actHook(async () => {
      await result.current.mutateAsync("scan-1");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/scans/{scanId}/start",
      expect.objectContaining({ params: { path: { scanId: "scan-1" } } }),
    );
  });

  it("throws when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(fail("not found"));

    const { result, actHook } = renderHookWithQuery(() => useStartScan());

    await actHook(async () => {
      await expect(result.current.mutateAsync("bad-id")).rejects.toThrow();
    });
  });

  it("invalidates the ['scans'] query key on success", async () => {
    mockPost.mockResolvedValue(ok({ id: "scan-1", status: "running" }));

    const { result, actHook, queryClient } = renderHookWithQuery(() =>
      useStartScan(),
    );

    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync("scan-1");
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["scans"] }),
    );
  });

  it("forwards the scan id as a path param for different scan ids", async () => {
    mockPost.mockResolvedValue(ok({ id: "scan-2", status: "running" }));

    const { result, actHook } = renderHookWithQuery(() => useStartScan());

    await actHook(async () => {
      await result.current.mutateAsync("scan-2");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/scans/{scanId}/start",
      expect.objectContaining({ params: { path: { scanId: "scan-2" } } }),
    );
  });
});
