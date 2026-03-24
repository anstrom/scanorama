import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useSchedules,
  useSchedule,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  useEnableSchedule,
  useDisableSchedule,
} from "./use-schedules";

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

const mockSchedules = [
  {
    id: "sched-1",
    name: "Daily Security Scan",
    cron_expr: "0 2 * * *",
    enabled: true,
    targets: ["192.168.1.0/24"],
    next_run: "2024-06-02T02:00:00Z",
    last_run: "2024-06-01T02:00:00Z",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-06-01T00:00:00Z",
    profile_id: "profile-1",
  },
  {
    id: "sched-2",
    name: "Weekly Recon",
    cron_expr: "0 0 * * 1",
    enabled: false,
    targets: ["10.0.0.0/8"],
    next_run: undefined,
    last_run: undefined,
    created_at: "2024-02-01T00:00:00Z",
    updated_at: "2024-06-02T00:00:00Z",
  },
];

const mockPagination = {
  page: 1,
  page_size: 25,
  total_items: 2,
  total_pages: 1,
};

const mockSchedule = mockSchedules[0];

// ── useSchedules ──────────────────────────────────────────────────────────────

describe("useSchedules", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useSchedules());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns paginated schedule list on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockSchedules, pagination: mockPagination }),
    );
    const { result } = renderHookWithQuery(() => useSchedules());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].id).toBe("sched-1");
    expect(result.current.data?.pagination?.total_items).toBe(2);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal server error"));
    const { result } = renderHookWithQuery(() => useSchedules());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("forwards page, page_size, and enabled as query params", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: { ...mockPagination, total_items: 0 } }),
    );
    const params = { page: 2, page_size: 10, enabled: true };
    const { result } = renderHookWithQuery(() => useSchedules(params));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/schedules",
      expect.objectContaining({
        params: { query: params },
      }),
    );
  });

  it("caches the result under the ['schedules', params] query key", async () => {
    const params = { page: 1, page_size: 25 };
    mockGet.mockResolvedValue(
      ok({ data: mockSchedules, pagination: mockPagination }),
    );
    const { result, queryClient } = renderHookWithQuery(() =>
      useSchedules(params),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["schedules", params]);
    expect(cached).toBeDefined();
  });

  it("returns empty list when API returns no schedules", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: { ...mockPagination, total_items: 0 } }),
    );
    const { result } = renderHookWithQuery(() => useSchedules());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.data).toHaveLength(0);
  });
});

// ── useSchedule ───────────────────────────────────────────────────────────────

describe("useSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useSchedule("sched-1"));
    expect(result.current.isLoading).toBe(true);
  });

  it("is disabled (idle) when id is empty", () => {
    const { result } = renderHookWithQuery(() => useSchedule(""));
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("returns schedule data on success", async () => {
    mockGet.mockResolvedValue(ok(mockSchedule));
    const { result } = renderHookWithQuery(() => useSchedule("sched-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe("sched-1");
    expect(result.current.data?.name).toBe("Daily Security Scan");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));
    const { result } = renderHookWithQuery(() => useSchedule("nonexistent"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the correct path param", async () => {
    mockGet.mockResolvedValue(ok(mockSchedule));
    const { result } = renderHookWithQuery(() => useSchedule("sched-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockGet).toHaveBeenCalledWith(
      "/schedules/{scheduleId}",
      expect.objectContaining({
        params: { path: { scheduleId: "sched-1" } },
      }),
    );
  });

  it("caches under the ['schedules', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockSchedule));
    const { result, queryClient } = renderHookWithQuery(() =>
      useSchedule("sched-1"),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const cached = queryClient.getQueryData(["schedules", "sched-1"]);
    expect(cached).toBeDefined();
  });
});

// ── useCreateSchedule ─────────────────────────────────────────────────────────

describe("useCreateSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useCreateSchedule());
    expect(result.current.isPending).toBe(false);
    expect(result.current.isIdle).toBe(true);
  });

  it("calls POST /schedules with the request body", async () => {
    mockPost.mockResolvedValue(okPost(mockSchedule));
    const { result, actHook } = renderHookWithQuery(() => useCreateSchedule());
    await actHook(async () => {
      await result.current.mutateAsync({
        name: "Daily Security Scan",
        cron_expr: "0 2 * * *",
        enabled: true,
      });
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/schedules",
      expect.objectContaining({
        body: expect.objectContaining({ name: "Daily Security Scan" }),
      }),
    );
  });

  it("throws a descriptive error when api.POST returns an error", async () => {
    mockPost.mockResolvedValue(failPost("cron expression invalid"));
    const { result, actHook } = renderHookWithQuery(() => useCreateSchedule());
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ name: "Bad Schedule" }),
      ).rejects.toThrow("cron expression invalid");
    });
  });

  it("invalidates ['schedules'] queries on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: mockSchedules, pagination: mockPagination }),
    );
    mockPost.mockResolvedValue(okPost(mockSchedule));
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useCreateSchedule(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync({
        name: "New Schedule",
        cron_expr: "0 2 * * *",
      });
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["schedules"] }),
    );
  });
});

// ── useUpdateSchedule ─────────────────────────────────────────────────────────

describe("useUpdateSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.PUT with the correct path and body", async () => {
    const updated = { ...mockSchedule, name: "Renamed Schedule" };
    mockPut.mockResolvedValue(okPut(updated));
    const { result, actHook } = renderHookWithQuery(() => useUpdateSchedule());
    await actHook(async () => {
      await result.current.mutateAsync({
        id: "sched-1",
        body: { name: "Renamed Schedule" },
      });
    });
    expect(mockPut).toHaveBeenCalledWith(
      "/schedules/{scheduleId}",
      expect.objectContaining({
        params: { path: { scheduleId: "sched-1" } },
        body: expect.objectContaining({ name: "Renamed Schedule" }),
      }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockPut.mockResolvedValue(failPut("not found"));
    const { result, actHook } = renderHookWithQuery(() => useUpdateSchedule());
    await actHook(async () => {
      await expect(
        result.current.mutateAsync({ id: "nonexistent", body: {} }),
      ).rejects.toThrow("not found");
    });
  });
});

// ── useDeleteSchedule ─────────────────────────────────────────────────────────

describe("useDeleteSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in idle state", () => {
    const { result } = renderHookWithQuery(() => useDeleteSchedule());
    expect(result.current.isPending).toBe(false);
    expect(result.current.isIdle).toBe(true);
  });

  it("calls api.DELETE with the correct path param", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, actHook } = renderHookWithQuery(() => useDeleteSchedule());
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(mockDelete).toHaveBeenCalledWith(
      "/schedules/{scheduleId}",
      expect.objectContaining({
        params: { path: { scheduleId: "sched-1" } },
      }),
    );
  });

  it("throws a descriptive error when api.DELETE returns an error", async () => {
    mockDelete.mockResolvedValue(failDelete("not found"));
    const { result, actHook } = renderHookWithQuery(() => useDeleteSchedule());
    await actHook(async () => {
      await expect(result.current.mutateAsync("sched-1")).rejects.toThrow(
        "not found",
      );
    });
  });

  it("invalidates ['schedules'] queries on success", async () => {
    mockDelete.mockResolvedValue(okDelete());
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useDeleteSchedule(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["schedules"] }),
    );
  });
});

// ── useEnableSchedule ─────────────────────────────────────────────────────────

describe("useEnableSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.POST with /schedules/{scheduleId}/enable", async () => {
    const enabled = { ...mockSchedule, enabled: true };
    mockPost.mockResolvedValue(okPost(enabled));
    const { result, actHook } = renderHookWithQuery(() => useEnableSchedule());
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/schedules/{scheduleId}/enable",
      expect.objectContaining({
        params: { path: { scheduleId: "sched-1" } },
      }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockPost.mockResolvedValue(failPost("schedule not found"));
    const { result, actHook } = renderHookWithQuery(() => useEnableSchedule());
    await actHook(async () => {
      await expect(result.current.mutateAsync("sched-1")).rejects.toThrow(
        "schedule not found",
      );
    });
  });

  it("invalidates ['schedules'] on success", async () => {
    mockPost.mockResolvedValue(okPost({ ...mockSchedule, enabled: true }));
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useEnableSchedule(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["schedules"] }),
    );
  });
});

// ── useDisableSchedule ────────────────────────────────────────────────────────

describe("useDisableSchedule", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.POST with /schedules/{scheduleId}/disable", async () => {
    const disabled = { ...mockSchedule, enabled: false };
    mockPost.mockResolvedValue(okPost(disabled));
    const { result, actHook } = renderHookWithQuery(() => useDisableSchedule());
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(mockPost).toHaveBeenCalledWith(
      "/schedules/{scheduleId}/disable",
      expect.objectContaining({
        params: { path: { scheduleId: "sched-1" } },
      }),
    );
  });

  it("throws a descriptive error on failure", async () => {
    mockPost.mockResolvedValue(failPost("schedule not found"));
    const { result, actHook } = renderHookWithQuery(() => useDisableSchedule());
    await actHook(async () => {
      await expect(result.current.mutateAsync("sched-1")).rejects.toThrow(
        "schedule not found",
      );
    });
  });

  it("invalidates ['schedules'] on success", async () => {
    mockPost.mockResolvedValue(okPost({ ...mockSchedule, enabled: false }));
    const { result, queryClient, actHook } = renderHookWithQuery(() =>
      useDisableSchedule(),
    );
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    await actHook(async () => {
      await result.current.mutateAsync("sched-1");
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["schedules"] }),
    );
  });
});
