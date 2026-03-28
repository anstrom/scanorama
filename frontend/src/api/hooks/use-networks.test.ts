import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useNetworks,
  useNetworkStats,
  useNetwork,
  useNetworkExclusions,
  useGlobalExclusions,
  useCreateNetwork,
  useDeleteNetwork,
  useEnableNetwork,
  useDisableNetwork,
  useRenameNetwork,
  useDeleteExclusion,
  useNetworkDiscoveryJobs,
  useStartNetworkScan,
} from "./use-networks";

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
const mockPost = vi.mocked(api.POST);
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

const mockNetwork = {
  id: "net-1",
  name: "Office LAN",
  cidr: "192.168.1.0/24",
  is_active: true,
  host_count: 25,
  active_host_count: 20,
  discovery_method: "ping",
  scan_enabled: true,
  created_by: "admin",
  created_at: "2024-01-01T00:00:00Z",
};

const mockExclusions = [
  {
    id: "excl-1",
    network_id: "net-1",
    excluded_cidr: "192.168.1.128/25",
    reason: "Printers",
    created_by: "admin",
    created_at: "2024-01-02T00:00:00Z",
    enabled: true,
  },
  {
    id: "excl-2",
    network_id: "net-1",
    excluded_cidr: "192.168.1.200/30",
    reason: "Management",
    created_by: "admin",
    created_at: "2024-01-03T00:00:00Z",
    enabled: true,
  },
];

const okPost = (data: unknown): ReturnType<typeof mockPost> =>
  Promise.resolve({
    data,
    error: undefined,
    response: new Response(),
  }) as ReturnType<typeof mockPost>;

const failPost = (
  message = "something went wrong",
): ReturnType<typeof mockPost> =>
  Promise.resolve({
    data: undefined,
    error: { message },
    response: new Response(),
  }) as ReturnType<typeof mockPost>;

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

// ── useNetwork ────────────────────────────────────────────────────────────────

describe("useNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetwork("net-1"));
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled (not loading) when id is empty", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetwork(""));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("returns network data on success", async () => {
    mockGet.mockResolvedValue(ok(mockNetwork));
    const { result } = renderHookWithQuery(() => useNetwork("net-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe("net-1");
    expect(result.current.data?.name).toBe("Office LAN");
    expect(result.current.data?.cidr).toBe("192.168.1.0/24");
  });

  it("calls api.GET with the correct path param", async () => {
    mockGet.mockResolvedValue(ok(mockNetwork));
    const { result } = renderHookWithQuery(() => useNetwork("net-42"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockGet).toHaveBeenCalledWith(
      "/networks/{networkId}",
      expect.objectContaining({ params: { path: { networkId: "net-42" } } }),
    );
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));
    const { result } = renderHookWithQuery(() => useNetwork("net-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("caches under the ['networks', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockNetwork));
    const { result, queryClient } = renderHookWithQuery(() =>
      useNetwork("net-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const cached = queryClient.getQueryData(["networks", "net-1"]);
    expect(cached).toBeDefined();
  });
});

// ── useNetworkExclusions ──────────────────────────────────────────────────────

describe("useNetworkExclusions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when networkId is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetworkExclusions("net-1"));
    expect(result.current.isLoading).toBe(true);
  });

  it("is disabled when networkId is empty", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetworkExclusions(""));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("returns an array of exclusions on success", async () => {
    mockGet.mockResolvedValue(ok(mockExclusions));
    const { result } = renderHookWithQuery(() => useNetworkExclusions("net-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.[0].excluded_cidr).toBe("192.168.1.128/25");
    expect(result.current.data?.[1].excluded_cidr).toBe("192.168.1.200/30");
  });

  it("returns an empty array when no exclusions exist", async () => {
    mockGet.mockResolvedValue(ok([]));
    const { result } = renderHookWithQuery(() => useNetworkExclusions("net-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("calls api.GET with the correct path param", async () => {
    mockGet.mockResolvedValue(ok(mockExclusions));
    const { result } = renderHookWithQuery(() =>
      useNetworkExclusions("net-99"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockGet).toHaveBeenCalledWith(
      "/networks/{networkId}/exclusions",
      expect.objectContaining({
        params: { path: { networkId: "net-99" } },
      }),
    );
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("forbidden"));
    const { result } = renderHookWithQuery(() => useNetworkExclusions("net-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("caches under the ['networks', networkId, 'exclusions'] query key", async () => {
    mockGet.mockResolvedValue(ok(mockExclusions));
    const { result, queryClient } = renderHookWithQuery(() =>
      useNetworkExclusions("net-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const cached = queryClient.getQueryData([
      "networks",
      "net-1",
      "exclusions",
    ]);
    expect(cached).toBeDefined();
  });
});

// ── useGlobalExclusions ───────────────────────────────────────────────────────

describe("useGlobalExclusions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useGlobalExclusions());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns global exclusions on success", async () => {
    const globalExcl = [
      {
        id: "g-1",
        excluded_cidr: "10.0.0.0/8",
        reason: "Private range",
        created_by: "admin",
        created_at: "2024-01-01T00:00:00Z",
        enabled: true,
      },
    ];
    mockGet.mockResolvedValue(ok(globalExcl));
    const { result } = renderHookWithQuery(() => useGlobalExclusions());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].excluded_cidr).toBe("10.0.0.0/8");
  });

  it("returns an empty array when no global exclusions exist", async () => {
    mockGet.mockResolvedValue(ok([]));
    const { result } = renderHookWithQuery(() => useGlobalExclusions());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("calls api.GET with the /exclusions path", async () => {
    mockGet.mockResolvedValue(ok([]));
    const { result } = renderHookWithQuery(() => useGlobalExclusions());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockGet).toHaveBeenCalledWith("/exclusions");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("server error"));
    const { result } = renderHookWithQuery(() => useGlobalExclusions());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("caches under the ['exclusions', 'global'] query key", async () => {
    mockGet.mockResolvedValue(ok([]));
    const { result, queryClient } = renderHookWithQuery(() =>
      useGlobalExclusions(),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const cached = queryClient.getQueryData(["exclusions", "global"]);
    expect(cached).toBeDefined();
  });
});

// ── useCreateNetwork ──────────────────────────────────────────────────────────

describe("useCreateNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useCreateNetwork());
    expect(result.current.isPending).toBe(false);
    expect(result.current.isSuccess).toBe(false);
  });

  it("returns the created network on success", async () => {
    mockPost.mockResolvedValue(okPost(mockNetwork));
    const { result, actHook } = renderHookWithQuery(() => useCreateNetwork());
    let created: typeof mockNetwork | undefined;
    await actHook(async () => {
      created = (await result.current.mutateAsync({
        name: "Office LAN",
        cidr: "192.168.1.0/24",
        discovery_method: "ping",
        scan_enabled: true,
        is_active: true,
      })) as typeof mockNetwork;
    });
    expect(created?.id).toBe("net-1");
    expect(created?.name).toBe("Office LAN");
  });

  it("calls api.POST with /networks and the request body", async () => {
    mockPost.mockResolvedValue(okPost(mockNetwork));
    const body = {
      name: "DMZ",
      cidr: "10.0.0.0/8",
      discovery_method: "tcp" as const,
      scan_enabled: false,
      is_active: true,
    };
    const { result, actHook } = renderHookWithQuery(() => useCreateNetwork());
    await actHook(async () => {
      await result.current.mutateAsync(body);
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/networks",
      expect.objectContaining({ body }),
    );
  });

  it("throws a descriptive error when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(failPost("CIDR already exists"));
    const { result, actHook } = renderHookWithQuery(() => useCreateNetwork());
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ name: "Dup", cidr: "192.168.1.0/24" }),
      ).rejects.toThrow("CIDR already exists");
    });
  });

  it("invalidates ['networks'] queries on success", async () => {
    mockPost.mockResolvedValue(okPost(mockNetwork));
    mockGet.mockResolvedValue(
      ok({ data: [mockNetwork], pagination: mockPagination }),
    );
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useCreateNetwork(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync({
        name: "Office LAN",
        cidr: "192.168.1.0/24",
      });
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["networks"] }),
    );
  });
});

// ── useDeleteNetwork ──────────────────────────────────────────────────────────

describe("useDeleteNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useDeleteNetwork());
    expect(result.current.isPending).toBe(false);
  });

  it("calls api.DELETE with the correct path param", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, actHook } = renderHookWithQuery(() => useDeleteNetwork());
    await actHook(async () => {
      await result.current.mutateAsync("net-1");
    });
    expect(mockDelete).toHaveBeenCalledWith(
      "/networks/{networkId}",
      expect.objectContaining({ params: { path: { networkId: "net-1" } } }),
    );
  });

  it("throws a descriptive error when api.DELETE returns an error", async () => {
    mockDelete.mockResolvedValue(failDelete("network not found"));
    const { result, actHook } = renderHookWithQuery(() => useDeleteNetwork());
    await actHook(async () => {
      await expect(result.current.mutateAsync("net-99")).rejects.toThrow(
        "network not found",
      );
    });
  });

  it("invalidates ['networks'] queries on success", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useDeleteNetwork(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync("net-1");
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["networks"] }),
    );
  });
});

// ── useEnableNetwork / useDisableNetwork ──────────────────────────────────────

describe("useEnableNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.POST with /networks/{networkId}/enable", async () => {
    mockPost.mockResolvedValue(okPost({ ...mockNetwork, is_active: true }));
    const { result, actHook } = renderHookWithQuery(() => useEnableNetwork());
    await actHook(async () => {
      await result.current.mutateAsync("net-1");
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/networks/{networkId}/enable",
      expect.objectContaining({ params: { path: { networkId: "net-1" } } }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockPost.mockResolvedValue(failPost("already enabled"));
    const { result, actHook } = renderHookWithQuery(() => useEnableNetwork());
    await actHook(async () => {
      await expect(result.current.mutateAsync("net-1")).rejects.toThrow(
        "already enabled",
      );
    });
  });
});

describe("useDisableNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.POST with /networks/{networkId}/disable", async () => {
    mockPost.mockResolvedValue(okPost({ ...mockNetwork, is_active: false }));
    const { result, actHook } = renderHookWithQuery(() => useDisableNetwork());
    await actHook(async () => {
      await result.current.mutateAsync("net-1");
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/networks/{networkId}/disable",
      expect.objectContaining({ params: { path: { networkId: "net-1" } } }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockPost.mockResolvedValue(failPost("cannot disable last network"));
    const { result, actHook } = renderHookWithQuery(() => useDisableNetwork());
    await actHook(async () => {
      await expect(result.current.mutateAsync("net-1")).rejects.toThrow(
        "cannot disable last network",
      );
    });
  });
});

// ── useRenameNetwork ──────────────────────────────────────────────────────────

describe("useRenameNetwork", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.PUT with the correct path and body", async () => {
    mockPut.mockResolvedValue(okPut({ ...mockNetwork, name: "New Name" }));
    const { result, actHook } = renderHookWithQuery(() => useRenameNetwork());
    await actHook(async () => {
      await result.current.mutateAsync({
        networkId: "net-1",
        newName: "New Name",
      });
    });
    expect(mockPut).toHaveBeenCalledWith(
      "/networks/{networkId}/rename",
      expect.objectContaining({
        params: { path: { networkId: "net-1" } },
        body: { new_name: "New Name" },
      }),
    );
  });

  it("returns the updated network on success", async () => {
    const renamed = { ...mockNetwork, name: "Renamed LAN" };
    mockPut.mockResolvedValue(okPut(renamed));
    const { result, actHook } = renderHookWithQuery(() => useRenameNetwork());
    let updated: typeof renamed | undefined;
    await actHook(async () => {
      updated = (await result.current.mutateAsync({
        networkId: "net-1",
        newName: "Renamed LAN",
      })) as typeof renamed;
    });
    expect(updated?.name).toBe("Renamed LAN");
  });

  it("throws a descriptive error on failure", async () => {
    mockPut.mockResolvedValue(failPut("name already taken"));
    const { result, actHook } = renderHookWithQuery(() => useRenameNetwork());
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ networkId: "net-1", newName: "Dup" }),
      ).rejects.toThrow("name already taken");
    });
  });
});

// ── useDeleteExclusion ────────────────────────────────────────────────────────

describe("useDeleteExclusion", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.DELETE with the correct exclusion path", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, actHook } = renderHookWithQuery(() => useDeleteExclusion());
    await actHook(async () => {
      await result.current.mutateAsync("excl-1");
    });
    expect(mockDelete).toHaveBeenCalledWith(
      "/exclusions/{exclusionId}",
      expect.objectContaining({
        params: { path: { exclusionId: "excl-1" } },
      }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockDelete.mockResolvedValue(failDelete("exclusion not found"));
    const { result, actHook } = renderHookWithQuery(() => useDeleteExclusion());
    await actHook(async () => {
      await expect(result.current.mutateAsync("excl-99")).rejects.toThrow(
        "exclusion not found",
      );
    });
  });

  it("invalidates both ['networks'] and ['exclusions'] on success", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useDeleteExclusion(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync("excl-1");
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["networks"] }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["exclusions"] }),
    );
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

// ── useNetworkDiscoveryJobs ───────────────────────────────────────────────────

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const mockDiscoveryJobs = [
  {
    id: "job-1",
    name: "Office LAN Discovery",
    networks: ["192.168.1.0/24"],
    method: "ping",
    status: "completed",
    progress: 100,
    created_at: "2024-01-01T09:00:00Z",
  },
];

function okFetch(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  } as Response);
}

function failFetch(status = 500) {
  return Promise.resolve({
    ok: false,
    status,
    json: () => Promise.resolve({ message: "server error" }),
  } as Response);
}

describe("useNetworkDiscoveryJobs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when networkId is provided", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-1"),
    );
    expect(result.current.isLoading).toBe(true);
  });

  it("is disabled (not loading) when networkId is empty", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useNetworkDiscoveryJobs(""));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("returns discovery jobs on success", async () => {
    const payload = {
      data: mockDiscoveryJobs,
      pagination: { page: 1, page_size: 10, total_items: 1, total_pages: 1 },
    };
    mockFetch.mockResolvedValue(okFetch(payload));
    const { result } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.data).toHaveLength(1);
    expect(result.current.data?.data?.[0].id).toBe("job-1");
  });

  it("calls fetch with the correct URL for a given networkId", async () => {
    mockFetch.mockResolvedValue(okFetch({ data: [], pagination: {} }));
    const { result } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-42"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/networks/net-42/discovery"),
    );
  });

  it("appends page and page_size query params when provided", async () => {
    mockFetch.mockResolvedValue(okFetch({ data: [], pagination: {} }));
    const { result } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-1", { page: 2, page_size: 5 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain("page=2");
    expect(calledUrl).toContain("page_size=5");
  });

  it("enters error state when fetch returns a non-ok response", async () => {
    mockFetch.mockResolvedValue(failFetch(500));
    const { result } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-1"),
    );
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("caches under the ['networks', id, 'discovery', params] query key", async () => {
    const params = { page: 1, page_size: 10 };
    mockFetch.mockResolvedValue(okFetch({ data: mockDiscoveryJobs, pagination: {} }));
    const { result, queryClient } = renderHookWithQuery(() =>
      useNetworkDiscoveryJobs("net-1", params),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const cached = queryClient.getQueryData([
      "networks",
      "net-1",
      "discovery",
      params,
    ]);
    expect(cached).toBeDefined();
  });
});

// ── useStartNetworkScan ───────────────────────────────────────────────────────

const mockScan = {
  id: "scan-1",
  name: "Network Scan",
  targets: ["192.168.1.1", "192.168.1.2"],
  status: "pending",
};

describe("useStartNetworkScan", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useStartNetworkScan());
    expect(result.current.isPending).toBe(false);
    expect(result.current.isSuccess).toBe(false);
  });

  it("returns the created scan on success", async () => {
    mockFetch.mockResolvedValue(okFetch(mockScan));
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    let data: typeof mockScan | undefined;
    await actHook(async () => {
      data = (await result.current.mutateAsync({
        networkId: "net-1",
      })) as typeof mockScan;
    });
    expect(data?.id).toBe("scan-1");
    expect(data?.targets).toHaveLength(2);
  });

  it("calls fetch with POST to /networks/{id}/scan", async () => {
    mockFetch.mockResolvedValue(okFetch(mockScan));
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    await actHook(async () => {
      await result.current.mutateAsync({ networkId: "net-99" });
    });
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/networks/net-99/scan"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("sends os_detection: false by default", async () => {
    mockFetch.mockResolvedValue(okFetch(mockScan));
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    await actHook(async () => {
      await result.current.mutateAsync({ networkId: "net-1" });
    });
    const [, init] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toEqual({ os_detection: false });
  });

  it("sends os_detection: true when requested", async () => {
    mockFetch.mockResolvedValue(okFetch(mockScan));
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    await actHook(async () => {
      await result.current.mutateAsync({
        networkId: "net-1",
        osDetection: true,
      });
    });
    const [, init] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toEqual({ os_detection: true });
  });

  it("throws a descriptive error when fetch returns a non-ok response", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ message: "no active hosts" }),
    });
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ networkId: "net-1" }),
      ).rejects.toThrow("no active hosts");
    });
  });

  it("throws a fallback error when the error body has no message", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({}),
    });
    const { result, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ networkId: "net-1" }),
      ).rejects.toThrow("Failed to create network scan");
    });
  });

  it("invalidates ['scans'] queries on success", async () => {
    mockFetch.mockResolvedValue(okFetch(mockScan));
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useStartNetworkScan(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync({ networkId: "net-1" });
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["scans"] }),
    );
  });
});
