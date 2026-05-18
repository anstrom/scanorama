import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useRecentPages, type RecentPage } from "./use-recent-pages";

// ── localStorage mock ──────────────────────────────────────────────────────────
// Node 26 has an experimental (broken) global localStorage that conflicts with
// jsdom's. We stub it with a working in-memory implementation.

function makeStorageMock(): Storage {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      Object.keys(store).forEach((k) => delete store[k]);
    },
    get length() {
      return Object.keys(store).length;
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
  };
}

let storageMock: Storage;

const STORAGE_KEY = "scanorama_recent_pages";

function makeRecent(label: string, url: string): RecentPage {
  return { label, url, type: "recent" };
}

describe("useRecentPages", () => {
  beforeEach(() => {
    storageMock = makeStorageMock();
    vi.stubGlobal("localStorage", storageMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns an empty list when localStorage is empty", () => {
    const { result } = renderHook(() => useRecentPages());
    expect(result.current.recentPages).toEqual([]);
  });

  it("adds a page and returns it", () => {
    const { result } = renderHook(() => useRecentPages());

    act(() => {
      result.current.addRecentPage(makeRecent("Hosts", "/hosts"));
    });

    expect(result.current.recentPages).toHaveLength(1);
    expect(result.current.recentPages[0].url).toBe("/hosts");
    expect(result.current.recentPages[0].label).toBe("Hosts");
    expect(result.current.recentPages[0].type).toBe("recent");
  });

  it("persists to localStorage", () => {
    const { result } = renderHook(() => useRecentPages());

    act(() => {
      result.current.addRecentPage(makeRecent("Hosts", "/hosts"));
    });

    const stored = JSON.parse(
      storageMock.getItem(STORAGE_KEY) ?? "[]",
    ) as RecentPage[];
    expect(stored).toHaveLength(1);
    expect(stored[0].url).toBe("/hosts");
  });

  it("deduplicates entries by URL (keeps the latest visit first)", () => {
    const { result } = renderHook(() => useRecentPages());

    act(() => {
      result.current.addRecentPage(makeRecent("Hosts", "/hosts"));
    });
    act(() => {
      result.current.addRecentPage(makeRecent("Scans", "/scans"));
    });
    act(() => {
      result.current.addRecentPage(makeRecent("Hosts", "/hosts"));
    });

    expect(result.current.recentPages).toHaveLength(2);
    expect(result.current.recentPages[0].url).toBe("/hosts");
    expect(result.current.recentPages[1].url).toBe("/scans");
  });

  it("limits to 5 entries (newest first)", () => {
    const { result } = renderHook(() => useRecentPages());

    act(() => {
      for (let i = 1; i <= 7; i++) {
        result.current.addRecentPage(makeRecent(`Page ${i}`, `/page/${i}`));
      }
    });

    expect(result.current.recentPages).toHaveLength(5);
    // Newest is last-added (page 7).
    expect(result.current.recentPages[0].url).toBe("/page/7");
    // Oldest retained is page 3 (pages 1 and 2 were evicted).
    expect(result.current.recentPages[4].url).toBe("/page/3");
  });

  it("loads existing entries from localStorage on mount", () => {
    const existing: RecentPage[] = [
      makeRecent("Networks", "/networks"),
      makeRecent("Admin", "/admin"),
    ];
    storageMock.setItem(STORAGE_KEY, JSON.stringify(existing));

    const { result } = renderHook(() => useRecentPages());

    expect(result.current.recentPages).toHaveLength(2);
    expect(result.current.recentPages[0].url).toBe("/networks");
  });
});
