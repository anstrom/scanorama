import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useDevices,
  useDevice,
  useCreateDevice,
  useUpdateDevice,
  useDeleteDevice,
  useAttachHost,
  useDetachHost,
  useAcceptSuggestion,
  useDismissSuggestion,
} from "./use-devices";

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

const ok = (data: unknown) =>
  Promise.resolve({ data, error: undefined, response: new Response() }) as ReturnType<typeof mockGet>;

const fail = (msg = "boom") =>
  Promise.resolve({
    data: undefined,
    error: { message: msg },
    response: new Response(null, { status: 500 }),
  }) as ReturnType<typeof mockGet>;

const okMut = (data: unknown) =>
  Promise.resolve({ data, error: undefined, response: new Response() }) as ReturnType<typeof mockPost>;

const failMut = (msg = "boom") =>
  Promise.resolve({
    data: undefined,
    error: { message: msg },
    response: new Response(null, { status: 500 }),
  }) as ReturnType<typeof mockPost>;

// ── useDevices ────────────────────────────────────────────────────────────────

describe("useDevices", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns the device list on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/devices");
      return ok({ devices: [{ id: "d1", name: "Lab Pi", mac_count: 2, host_count: 1 }] });
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useDevices());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect((result.current.data as { name: string }[])[0].name).toBe("Lab Pi");
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useDevices());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

// ── useDevice ─────────────────────────────────────────────────────────────────

describe("useDevice", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns device detail on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/devices/{id}");
      return ok({ id: "d1", name: "Lab Pi", known_macs: [], known_names: [], hosts: [] });
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useDevice("d1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect((result.current.data as { name: string }).name).toBe("Lab Pi");
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useDevice("d1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHookWithQuery(() => useDevice(""));
    expect(result.current.fetchStatus).toBe("idle");
  });
});

// ── useCreateDevice ───────────────────────────────────────────────────────────

describe("useCreateDevice", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /devices with the provided body", async () => {
    mockPost.mockResolvedValue(okMut({ id: "d2", name: "Core Switch" }));

    const { result, actHook } = renderHookWithQuery(() => useCreateDevice());
    await actHook(async () => {
      await result.current.mutateAsync({ name: "Core Switch" });
    });

    expect(mockPost).toHaveBeenCalledWith("/devices", expect.objectContaining({
      body: { name: "Core Switch" },
    }));
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(failMut("name required") as ReturnType<typeof mockPost>);

    const { result, actHook } = renderHookWithQuery(() => useCreateDevice());
    await expect(
      actHook(async () => { await result.current.mutateAsync({ name: "" }); }),
    ).rejects.toThrow();
  });
});

// ── useUpdateDevice ───────────────────────────────────────────────────────────

describe("useUpdateDevice", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls PUT /devices/{id} with the provided body", async () => {
    mockPut.mockResolvedValue(okMut({ id: "d1", name: "Updated" }) as ReturnType<typeof mockPut>);

    const { result, actHook } = renderHookWithQuery(() => useUpdateDevice());
    await actHook(async () => {
      await result.current.mutateAsync({ id: "d1", body: { name: "Updated" } });
    });

    expect(mockPut).toHaveBeenCalledWith("/devices/{id}", expect.objectContaining({
      params: { path: { id: "d1" } },
      body: { name: "Updated" },
    }));
  });

  it("throws on error", async () => {
    mockPut.mockResolvedValue(failMut("not found") as ReturnType<typeof mockPut>);

    const { result, actHook } = renderHookWithQuery(() => useUpdateDevice());
    await expect(
      actHook(async () => { await result.current.mutateAsync({ id: "d1", body: { name: "x" } }); }),
    ).rejects.toThrow();
  });
});

// ── useDeleteDevice ───────────────────────────────────────────────────────────

describe("useDeleteDevice", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls DELETE /devices/{id}", async () => {
    mockDelete.mockResolvedValue(okMut(undefined) as ReturnType<typeof mockDelete>);

    const { result, actHook } = renderHookWithQuery(() => useDeleteDevice());
    await actHook(async () => { await result.current.mutateAsync("d1"); });

    expect(mockDelete).toHaveBeenCalledWith("/devices/{id}", expect.objectContaining({
      params: { path: { id: "d1" } },
    }));
  });

  it("throws on error", async () => {
    mockDelete.mockResolvedValue(failMut("not found") as ReturnType<typeof mockDelete>);

    const { result, actHook } = renderHookWithQuery(() => useDeleteDevice());
    await expect(
      actHook(async () => { await result.current.mutateAsync("d1"); }),
    ).rejects.toThrow();
  });
});

// ── useAttachHost ─────────────────────────────────────────────────────────────

describe("useAttachHost", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /devices/{id}/hosts/{host_id}", async () => {
    mockPost.mockResolvedValue(okMut(undefined));

    const { result, actHook } = renderHookWithQuery(() => useAttachHost());
    await actHook(async () => {
      await result.current.mutateAsync({ deviceId: "d1", hostId: "h1" });
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/devices/{id}/hosts/{host_id}",
      expect.objectContaining({ params: { path: { id: "d1", host_id: "h1" } } }),
    );
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(failMut("conflict"));

    const { result, actHook } = renderHookWithQuery(() => useAttachHost());
    await expect(
      actHook(async () => { await result.current.mutateAsync({ deviceId: "d1", hostId: "h1" }); }),
    ).rejects.toThrow();
  });
});

// ── useDetachHost ─────────────────────────────────────────────────────────────

describe("useDetachHost", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls DELETE /devices/{id}/hosts/{host_id}", async () => {
    mockDelete.mockResolvedValue(okMut(undefined) as ReturnType<typeof mockDelete>);

    const { result, actHook } = renderHookWithQuery(() => useDetachHost());
    await actHook(async () => {
      await result.current.mutateAsync({ deviceId: "d1", hostId: "h1" });
    });

    expect(mockDelete).toHaveBeenCalledWith(
      "/devices/{id}/hosts/{host_id}",
      expect.objectContaining({ params: { path: { id: "d1", host_id: "h1" } } }),
    );
  });

  it("throws on error", async () => {
    mockDelete.mockResolvedValue(failMut("not found") as ReturnType<typeof mockDelete>);

    const { result, actHook } = renderHookWithQuery(() => useDetachHost());
    await expect(
      actHook(async () => { await result.current.mutateAsync({ deviceId: "d1", hostId: "h1" }); }),
    ).rejects.toThrow();
  });
});

// ── useAcceptSuggestion ───────────────────────────────────────────────────────

describe("useAcceptSuggestion", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /devices/suggestions/{id}/accept", async () => {
    mockPost.mockResolvedValue(okMut(undefined));

    const { result, actHook } = renderHookWithQuery(() => useAcceptSuggestion());
    await actHook(async () => { await result.current.mutateAsync("s1"); });

    expect(mockPost).toHaveBeenCalledWith(
      "/devices/suggestions/{id}/accept",
      expect.objectContaining({ params: { path: { id: "s1" } } }),
    );
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(failMut("not found"));

    const { result, actHook } = renderHookWithQuery(() => useAcceptSuggestion());
    await expect(
      actHook(async () => { await result.current.mutateAsync("s1"); }),
    ).rejects.toThrow();
  });
});

// ── useDismissSuggestion ──────────────────────────────────────────────────────

describe("useDismissSuggestion", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /devices/suggestions/{id}/dismiss", async () => {
    mockPost.mockResolvedValue(okMut(undefined));

    const { result, actHook } = renderHookWithQuery(() => useDismissSuggestion());
    await actHook(async () => { await result.current.mutateAsync("s1"); });

    expect(mockPost).toHaveBeenCalledWith(
      "/devices/suggestions/{id}/dismiss",
      expect.objectContaining({ params: { path: { id: "s1" } } }),
    );
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(failMut("not found"));

    const { result, actHook } = renderHookWithQuery(() => useDismissSuggestion());
    await expect(
      actHook(async () => { await result.current.mutateAsync("s1"); }),
    ).rejects.toThrow();
  });
});
