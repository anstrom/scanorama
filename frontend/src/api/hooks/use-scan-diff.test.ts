import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { useScanDiff } from "./use-scan-diff";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
  },
}));

import { api } from "../client";
const mockGet = vi.mocked(api.GET);

const ok = (data: unknown): Awaited<ReturnType<typeof mockGet>> =>
  ({
    data,
    error: undefined,
    response: new Response(),
  }) as Awaited<ReturnType<typeof mockGet>>;

const fail = (message = "error"): Awaited<ReturnType<typeof mockGet>> =>
  ({
    data: undefined,
    error: { message },
    response: new Response(),
  }) as Awaited<ReturnType<typeof mockGet>>;

const mockDiff = {
  scan_a_id: "aaaaaaaa-0000-0000-0000-000000000001",
  scan_b_id: "bbbbbbbb-0000-0000-0000-000000000002",
  host_id: "cccccccc-0000-0000-0000-000000000003",
  ports: [
    {
      port: 443,
      protocol: "tcp",
      state: "open",
      service_name: "https",
      status: "new",
    },
    {
      port: 22,
      protocol: "tcp",
      state: "open",
      service_name: "ssh",
      status: "unchanged",
    },
  ],
  os_changed: false,
  new_count: 1,
  closed_count: 0,
  changed_count: 0,
  unchanged_count: 1,
};

const idA = "aaaaaaaa-0000-0000-0000-000000000001";
const idB = "bbbbbbbb-0000-0000-0000-000000000002";

// ── useScanDiff ───────────────────────────────────────────────────────────────

describe("useScanDiff", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("is disabled when scanAId is undefined", () => {
    const { result } = renderHookWithQuery(() => useScanDiff(undefined, idB));
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("is disabled when scanBId is undefined", () => {
    const { result } = renderHookWithQuery(() => useScanDiff(idA, undefined));
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("is disabled when both IDs are undefined", () => {
    const { result } = renderHookWithQuery(() =>
      useScanDiff(undefined, undefined),
    );
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("fetches diff when both IDs are provided (loading then success)", async () => {
    mockGet.mockResolvedValue(ok(mockDiff));
    const { result } = renderHookWithQuery(() => useScanDiff(idA, idB));

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGet).toHaveBeenCalledWith("/scans/diff", expect.anything());
    const diff = result.current.data;
    expect(diff?.scan_a_id).toBe(idA);
    expect(diff?.scan_b_id).toBe(idB);
    expect(diff?.new_count).toBe(1);
    expect(diff?.unchanged_count).toBe(1);
    expect(diff?.os_changed).toBe(false);
    expect(diff?.ports).toHaveLength(2);
    expect(diff?.ports?.[0]?.status).toBe("new");
    expect(diff?.ports?.[1]?.status).toBe("unchanged");
  });

  it("surfaces error state when the API call fails", async () => {
    mockGet.mockResolvedValue(fail("scan not found"));
    const { result } = renderHookWithQuery(() => useScanDiff(idA, idB));

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeDefined();
  });
});
