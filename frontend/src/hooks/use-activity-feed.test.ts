import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock use-ws before importing the hook under test.
const mockOn = vi.fn();
const mockManager = { on: mockOn };

vi.mock("../lib/use-ws", () => ({
  useWs: vi.fn(() => ({ manager: mockManager })),
}));

import { useActivityFeed } from "./use-activity-feed";

// ── Helpers ───────────────────────────────────────────────────────────────────

type EventCallback = (msg: object) => void;

/**
 * Returns the registered handler for a given WS event type, or throws if none.
 * Relies on mockOn having been called as: manager.on("event_type", handler).
 */
function getHandler(eventType: string): EventCallback {
  const call = mockOn.mock.calls.find((c) => c[0] === eventType);
  if (!call) throw new Error(`No handler registered for "${eventType}"`);
  return call[1] as EventCallback;
}

function ts() {
  return new Date().toISOString();
}

// ── Tests ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  vi.clearAllMocks();
  // Reset so each test has a fresh subscription list.
  mockOn.mockReturnValue(() => {}); // returns an unsubscribe fn
});

describe("useActivityFeed", () => {
  it("returns empty array initially", () => {
    const { result } = renderHook(() => useActivityFeed());
    expect(result.current).toEqual([]);
  });

  it("subscribes to scan_update, discovery_update, host_status_change", () => {
    renderHook(() => useActivityFeed());
    const eventTypes = mockOn.mock.calls.map((c) => c[0]);
    expect(eventTypes).toContain("scan_update");
    expect(eventTypes).toContain("discovery_update");
    expect(eventTypes).toContain("host_status_change");
  });

  // ── scan_update ────────────────────────────────────────────────────────────

  it("adds scan_started event on running status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({ data: { scan_id: "42", status: "running" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(1);
    expect(result.current[0].kind).toBe("scan_started");
    expect(result.current[0].title).toBe("Scan started");
    expect(result.current[0].detail).toBe("Scan #42");
  });

  it("adds scan_started event on queued status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({ data: { scan_id: "7", status: "queued" }, timestamp: ts() });
    });

    expect(result.current[0].kind).toBe("scan_started");
  });

  it("adds scan_completed event with results count", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({
        data: { scan_id: "1", status: "completed", results_count: 5 },
        timestamp: ts(),
      });
    });

    expect(result.current[0].kind).toBe("scan_completed");
    expect(result.current[0].title).toBe("Scan completed");
    expect(result.current[0].detail).toContain("5 results");
  });

  it("adds scan_failed event with error message", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({
        data: { scan_id: "2", status: "failed", error: "nmap not found" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].kind).toBe("scan_failed");
    expect(result.current[0].detail).toContain("nmap not found");
  });

  it("adds scan_failed event on error status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({ data: { scan_id: "3", status: "error" }, timestamp: ts() });
    });

    expect(result.current[0].kind).toBe("scan_failed");
  });

  it("suppresses duplicate scan status ticks", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");
    const now = ts();

    act(() => {
      handler({ data: { scan_id: "5", status: "running" }, timestamp: now });
      handler({ data: { scan_id: "5", status: "running" }, timestamp: now });
      handler({ data: { scan_id: "5", status: "running" }, timestamp: now });
    });

    // Only the first transition fires.
    expect(result.current).toHaveLength(1);
  });

  it("fires again when scan transitions to a new status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({ data: { scan_id: "6", status: "running" }, timestamp: ts() });
      handler({ data: { scan_id: "6", status: "completed" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(2);
    expect(result.current[0].kind).toBe("scan_completed"); // newest first
    expect(result.current[1].kind).toBe("scan_started");
  });

  it("ignores scan_update with unknown status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("scan_update");

    act(() => {
      handler({ data: { scan_id: "9", status: "pending" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(0);
  });

  // ── discovery_update ───────────────────────────────────────────────────────

  it("adds discovery_started event on running status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("discovery_update");

    act(() => {
      handler({
        data: { job_id: "abc123def", status: "running" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].kind).toBe("discovery_started");
    expect(result.current[0].detail).toContain("abc123de");
  });

  it("adds discovery_completed event with new/gone counts", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("discovery_update");

    act(() => {
      handler({
        data: {
          job_id: "xyz",
          status: "completed",
          new_hosts_count: 3,
          gone_hosts_count: 1,
        },
        timestamp: ts(),
      });
    });

    expect(result.current[0].kind).toBe("discovery_completed");
    expect(result.current[0].detail).toContain("+3 new");
    expect(result.current[0].detail).toContain("-1 gone");
  });

  it("adds discovery_completed with 'No changes' when counts are zero", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("discovery_update");

    act(() => {
      handler({
        data: { job_id: "no-change", status: "completed" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].detail).toBe("No changes");
  });

  it("suppresses duplicate discovery status ticks", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("discovery_update");

    act(() => {
      handler({ data: { job_id: "j1", status: "running" }, timestamp: ts() });
      handler({ data: { job_id: "j1", status: "running" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(1);
  });

  it("ignores discovery_update with unknown status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("discovery_update");

    act(() => {
      handler({ data: { job_id: "j2", status: "queued" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(0);
  });

  // ── host_status_change ─────────────────────────────────────────────────────

  it("adds host_status_change event for host going up", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({
        data: { ip_address: "192.168.1.1", new_status: "up" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].kind).toBe("host_status_change");
    expect(result.current[0].title).toBe("Host online");
    expect(result.current[0].detail).toBe("192.168.1.1");
  });

  it("adds host_status_change event for host going down", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({
        data: { ip_address: "10.0.0.5", new_status: "down" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].title).toBe("Host offline");
  });

  it("adds host_status_change event for host gone", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({
        data: { ip_address: "10.0.0.9", new_status: "gone" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].title).toBe("Host gone");
  });

  it("uses generic title for unknown host status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({
        data: { ip_address: "10.0.0.8", new_status: "unknown" },
        timestamp: ts(),
      });
    });

    expect(result.current[0].title).toBe("Host status changed");
  });

  it("ignores host_status_change with no ip_address", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({ data: { new_status: "up" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(0);
  });

  it("ignores host_status_change with no status", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({ data: { ip_address: "10.0.0.1" }, timestamp: ts() });
    });

    expect(result.current).toHaveLength(0);
  });

  // ── General ───────────────────────────────────────────────────────────────

  it("deduplicates events by id", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");
    const fixedTs = "2025-01-01T00:00:00.000Z";

    act(() => {
      // Same ip + status + timestamp → same id → should only appear once
      handler({ data: { ip_address: "10.0.0.1", new_status: "up" }, timestamp: fixedTs });
      handler({ data: { ip_address: "10.0.0.1", new_status: "up" }, timestamp: fixedTs });
    });

    expect(result.current).toHaveLength(1);
  });

  it("prepends new events (newest first)", () => {
    const { result } = renderHook(() => useActivityFeed());
    const handler = getHandler("host_status_change");

    act(() => {
      handler({ data: { ip_address: "10.0.0.1", new_status: "up" }, timestamp: ts() });
      handler({ data: { ip_address: "10.0.0.2", new_status: "down" }, timestamp: ts() });
    });

    expect(result.current[0].detail).toBe("10.0.0.2"); // newest first
    expect(result.current[1].detail).toBe("10.0.0.1");
  });

  it("unsubscribes all handlers on unmount", () => {
    const unsub1 = vi.fn();
    const unsub2 = vi.fn();
    const unsub3 = vi.fn();
    mockOn
      .mockReturnValueOnce(unsub1)
      .mockReturnValueOnce(unsub2)
      .mockReturnValueOnce(unsub3);

    const { unmount } = renderHook(() => useActivityFeed());
    unmount();

    expect(unsub1).toHaveBeenCalled();
    expect(unsub2).toHaveBeenCalled();
    expect(unsub3).toHaveBeenCalled();
  });

  it("does nothing when manager is null", async () => {
    const { useWs } = await import("../lib/use-ws");
    vi.mocked(useWs).mockReturnValueOnce({ manager: null, status: "disconnected" });

    const { result } = renderHook(() => useActivityFeed());
    expect(result.current).toEqual([]);
    // No subscriptions should have been attempted
    expect(mockOn).not.toHaveBeenCalled();
  });
});
