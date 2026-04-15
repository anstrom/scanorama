import { waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHookWithQuery } from "../../test/utils";
import { useHostNetworks } from "./use-host-networks";

vi.mock("../client", () => ({
  api: {
    GET: vi.fn(),
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

const fail = (message = "boom"): ReturnType<typeof mockGet> =>
  Promise.resolve({
    data: undefined,
    error: { message },
    response: new Response(null, { status: 500 }),
  }) as ReturnType<typeof mockGet>;

describe("useHostNetworks", () => {
  beforeEach(() => vi.resetAllMocks());

  it("returns the list of networks on success", async () => {
    mockGet.mockImplementation(((path: string) => {
      expect(path).toBe("/hosts/{hostId}/networks");
      return ok([
        { id: "a", name: "dmz", cidr: "10.0.0.0/24" },
        { id: "b", name: "corp", cidr: "10.0.0.0/16" },
      ]);
    }) as typeof mockGet);

    const { result } = renderHookWithQuery(() => useHostNetworks("host-1"));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
    expect((result.current.data as { cidr: string }[])[0].cidr).toBe(
      "10.0.0.0/24",
    );
  });

  it("surfaces errors", async () => {
    mockGet.mockImplementation((() => fail()) as typeof mockGet);
    const { result } = renderHookWithQuery(() => useHostNetworks("host-1"));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHookWithQuery(() => useHostNetworks(""));
    expect(result.current.fetchStatus).toBe("idle");
  });
});
