import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useKeyboardShortcuts, G_TIMEOUT_MS } from "./use-keyboard-shortcuts";

// ── Mock TanStack Router ───────────────────────────────────────────────────────

const mockNavigate = vi.fn();
let mockPathname = "/";

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: mockPathname } }),
}));

// ── Helpers ────────────────────────────────────────────────────────────────────

function fireKey(key: string, options: Partial<KeyboardEventInit> = {}) {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, ...options });
  document.dispatchEvent(event);
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("useKeyboardShortcuts", () => {
  beforeEach(() => {
    mockNavigate.mockClear();
    mockPathname = "/";
    // Ensure no element is focused inside an input
    (document.activeElement as HTMLElement | null)?.blur?.();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("navigates to /hosts on g h", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("h");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/hosts" });
  });

  it("navigates to /scans on g s", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("s");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/scans" });
  });

  it("navigates to /networks on g n", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("n");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/networks" });
  });

  it("navigates to / on g d", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("d");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/" });
  });

  it("navigates to /admin on g a", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("a");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/admin" });
  });

  it("does not navigate when g is pressed without a follow-up key", () => {
    vi.useFakeTimers();
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
    });

    act(() => {
      vi.advanceTimersByTime(G_TIMEOUT_MS + 100);
    });

    expect(mockNavigate).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it("cancels pending g on timeout so subsequent g h works fresh", () => {
    vi.useFakeTimers();
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
    });
    // Let the timeout expire
    act(() => {
      vi.advanceTimersByTime(G_TIMEOUT_MS + 100);
    });
    // Now press g h — should navigate
    act(() => {
      fireKey("g");
      fireKey("h");
    });

    expect(mockNavigate).toHaveBeenCalledTimes(1);
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/hosts" });
    vi.useRealTimers();
  });

  it("ignores unrecognised second key after g and does not navigate", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("z"); // not a valid binding
    });

    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("toggles showHelp on ?", () => {
    const { result } = renderHook(() => useKeyboardShortcuts());

    expect(result.current.showHelp).toBe(false);

    act(() => {
      fireKey("?");
    });

    expect(result.current.showHelp).toBe(true);

    act(() => {
      fireKey("?");
    });

    expect(result.current.showHelp).toBe(false);
  });

  it("closes help overlay on Escape when it is open", () => {
    const { result } = renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("?");
    });
    expect(result.current.showHelp).toBe(true);

    act(() => {
      fireKey("Escape");
    });
    expect(result.current.showHelp).toBe(false);
  });

  it("does not fire shortcuts when focus is inside an input", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("g");
      fireKey("h");
    });

    expect(mockNavigate).not.toHaveBeenCalled();

    document.body.removeChild(input);
  });

  it("does not fire shortcuts when focus is inside a textarea", () => {
    const textarea = document.createElement("textarea");
    document.body.appendChild(textarea);

    const { result } = renderHook(() => useKeyboardShortcuts());

    // Initially showHelp should be false
    expect(result.current.showHelp).toBe(false);

    // Focus textarea and press ?
    textarea.focus();
    act(() => {
      fireKey("?");
    });

    // showHelp should remain false since focus was in textarea
    expect(result.current.showHelp).toBe(false);

    // Unfocus textarea and press ? again
    textarea.blur();
    act(() => {
      fireKey("?");
    });

    // Now showHelp should toggle to true, proving suppression was the reason it stayed false
    expect(result.current.showHelp).toBe(true);

    document.body.removeChild(textarea);
  });

  it("navigates to /scans on n when not on /scans", () => {
    mockPathname = "/hosts";
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("n");
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/scans" });
  });

  it("dispatches new-scan-requested event on n when already on /scans", () => {
    mockPathname = "/scans";
    const listener = vi.fn();
    document.addEventListener("new-scan-requested", listener);

    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("n");
    });

    expect(listener).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();

    document.removeEventListener("new-scan-requested", listener);
  });

  it("ignores modifier-key combos (other than Cmd/Ctrl+K)", () => {
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("h", { ctrlKey: true });
    });

    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("dispatches search-requested on Cmd+K (metaKey)", () => {
    const listener = vi.fn();
    document.addEventListener("search-requested", listener);

    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("k", { metaKey: true });
    });

    expect(listener).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();

    document.removeEventListener("search-requested", listener);
  });

  it("dispatches search-requested on Ctrl+K", () => {
    const listener = vi.fn();
    document.addEventListener("search-requested", listener);

    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireKey("k", { ctrlKey: true });
    });

    expect(listener).toHaveBeenCalledTimes(1);

    document.removeEventListener("search-requested", listener);
  });
});
