import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { useProfiles, useProfile } from "./use-profiles";

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

const mockProfile = {
  id: "p1",
  name: "Quick scan",
  description: "Fast TCP connect scan",
  scan_type: "connect",
  ports: "22,80,443",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const mockProfile2 = {
  id: "p2",
  name: "Full scan",
  description: "Comprehensive SYN scan",
  scan_type: "syn",
  ports: "1-65535",
  created_at: "2024-01-02T00:00:00Z",
  updated_at: "2024-01-02T00:00:00Z",
};

const mockPagination = {
  page: 1,
  page_size: 20,
  total_items: 2,
  total_pages: 1,
};

// ── useProfiles ───────────────────────────────────────────────────────────────

describe("useProfiles", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useProfiles());
    expect(result.current.isLoading).toBe(true);
  });

  it("returns profile list and pagination on success", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [mockProfile, mockProfile2], pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useProfiles());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(2);
    expect(result.current.data?.data?.[0].id).toBe("p1");
    expect(result.current.data?.data?.[1].id).toBe("p2");
    expect(result.current.data?.pagination?.total_items).toBe(2);
    expect(result.current.data?.pagination?.page).toBe(1);
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("internal server error"));

    const { result } = renderHookWithQuery(() => useProfiles());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("forwards page and page_size as query params", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [], pagination: { ...mockPagination, total_items: 0 } }),
    );

    const { result } = renderHookWithQuery(() =>
      useProfiles({ page: 2, page_size: 10 }),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/profiles",
      expect.objectContaining({
        params: { query: { page: 2, page_size: 10 } },
      }),
    );
  });

  it("returns an empty data array when there are no profiles", async () => {
    mockGet.mockResolvedValue(
      ok({
        data: [],
        pagination: { ...mockPagination, total_items: 0, total_pages: 0 },
      }),
    );

    const { result } = renderHookWithQuery(() => useProfiles());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.data).toHaveLength(0);
  });

  it("calls api.GET with the /profiles path", async () => {
    mockGet.mockResolvedValue(
      ok({ data: [mockProfile], pagination: mockPagination }),
    );

    const { result } = renderHookWithQuery(() => useProfiles());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/profiles",
      expect.objectContaining({ params: { query: {} } }),
    );
  });

  it("caches the result under the ['profiles', params] query key", async () => {
    const params = { page: 1, page_size: 20 };
    mockGet.mockResolvedValue(
      ok({ data: [mockProfile], pagination: mockPagination }),
    );

    const { result, queryClient } = renderHookWithQuery(() =>
      useProfiles(params),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["profiles", params]);
    expect(cached).toBeDefined();
  });
});

// ── useProfile ────────────────────────────────────────────────────────────────

describe("useProfile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("starts in a loading state when an id is provided", () => {
    mockGet.mockReturnValue(new Promise(() => {}));
    const { result } = renderHookWithQuery(() => useProfile("p1"));
    expect(result.current.isLoading).toBe(true);
  });

  it("returns the profile data on success", async () => {
    mockGet.mockResolvedValue(ok(mockProfile));

    const { result } = renderHookWithQuery(() => useProfile("p1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe("p1");
    expect(result.current.data?.name).toBe("Quick scan");
    expect(result.current.data?.scan_type).toBe("connect");
    expect(result.current.data?.ports).toBe("22,80,443");
    expect(result.current.data?.description).toBe("Fast TCP connect scan");
  });

  it("enters error state when api.GET returns an error", async () => {
    mockGet.mockResolvedValue(fail("not found"));

    const { result } = renderHookWithQuery(() => useProfile("nonexistent"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it("is disabled and does not fetch when id is undefined", () => {
    const { result } = renderHookWithQuery(() => useProfile(undefined));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.fetchStatus).toBe("idle");
    expect(result.current.data).toBeUndefined();
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("is disabled and does not fetch when id is empty string", () => {
    const { result } = renderHookWithQuery(() => useProfile(""));
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("forwards the profile id as a path param", async () => {
    mockGet.mockResolvedValue(ok(mockProfile));

    const { result } = renderHookWithQuery(() => useProfile("p1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith(
      "/profiles/{profileId}",
      expect.objectContaining({ params: { path: { profileId: "p1" } } }),
    );
  });

  it("caches the result under the ['profiles', id] query key", async () => {
    mockGet.mockResolvedValue(ok(mockProfile));

    const { result, queryClient } = renderHookWithQuery(() => useProfile("p1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const cached = queryClient.getQueryData(["profiles", "p1"]);
    expect(cached).toBeDefined();
    expect((cached as typeof mockProfile).name).toBe("Quick scan");
  });
});
