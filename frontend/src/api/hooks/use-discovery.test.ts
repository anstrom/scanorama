import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useDiscoveryJobs,
  useDiscoveryJob,
  useCreateDiscoveryJob,
  useStartDiscovery,
  useStopDiscovery,
} from "./use-discovery";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);
const mockPost = vi.mocked(api.POST);

// ── Helpers ───────────────────────────────────────────────────────────────────

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

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockPagination = {
  page: 1,
  page_size: 20,
  total_items: 2,
  total_pages: 1,
};

const mockJobs = [
  {
    id: "job-1",
    name: "Office LAN Discovery",
    networks: ["192.168.1.0/24"],
    method: "ping" as const,
    status: "completed" as const,
    progress: 100,
    started_at: "2024-01-01T10:00:00Z",
    created_at: "2024-01-01T09:00:00Z",
  },
  {
    id: "job-2",
    name: "DMZ Discovery",
    networks: ["10.0.0.0/8"],
    method: "icmp" as const,
    status: "running" as const,
    progress: 45,
    started_at: "2024-01-02T10:00:00Z",
    created_at: "2024-01-02T09:00:00Z",
  },
];

const mockJob = mockJobs[0];

// ── useDiscoveryJobs ──────────────────────────────────────────────────────────

describe("useDiscoveryJobs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useDiscoveryJobs());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();
  });

  it("returns job list and pagination on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockJobs, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useDiscoveryJobs());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].name).toBe("Office LAN Discovery");
    expect(result.current.data?.data?.[1].name).toBe("DMZ Discovery");
    expect(result.current.data?.pagination?.total_items).toBe(2);
    expect(result.current.data?.pagination?.total_pages).toBe(1);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal error"));

    const { result } = renderHookWithQuery(() => useDiscoveryJobs());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the /discovery path", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockJobs, pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useDiscoveryJobs());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/discovery",
      expect.objectContaining({ params: { query: {} } }),
    );
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(ok({ data: [], pagination: mockPagination }));

    const { result } = renderHookWithQuery(() =>
      useDiscoveryJobs({ page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/discovery",
      expect.objectContaining({
        params: { query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("returns an empty list when the API returns no jobs", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: { ...mockPagination, total_items: 0 } }),
    );

    const { result } = renderHookWithQuery(() => useDiscoveryJobs());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(0);
  });

  it("caches the result under the ['discovery', params] query key", async () => {
    const params = { page: 1, page_size: 20 };
    mockGet.mockResolvedValue(
      ok({ data: mockJobs, pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() =>
      useDiscoveryJobs(params),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["discovery", params]);
    expect(cached).toBeDefined();
  });
});

// ── useDiscoveryJob ───────────────────────────────────────────────────────────

describe("useDiscoveryJob", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useDiscoveryJob("job-1"));
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled (not loading) when id is empty", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useDiscoveryJob(""));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("returns job data on success", async () => {
    mockGet.mockResolvedValue(ok(mockJob));

    const { result } = renderHookWithQuery(() => useDiscoveryJob("job-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe("job-1");
    expect(result.current.data?.name).toBe("Office LAN Discovery");
    expect(result.current.data?.networks).toEqual(["192.168.1.0/24"]);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() => useDiscoveryJob("job-999"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the correct path param", async () => {
    mockGet.mockResolvedValue(ok(mockJob));

    const { result } = renderHookWithQuery(() => useDiscoveryJob("job-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/discovery/{discoveryId}",
      expect.objectContaining({
        params: { path: { discoveryId: "job-1" } },
      }),
    );
  });

  it("caches the result under the ['discovery', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockJob));

    const { result, queryClient } = renderHookWithQuery(() =>
      useDiscoveryJob("job-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["discovery", "job-1"]);
    expect(cached).toBeDefined();
  });
});

// ── useCreateDiscoveryJob ─────────────────────────────────────────────────────

describe("useCreateDiscoveryJob", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useCreateDiscoveryJob());
    expect(result.current.isPending).toBe(false);
    expect(result.current.isSuccess).toBe(false);
  });

  it("returns the created job on success", async () => {
    mockPost.mockResolvedValue(okPost(mockJob));

    const { result, actHook } = renderHookWithQuery(() =>
      useCreateDiscoveryJob(),
    );
    let created: typeof mockJob | undefined;
    await actHook(async () => {
      created = (await result.current.mutateAsync({
        name: "Office LAN Discovery",
        networks: ["192.168.1.0/24"],
        method: "ping",
      })) as typeof mockJob;
    });

    expect(created?.id).toBe("job-1");
    expect(created?.name).toBe("Office LAN Discovery");
  });

  it("calls api.POST with /discovery and the request body", async () => {
    mockPost.mockResolvedValue(okPost(mockJob));

    const body = {
      name: "DMZ Scan",
      networks: ["10.0.0.0/8"],
      method: "icmp" as const,
    };
    const { result, actHook } = renderHookWithQuery(() =>
      useCreateDiscoveryJob(),
    );
    await actHook(async () => {
      await result.current.mutateAsync(body);
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/discovery",
      expect.objectContaining({ body }),
    );
  });

  it("throws a descriptive error when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(failPost("network already exists"));

    const { result, actHook } = renderHookWithQuery(() =>
      useCreateDiscoveryJob(),
    );
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ networks: ["192.168.1.0/24"] }),
      ).rejects.toThrow("network already exists");
    });
  });

  it("throws a fallback error when the error has no message", async () => {
    mockPost.mockResolvedValue(
      Promise.resolve({
        data: undefined,
        error: {},
        response: new Response(),
      }) as ReturnType<typeof mockPost>,
    );

    const { result, actHook } = renderHookWithQuery(() =>
      useCreateDiscoveryJob(),
    );
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ networks: ["192.168.1.0/24"] }),
      ).rejects.toThrow("Failed to create discovery job.");
    });
  });

  it("invalidates ['discovery'] queries on success", async () => {
    mockPost.mockResolvedValue(okPost(mockJob));
    mockGet.mockResolvedValue(
      ok({ data: [mockJob], pagination: mockPagination }),
    );

    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useCreateDiscoveryJob(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync({
        name: "Office LAN Discovery",
        networks: ["192.168.1.0/24"],
        method: "ping",
      });
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["discovery"] }),
    );
  });
});

// ── useStartDiscovery ─────────────────────────────────────────────────────────

describe("useStartDiscovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useStartDiscovery());
    expect(result.current.isPending).toBe(false);
  });

  it("returns the updated job on success", async () => {
    const started = { ...mockJob, status: "running" as const };
    mockPost.mockResolvedValue(okPost(started));

    const { result, actHook } = renderHookWithQuery(() => useStartDiscovery());
    let data: typeof started | undefined;
    await actHook(async () => {
      data = (await result.current.mutateAsync("job-1")) as typeof started;
    });

    expect(data?.id).toBe("job-1");
    expect(data?.status).toBe("running");
  });

  it("calls api.POST with /discovery/{discoveryId}/start", async () => {
    const started = { ...mockJob, status: "running" as const };
    mockPost.mockResolvedValue(okPost(started));

    const { result, actHook } = renderHookWithQuery(() => useStartDiscovery());
    await actHook(async () => {
      await result.current.mutateAsync("job-1");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/discovery/{discoveryId}/start",
      expect.objectContaining({
        params: { path: { discoveryId: "job-1" } },
      }),
    );
  });

  it("forwards a different discovery id as the path param", async () => {
    const started = { ...mockJobs[1], status: "running" as const };
    mockPost.mockResolvedValue(okPost(started));

    const { result, actHook } = renderHookWithQuery(() => useStartDiscovery());
    await actHook(async () => {
      await result.current.mutateAsync("job-2");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/discovery/{discoveryId}/start",
      expect.objectContaining({
        params: { path: { discoveryId: "job-2" } },
      }),
    );
  });

  it("throws a descriptive error when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(failPost("job already running"));

    const { result, actHook } = renderHookWithQuery(() => useStartDiscovery());
    await actHook(async () => {
      await expect(result.current.mutateAsync("job-1")).rejects.toThrow(
        "job already running",
      );
    });
  });

  it("invalidates ['discovery'] queries on success", async () => {
    const started = { ...mockJob, status: "running" as const };
    mockPost.mockResolvedValue(okPost(started));
    mockGet.mockResolvedValue(
      ok({ data: [started], pagination: mockPagination }),
    );

    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useStartDiscovery(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync("job-1");
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["discovery"] }),
    );
  });
});

// ── useStopDiscovery ──────────────────────────────────────────────────────────

describe("useStopDiscovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useStopDiscovery());
    expect(result.current.isPending).toBe(false);
  });

  it("returns the updated job on success", async () => {
    const stopped = { ...mockJobs[1], status: "failed" as const };
    mockPost.mockResolvedValue(okPost(stopped));

    const { result, actHook } = renderHookWithQuery(() => useStopDiscovery());
    let data: typeof stopped | undefined;
    await actHook(async () => {
      data = (await result.current.mutateAsync("job-2")) as typeof stopped;
    });

    expect(data?.id).toBe("job-2");
  });

  it("calls api.POST with /discovery/{discoveryId}/stop", async () => {
    const stopped = { ...mockJobs[1], status: "failed" as const };
    mockPost.mockResolvedValue(okPost(stopped));

    const { result, actHook } = renderHookWithQuery(() => useStopDiscovery());
    await actHook(async () => {
      await result.current.mutateAsync("job-2");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/discovery/{discoveryId}/stop",
      expect.objectContaining({
        params: { path: { discoveryId: "job-2" } },
      }),
    );
  });

  it("forwards a different discovery id as the path param", async () => {
    const stopped = { ...mockJob, status: "failed" as const };
    mockPost.mockResolvedValue(okPost(stopped));

    const { result, actHook } = renderHookWithQuery(() => useStopDiscovery());
    await actHook(async () => {
      await result.current.mutateAsync("job-1");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/discovery/{discoveryId}/stop",
      expect.objectContaining({
        params: { path: { discoveryId: "job-1" } },
      }),
    );
  });

  it("throws a descriptive error when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(failPost("job not running"));

    const { result, actHook } = renderHookWithQuery(() => useStopDiscovery());
    await actHook(async () => {
      await expect(result.current.mutateAsync("job-2")).rejects.toThrow(
        "job not running",
      );
    });
  });

  it("invalidates ['discovery'] queries on success", async () => {
    const stopped = { ...mockJobs[1], status: "failed" as const };
    mockPost.mockResolvedValue(okPost(stopped));
    mockGet.mockResolvedValue(
      ok({ data: [stopped], pagination: mockPagination }),
    );

    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useStopDiscovery(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await actHook(async () => {
      await result.current.mutateAsync("job-2");
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["discovery"] }),
    );
  });
});
