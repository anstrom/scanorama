import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { useHealth, useVersion, useStatus } from "./use-system";

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

// ── useHealth ─────────────────────────────────────────────────────────────────

describe("useHealth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {})); // never resolves
    const { result } = renderHookWithQuery(() => useHealth());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns health data on success", async () => {
    mockGet.mockResolvedValue(
      ok({
        status: "healthy",
        uptime: "2h30m",
        checks: { database: "ok", scanner: "ok" },
      }),
    );

    const { result } = renderHookWithQuery(() => useHealth());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe("healthy");
    expect(result.current.data?.uptime).toBe("2h30m");
  });

  it("returns checks map on success", async () => {
    mockGet.mockResolvedValue(
      ok({
        status: "healthy",
        uptime: "1h",
        checks: { database: "ok", scanner: "degraded" },
      }),
    );

    const { result } = renderHookWithQuery(() => useHealth());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.checks).toEqual({
      database: "ok",
      scanner: "degraded",
    });
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("service unavailable"));

    const { result } = renderHookWithQuery(() => useHealth());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the /health path", async () => {
    mockGet.mockResolvedValue(
      ok({ status: "healthy", uptime: "0s", checks: {} }),
    );

    const { result } = renderHookWithQuery(() => useHealth());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith("/health");
  });

  it("caches the result under the ['health'] query key", async () => {
    mockGet.mockResolvedValue(
      ok({ status: "healthy", uptime: "1m", checks: {} }),
    );

    const { result, queryClient } = renderHookWithQuery(() => useHealth());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["health"]);
    expect(cached).toBeDefined();
    expect((cached as { status: string }).status).toBe("healthy");
  });
});

// ── useVersion ────────────────────────────────────────────────────────────────

describe("useVersion", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useVersion());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns version fields on success", async () => {
    mockGet.mockResolvedValue(
      ok({
        version: "v1.2.3",
        service: "scanorama",
        commit: "abc1234",
        build_time: "2024-06-01T12:00:00Z",
      }),
    );

    const { result } = renderHookWithQuery(() => useVersion());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.version).toBe("v1.2.3");
    expect(result.current.data?.service).toBe("scanorama");
    expect(result.current.data?.commit).toBe("abc1234");
    expect(result.current.data?.build_time).toBe("2024-06-01T12:00:00Z");
  });

  it("returns dev build fields as-is", async () => {
    mockGet.mockResolvedValue(
      ok({
        version: "dev",
        service: "scanorama",
        commit: "none",
        build_time: "unknown",
      }),
    );

    const { result } = renderHookWithQuery(() => useVersion());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.version).toBe("dev");
    expect(result.current.data?.commit).toBe("none");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() => useVersion());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the /version path", async () => {
    mockGet.mockResolvedValue(
      ok({
        version: "v1.0.0",
        service: "scanorama",
        commit: "deadbeef",
        build_time: "now",
      }),
    );

    const { result } = renderHookWithQuery(() => useVersion());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith("/version");
  });

  it("caches the result under the ['version'] query key", async () => {
    mockGet.mockResolvedValue(
      ok({
        version: "v2.0.0",
        service: "scanorama",
        commit: "c0ffee",
        build_time: "yesterday",
      }),
    );

    const { result, queryClient } = renderHookWithQuery(() => useVersion());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["version"]);
    expect(cached).toBeDefined();
    expect((cached as { version: string }).version).toBe("v2.0.0");
  });
});

// ── useStatus ─────────────────────────────────────────────────────────────────

describe("useStatus", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useStatus());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns status data on success", async () => {
    const payload = {
      status: "running",
      uptime: "5h10m",
      active_scans: 2,
      queued_scans: 0,
    };
    mockGet.mockResolvedValue(ok(payload));

    const { result } = renderHookWithQuery(() => useStatus());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeDefined();
    expect((result.current.data as typeof payload).status).toBe("running");
    expect((result.current.data as typeof payload).active_scans).toBe(2);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal error"));

    const { result } = renderHookWithQuery(() => useStatus());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("calls api.GET with the /status path", async () => {
    mockGet.mockResolvedValue(ok({ status: "running" }));

    const { result } = renderHookWithQuery(() => useStatus());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith("/status");
  });

  it("caches the result under the ['status'] query key", async () => {
    mockGet.mockResolvedValue(ok({ status: "idle", active_scans: 0 }));

    const { result, queryClient } = renderHookWithQuery(() => useStatus());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["status"]);
    expect(cached).toBeDefined();
  });
});
