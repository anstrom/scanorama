import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import {
  useAlertRules,
  useHostAlertRules,
  useCreateAlertRule,
  useUpdateAlertRule,
  useDeleteAlertRule,
  type AlertRule,
} from "./use-alerts";

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

const ok = (data: unknown): Awaited<ReturnType<typeof mockGet>> =>
  ({ data, error: undefined, response: new Response() }) as Awaited<
    ReturnType<typeof mockGet>
  >;

const fail = (message = "error"): Awaited<ReturnType<typeof mockGet>> =>
  ({ data: undefined, error: { message }, response: new Response() }) as Awaited<
    ReturnType<typeof mockGet>
  >;

// ── sample data ───────────────────────────────────────────────────────────────

const sampleRule: AlertRule = {
  id: "rule-1",
  host_id: "host-1",
  group_id: null,
  tag: null,
  trigger: "online",
  channel_type: "webhook",
  channel_url: "https://example.com/hook",
  enabled: true,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

// ── useAlertRules ─────────────────────────────────────────────────────────────

describe("useAlertRules", () => {
  afterEach(() => vi.resetAllMocks());

  it("returns the rule list on success", async () => {
    mockGet.mockResolvedValue(ok({ alert_rules: [sampleRule] }));

    const { result } = renderHookWithQuery(() => useAlertRules());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect((result.current.data as AlertRule[])[0].trigger).toBe("online");
  });

  it("returns empty array when list is empty", async () => {
    mockGet.mockResolvedValue(ok({ alert_rules: [] }));

    const { result } = renderHookWithQuery(() => useAlertRules());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("surfaces errors", async () => {
    mockGet.mockResolvedValue(fail());

    const { result } = renderHookWithQuery(() => useAlertRules());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

// ── useHostAlertRules ─────────────────────────────────────────────────────────

describe("useHostAlertRules", () => {
  afterEach(() => vi.resetAllMocks());

  it("returns rules for a given host on success", async () => {
    mockGet.mockResolvedValue(ok({ alert_rules: [sampleRule] }));

    const { result } = renderHookWithQuery(() => useHostAlertRules("host-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect((result.current.data as AlertRule[])[0].channel_url).toBe(
      "https://example.com/hook",
    );
  });

  it("is disabled when hostID is empty", () => {
    const { result } = renderHookWithQuery(() => useHostAlertRules(""));
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("surfaces errors", async () => {
    mockGet.mockResolvedValue(fail());

    const { result } = renderHookWithQuery(() => useHostAlertRules("host-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

// ── useCreateAlertRule ────────────────────────────────────────────────────────

describe("useCreateAlertRule", () => {
  afterEach(() => vi.resetAllMocks());

  it("fires POST and returns the new rule on success", async () => {
    mockPost.mockResolvedValue(ok(sampleRule));

    const { result, actHook } = renderHookWithQuery(() => useCreateAlertRule());
    let created: AlertRule | undefined;
    await actHook(async () => {
      created = await result.current.mutateAsync({
        host_id: "host-1",
        trigger: "online",
        channel_url: "https://example.com/hook",
      });
    });

    expect(created?.trigger).toBe("online");
  });

  it("throws on error", async () => {
    mockPost.mockResolvedValue(fail("bad trigger"));

    const { result, actHook } = renderHookWithQuery(() => useCreateAlertRule());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync({
          host_id: "host-1",
          trigger: "online",
          channel_url: "https://example.com/hook",
        });
      }),
    ).rejects.toThrow();
  });
});

// ── useUpdateAlertRule ────────────────────────────────────────────────────────

describe("useUpdateAlertRule", () => {
  afterEach(() => vi.resetAllMocks());

  it("fires PATCH and returns updated rule on success", async () => {
    const updated = { ...sampleRule, enabled: false };
    mockPatch.mockResolvedValue(ok(updated));

    const { result, actHook } = renderHookWithQuery(() => useUpdateAlertRule());
    let updatedRule: AlertRule | undefined;
    await actHook(async () => {
      updatedRule = await result.current.mutateAsync({
        id: "rule-1",
        body: { enabled: false },
      });
    });

    expect(updatedRule?.enabled).toBe(false);
  });

  it("throws on error", async () => {
    mockPatch.mockResolvedValue(fail("bad trigger"));

    const { result, actHook } = renderHookWithQuery(() => useUpdateAlertRule());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync({
          id: "rule-1",
          body: { trigger: "invalid" as "online" },
        });
      }),
    ).rejects.toThrow();
  });
});

// ── useDeleteAlertRule ────────────────────────────────────────────────────────

describe("useDeleteAlertRule", () => {
  afterEach(() => vi.resetAllMocks());

  it("fires DELETE and resolves on success", async () => {
    mockDelete.mockResolvedValue(ok(undefined));

    const { result, actHook } = renderHookWithQuery(() => useDeleteAlertRule());
    let resolved = false;
    await actHook(async () => {
      await result.current.mutateAsync({ id: "rule-1", hostID: "host-1" });
      resolved = true;
    });

    expect(resolved).toBe(true);
  });

  it("throws on error", async () => {
    mockDelete.mockResolvedValue(fail("not found"));

    const { result, actHook } = renderHookWithQuery(() => useDeleteAlertRule());
    await expect(
      actHook(async () => {
        await result.current.mutateAsync({ id: "rule-1" });
      }),
    ).rejects.toThrow();
  });
});
