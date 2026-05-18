import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useWebhooks,
  useWebhook,
  useCreateWebhook,
  useUpdateWebhook,
  useDeleteWebhook,
  useTestWebhook,
  useDeliveryLogs,
  type WebhookEndpoint,
  type WebhookDeliveryLog,
} from "./use-webhooks";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
    POST: vi.fn(),
    PATCH: vi.fn(),
    DELETE: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);
const mockPost = vi.mocked(api.POST);
const mockPatch = vi.mocked(api.PATCH);
const mockDelete = vi.mocked(api.DELETE);

const ok = (data: unknown) =>
  ({ data, error: undefined, response: new Response() }) as Awaited<ReturnType<typeof mockGet>>;

const fail = (msg = "error") =>
  ({
    data: undefined,
    error: { message: msg },
    response: new Response(null, { status: 500 }),
  }) as Awaited<ReturnType<typeof mockGet>>;

const okMut = (data: unknown) =>
  ({ data, error: undefined, response: new Response() }) as Awaited<ReturnType<typeof mockPost>>;

const failMut = (msg = "error") =>
  ({
    data: undefined,
    error: { message: msg },
    response: new Response(null, { status: 500 }),
  }) as Awaited<ReturnType<typeof mockPost>>;

const sampleEndpoint: WebhookEndpoint = {
  id: "ep-1",
  url: "https://example.com/hook",
  secret: "s3cr3t",
  events: ["host.online"],
  enabled: true,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const sampleLog: WebhookDeliveryLog = {
  id: "log-1",
  endpoint_id: "ep-1",
  event_type: "host.online",
  status_code: 200,
  attempt_count: 1,
  last_error: null,
  delivered_at: "2024-01-01T00:00:01Z",
  created_at: "2024-01-01T00:00:00Z",
};

// ── useWebhooks ───────────────────────────────────────────────────────────────

describe("useWebhooks", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns the webhook list on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/webhooks");
      return ok({ webhooks: [sampleEndpoint] });
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useWebhooks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect((result.current.data as WebhookEndpoint[])[0].url).toBe("https://example.com/hook");
  });

  it("returns empty array on empty response", async () => {
    mockGet.mockImplementation((() => ok({ webhooks: [] })) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useWebhooks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useWebhooks());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

// ── useWebhook ────────────────────────────────────────────────────────────────

describe("useWebhook", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns endpoint detail on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/webhooks/{id}");
      return ok(sampleEndpoint);
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useWebhook("ep-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect((result.current.data as WebhookEndpoint).url).toBe("https://example.com/hook");
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useWebhook("ep-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHookWithQuery(() => useWebhook(""));
    expect(result.current.fetchStatus).toBe("idle");
  });
});

// ── useCreateWebhook ──────────────────────────────────────────────────────────

describe("useCreateWebhook", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /webhooks with the provided body", async () => {
    mockPost.mockResolvedValue(okMut(sampleEndpoint));

    const { result, actHook } = renderHookWithQuery(() => useCreateWebhook());
    await actHook(async () => {
      await result.current.mutateAsync({ url: "https://example.com/hook", events: ["host.online"] });
    });

    expect(mockPost).toHaveBeenCalledWith("/webhooks", expect.objectContaining({
      body: expect.objectContaining({ url: "https://example.com/hook" }),
    }));
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(failMut("bad url") as Awaited<ReturnType<typeof mockPost>>);

    const { result, actHook } = renderHookWithQuery(() => useCreateWebhook());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync({ url: "", events: [] });
      }),
    ).rejects.toThrow();
  });
});

// ── useUpdateWebhook ──────────────────────────────────────────────────────────

describe("useUpdateWebhook", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls PATCH /webhooks/{id} with the provided body", async () => {
    mockPatch.mockResolvedValue(okMut(sampleEndpoint) as Awaited<ReturnType<typeof mockPatch>>);

    const { result, actHook } = renderHookWithQuery(() => useUpdateWebhook());
    await actHook(async () => {
      await result.current.mutateAsync({ id: "ep-1", body: { enabled: false } });
    });

    expect(mockPatch).toHaveBeenCalledWith("/webhooks/{id}", expect.objectContaining({
      params: { path: { id: "ep-1" } },
      body: expect.objectContaining({ enabled: false }),
    }));
  });

  it("throws on error", async () => {
    mockPatch.mockResolvedValue(failMut("not found") as Awaited<ReturnType<typeof mockPatch>>);

    const { result, actHook } = renderHookWithQuery(() => useUpdateWebhook());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync({ id: "ep-1", body: { enabled: false } });
      }),
    ).rejects.toThrow();
  });
});

// ── useDeleteWebhook ──────────────────────────────────────────────────────────

describe("useDeleteWebhook", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls DELETE /webhooks/{id}", async () => {
    mockDelete.mockResolvedValue(okMut(undefined) as Awaited<ReturnType<typeof mockDelete>>);

    const { result, actHook } = renderHookWithQuery(() => useDeleteWebhook());
    await actHook(async () => {
      await result.current.mutateAsync("ep-1");
    });

    expect(mockDelete).toHaveBeenCalledWith("/webhooks/{id}", expect.objectContaining({
      params: { path: { id: "ep-1" } },
    }));
  });

  it("throws on error", async () => {
    mockDelete.mockResolvedValue(failMut("not found") as Awaited<ReturnType<typeof mockDelete>>);

    const { result, actHook } = renderHookWithQuery(() => useDeleteWebhook());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync("ep-1");
      }),
    ).rejects.toThrow();
  });
});

// ── useTestWebhook ────────────────────────────────────────────────────────────

describe("useTestWebhook", () => {
  beforeEach(() => vi.resetAllMocks());

  it("calls POST /webhooks/{id}/test", async () => {
    mockPost.mockResolvedValue(okMut({ status: "delivered" }));

    const { result, actHook } = renderHookWithQuery(() => useTestWebhook());
    await actHook(async () => {
      await result.current.mutateAsync("ep-1");
    });

    expect(mockPost).toHaveBeenCalledWith(
      "/webhooks/{id}/test",
      expect.objectContaining({ params: { path: { id: "ep-1" } } }),
    );
  });

  it("throws on delivery failure", async () => {
    mockPost.mockResolvedValue(failMut("endpoint returned 500"));

    const { result, actHook } = renderHookWithQuery(() => useTestWebhook());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync("ep-1");
      }),
    ).rejects.toThrow();
  });
});

// ── useDeliveryLogs ───────────────────────────────────────────────────────────

describe("useDeliveryLogs", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns log list on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/webhooks/{id}/logs");
      return ok({ logs: [sampleLog] });
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useDeliveryLogs("ep-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect((result.current.data as WebhookDeliveryLog[])[0].event_type).toBe("host.online");
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useDeliveryLogs("ep-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHookWithQuery(() => useDeliveryLogs(""));
    expect(result.current.fetchStatus).toBe("idle");
  });
});
