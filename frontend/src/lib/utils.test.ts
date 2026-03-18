import { describe, it, expect, vi, afterEach } from "vitest";
import { formatRelativeTime, formatAbsoluteTime } from "./utils";

afterEach(() => {
  vi.useRealTimers();
});

// ── formatRelativeTime ────────────────────────────────────────────────────────

describe("formatRelativeTime", () => {
  function freeze(isoNow: string) {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(isoNow));
  }

  it('returns "just now" for 0 seconds ago', () => {
    freeze("2024-06-01T12:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("just now");
  });

  it('returns "just now" for 59 seconds ago', () => {
    freeze("2024-06-01T12:00:59Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("just now");
  });

  it('returns "1m ago" for exactly 60 seconds ago', () => {
    freeze("2024-06-01T12:01:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1m ago");
  });

  it('returns "30m ago" for 30 minutes ago', () => {
    freeze("2024-06-01T12:30:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("30m ago");
  });

  it('returns "59m ago" for 59 minutes ago', () => {
    freeze("2024-06-01T12:59:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("59m ago");
  });

  it('returns "1h ago" for exactly 1 hour ago', () => {
    freeze("2024-06-01T13:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1h ago");
  });

  it('returns "5h ago" for 5 hours ago', () => {
    freeze("2024-06-01T17:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("5h ago");
  });

  it('returns "23h ago" for 23 hours ago', () => {
    freeze("2024-06-02T11:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("23h ago");
  });

  it('returns "1d ago" for exactly 1 day ago', () => {
    freeze("2024-06-02T12:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1d ago");
  });

  it('returns "3d ago" for 3 days ago', () => {
    freeze("2024-06-04T12:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("3d ago");
  });

  it('returns "6d ago" for 6 days ago', () => {
    freeze("2024-06-07T12:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("6d ago");
  });

  it("returns a locale date string for 7 days ago (exactly at boundary)", () => {
    freeze("2024-06-08T12:00:00Z");
    const result = formatRelativeTime("2024-06-01T12:00:00Z");
    // Should not be a relative string — falls through to toLocaleDateString()
    expect(result).not.toMatch(/ago$/);
    expect(result).not.toBe("just now");
    expect(result.length).toBeGreaterThan(0);
  });

  it("returns a locale date string for dates older than a week", () => {
    freeze("2024-07-01T12:00:00Z");
    const result = formatRelativeTime("2024-06-01T12:00:00Z");
    expect(result).not.toMatch(/ago$/);
    expect(result.length).toBeGreaterThan(0);
  });

  it("accepts a Date object as input", () => {
    freeze("2024-06-01T12:05:00Z");
    const date = new Date("2024-06-01T12:00:00Z");
    expect(formatRelativeTime(date)).toBe("5m ago");
  });

  it("floors minutes correctly (89 seconds → 1m ago)", () => {
    freeze("2024-06-01T12:01:29Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1m ago");
  });

  it("floors hours correctly (119 minutes → 1h ago)", () => {
    freeze("2024-06-01T13:59:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1h ago");
  });

  it("floors days correctly (47 hours → 1d ago)", () => {
    freeze("2024-06-03T11:00:00Z");
    expect(formatRelativeTime("2024-06-01T12:00:00Z")).toBe("1d ago");
  });
});

// ── formatAbsoluteTime ────────────────────────────────────────────────────────

describe("formatAbsoluteTime", () => {
  it("accepts an ISO string and returns a non-empty string", () => {
    const result = formatAbsoluteTime("2024-06-01T12:00:00Z");
    expect(result).toBeTruthy();
    expect(typeof result).toBe("string");
  });

  it("accepts a Date object and returns a non-empty string", () => {
    const result = formatAbsoluteTime(new Date("2024-06-01T12:00:00Z"));
    expect(result).toBeTruthy();
    expect(typeof result).toBe("string");
  });

  it("produces the same output for equivalent string and Date inputs", () => {
    const iso = "2024-06-01T12:00:00Z";
    expect(formatAbsoluteTime(iso)).toBe(formatAbsoluteTime(new Date(iso)));
  });

  it("produces different output for different timestamps", () => {
    const a = formatAbsoluteTime("2024-01-01T00:00:00Z");
    const b = formatAbsoluteTime("2024-12-31T23:59:59Z");
    expect(a).not.toBe(b);
  });
});
