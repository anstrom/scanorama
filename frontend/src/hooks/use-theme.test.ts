import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTheme } from "./use-theme";

// ── localStorage mock ──────────────────────────────────────────────────────────
// Node 26 has an experimental (broken) global localStorage that conflicts with
// jsdom's. We stub it with a working in-memory implementation.

function makeStorageMock(): Storage {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { Object.keys(store).forEach((k) => delete store[k]); },
    get length() { return Object.keys(store).length; },
    key: (index: number) => Object.keys(store)[index] ?? null,
  };
}

let storageMock: Storage;

// ── matchMedia helper ──────────────────────────────────────────────────────────

function makeMatchMedia(prefersLight: boolean) {
  return vi.fn((query: string) => ({
    matches: prefersLight && query === "(prefers-color-scheme: light)",
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
  }));
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("useTheme", () => {
  beforeEach(() => {
    storageMock = makeStorageMock();
    vi.stubGlobal("localStorage", storageMock);
    document.documentElement.removeAttribute("data-theme");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.documentElement.removeAttribute("data-theme");
  });

  it("defaults to dark when no localStorage and no system preference for light", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(false));

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  it("reads 'light' from localStorage and applies data-theme='light'", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(false));
    storageMock.setItem("theme", "light");

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("reads 'dark' from localStorage and removes data-theme", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(true));
    // system prefers light but explicit localStorage wins
    storageMock.setItem("theme", "dark");

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  it("falls back to system preference (light) when localStorage is empty", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(true));

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("toggleTheme flips from dark to light, applies data-theme='light', persists to localStorage", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(false));

    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("dark");

    act(() => {
      result.current.toggleTheme();
    });

    expect(result.current.theme).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(storageMock.getItem("theme")).toBe("light");
  });

  it("toggleTheme flips from light to dark, removes data-theme, persists to localStorage", () => {
    vi.stubGlobal("matchMedia", makeMatchMedia(false));
    storageMock.setItem("theme", "light");

    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("light");

    act(() => {
      result.current.toggleTheme();
    });

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
    expect(storageMock.getItem("theme")).toBe("dark");
  });
});
