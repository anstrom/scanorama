import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { KeyboardEvent as ReactKeyboardEvent, FocusEvent as ReactFocusEvent } from "react";
import { useTableKeyNav } from "./use-table-key-nav";

// ── Helpers ───────────────────────────────────────────────────────────────────

/**
 * Creates a minimal mock React keyboard event.
 * tagName should be an uppercase HTML tag name (e.g. "DIV", "INPUT") to match
 * what browsers set on HTMLElement.tagName.
 */
function keyEvent(key: string, tagName = "DIV"): ReactKeyboardEvent {
  return {
    key,
    preventDefault: vi.fn(),
    target: { tagName },
  } as unknown as ReactKeyboardEvent;
}

/**
 * Creates a minimal mock React focus event.
 * When focusMovedOutside is true, currentTarget.contains() returns false,
 * which causes the blur handler to reset focusedIndex.
 */
function blurEvent(focusMovedOutside: boolean): ReactFocusEvent {
  return {
    relatedTarget: {},
    currentTarget: { contains: () => !focusMovedOutside },
  } as unknown as ReactFocusEvent;
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

const ITEMS = ["alpha", "beta", "gamma"];

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("useTableKeyNav", () => {
  let onActivate = vi.fn();
  let onSelect = vi.fn();
  let onEscape = vi.fn();

  beforeEach(() => {
    onActivate = vi.fn();
    onSelect = vi.fn();
    onEscape = vi.fn();
  });

  function setup(items = ITEMS) {
    return renderHook(() =>
      useTableKeyNav({ items, onActivate, onSelect, onEscape }),
    );
  }

  // ── Initial state ─────────────────────────────────────────────────────────

  it("starts with focusedIndex of -1 (nothing focused)", () => {
    const { result } = setup();
    expect(result.current.focusedIndex).toBe(-1);
  });

  it("containerProps always exposes tabIndex 0", () => {
    const { result } = setup();
    expect(result.current.containerProps.tabIndex).toBe(0);
  });

  // ── ArrowDown ─────────────────────────────────────────────────────────────

  it("ArrowDown increments focusedIndex from -1 to 0", () => {
    const { result } = setup();
    act(() => {
      result.current.containerProps.onKeyDown(keyEvent("ArrowDown"));
    });
    expect(result.current.focusedIndex).toBe(0);
  });

  it("ArrowDown increments focusedIndex on subsequent presses", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    expect(result.current.focusedIndex).toBe(1);
  });

  it("ArrowDown does not go past the last item", () => {
    const { result } = setup();
    // Navigate to the last item (index 2 in a 3-item list)
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 1
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 2
    expect(result.current.focusedIndex).toBe(2);

    // One more press — must clamp at the last index
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    expect(result.current.focusedIndex).toBe(2);
  });

  it("ArrowDown keeps focusedIndex at -1 when items list is empty", () => {
    const { result } = setup([]);
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    expect(result.current.focusedIndex).toBe(-1);
  });

  // ── ArrowUp ───────────────────────────────────────────────────────────────

  it("ArrowUp decrements focusedIndex correctly", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 1
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowUp")); });  // 0
    expect(result.current.focusedIndex).toBe(0);
  });

  it("ArrowUp does not decrement focusedIndex below 0", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowUp")); });  // stays 0
    expect(result.current.focusedIndex).toBe(0);

    // Another press — must remain clamped at 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowUp")); });
    expect(result.current.focusedIndex).toBe(0);
  });

  it("ArrowUp keeps focusedIndex at -1 when items list is empty", () => {
    const { result } = setup([]);
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowUp")); });
    expect(result.current.focusedIndex).toBe(-1);
  });

  // ── Enter ─────────────────────────────────────────────────────────────────

  it("Enter calls onActivate with the focused item and its index", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 1
    act(() => { result.current.containerProps.onKeyDown(keyEvent("Enter")); });
    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(onActivate).toHaveBeenCalledWith(ITEMS[1], 1);
  });

  it("Enter does nothing when focusedIndex is -1", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("Enter")); });
    expect(onActivate).not.toHaveBeenCalled();
  });

  // ── Space ─────────────────────────────────────────────────────────────────

  it("Space calls onSelect with the focused item and its index", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => { result.current.containerProps.onKeyDown(keyEvent(" ")); });
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(ITEMS[0], 0);
  });

  it("Space does nothing when focusedIndex is -1", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent(" ")); });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("Space does nothing when onSelect is not provided", () => {
    const { result } = renderHook(() =>
      useTableKeyNav({ items: ITEMS, onActivate }),
    );
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    // Must not throw when onSelect is absent
    act(() => { result.current.containerProps.onKeyDown(keyEvent(" ")); });
    expect(onActivate).not.toHaveBeenCalled();
  });

  // ── Escape ────────────────────────────────────────────────────────────────

  it("Escape calls onEscape and resets focusedIndex to -1", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    expect(result.current.focusedIndex).toBe(0);

    act(() => { result.current.containerProps.onKeyDown(keyEvent("Escape")); });
    expect(onEscape).toHaveBeenCalledTimes(1);
    expect(result.current.focusedIndex).toBe(-1);
  });

  it("Escape resets focusedIndex even when onEscape is not provided", () => {
    const { result } = renderHook(() =>
      useTableKeyNav({ items: ITEMS, onActivate }),
    );
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); });
    act(() => { result.current.containerProps.onKeyDown(keyEvent("Escape")); });
    expect(result.current.focusedIndex).toBe(-1);
  });

  // ── Interactive-element guard ─────────────────────────────────────────────

  it("ignores ArrowDown when the event target is an <input>", () => {
    const { result } = setup();
    act(() => {
      result.current.containerProps.onKeyDown(keyEvent("ArrowDown", "INPUT"));
    });
    expect(result.current.focusedIndex).toBe(-1);
  });

  it("ignores Enter when the event target is a <button>", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => {
      result.current.containerProps.onKeyDown(keyEvent("Enter", "BUTTON"));
    });
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("ignores Space when the event target is a <select>", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => {
      result.current.containerProps.onKeyDown(keyEvent(" ", "SELECT"));
    });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("ignores Escape when the event target is a <textarea>", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    act(() => {
      result.current.containerProps.onKeyDown(keyEvent("Escape", "TEXTAREA"));
    });
    expect(onEscape).not.toHaveBeenCalled();
    // focusedIndex was not reset because the key was ignored
    expect(result.current.focusedIndex).toBe(0);
  });

  // ── isFocused ─────────────────────────────────────────────────────────────

  it("isFocused returns true for the currently focused index", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    expect(result.current.isFocused(0)).toBe(true);
  });

  it("isFocused returns false for non-focused indices", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    expect(result.current.isFocused(1)).toBe(false);
    expect(result.current.isFocused(2)).toBe(false);
  });

  it("isFocused returns false for all indices when focusedIndex is -1", () => {
    const { result } = setup();
    ITEMS.forEach((_, i) => {
      expect(result.current.isFocused(i)).toBe(false);
    });
  });

  // ── setFocusedIndex ───────────────────────────────────────────────────────

  it("setFocusedIndex can be called directly to jump to a specific row", () => {
    const { result } = setup();
    act(() => { result.current.setFocusedIndex(2); });
    expect(result.current.focusedIndex).toBe(2);
    expect(result.current.isFocused(2)).toBe(true);
  });

  // ── Blur ──────────────────────────────────────────────────────────────────

  it("onBlur resets focusedIndex when focus moves outside the container", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    expect(result.current.focusedIndex).toBe(0);

    act(() => { result.current.containerProps.onBlur(blurEvent(true)); });
    expect(result.current.focusedIndex).toBe(-1);
  });

  it("onBlur keeps focusedIndex when focus moves to a child inside the container", () => {
    const { result } = setup();
    act(() => { result.current.containerProps.onKeyDown(keyEvent("ArrowDown")); }); // 0
    expect(result.current.focusedIndex).toBe(0);

    act(() => { result.current.containerProps.onBlur(blurEvent(false)); });
    expect(result.current.focusedIndex).toBe(0);
  });
});
