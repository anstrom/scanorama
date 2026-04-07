import { useState, useCallback } from "react";

export interface UseTableKeyNavOptions<T> {
  /** The current page of items shown in the table. */
  items: T[];
  /** Called when Enter is pressed on a focused row (open detail panel). */
  onActivate: (item: T, index: number) => void;
  /** Called when Space is pressed on a focused row (optional — e.g. toggle checkbox). */
  onSelect?: (item: T, index: number) => void;
  /** Called when Escape is pressed (e.g. close detail panel). */
  onEscape?: () => void;
}

export interface UseTableKeyNavResult {
  /** Index of the currently keyboard-focused row (-1 = none). */
  focusedIndex: number;
  setFocusedIndex: (index: number) => void;
  /** Spread onto the keyboard-navigable container. */
  containerProps: {
    tabIndex: 0;
    onKeyDown: (e: React.KeyboardEvent) => void;
    onBlur: (e: React.FocusEvent) => void;
  };
  /** Returns true if the given row index is currently focused. */
  isFocused: (index: number) => boolean;
}

export function useTableKeyNav<T>({
  items,
  onActivate,
  onSelect,
  onEscape,
}: UseTableKeyNavOptions<T>): UseTableKeyNavResult {
  const [focusedIndex, setFocusedIndex] = useState(-1);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      // Ignore if the event originated inside an interactive element
      // (input, select, button, textarea) to avoid stealing keystrokes.
      const tag = (e.target as HTMLElement).tagName.toLowerCase();
      if (["input", "select", "button", "textarea"].includes(tag)) return;

      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setFocusedIndex((prev) =>
            items.length === 0 ? -1 : Math.min(prev + 1, items.length - 1),
          );
          break;
        case "ArrowUp":
          e.preventDefault();
          setFocusedIndex((prev) =>
            items.length === 0 ? -1 : Math.max(prev - 1, 0),
          );
          break;
        case "Enter":
          if (focusedIndex >= 0 && focusedIndex < items.length) {
            e.preventDefault();
            onActivate(items[focusedIndex], focusedIndex);
          }
          break;
        case " ":
          if (focusedIndex >= 0 && focusedIndex < items.length && onSelect) {
            e.preventDefault();
            onSelect(items[focusedIndex], focusedIndex);
          }
          break;
        case "Escape":
          e.preventDefault();
          setFocusedIndex(-1);
          onEscape?.();
          break;
      }
    },
    [focusedIndex, items, onActivate, onSelect, onEscape],
  );

  // Clear focus when the container loses focus entirely (not just to a child)
  const handleBlur = useCallback((e: React.FocusEvent) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node)) {
      setFocusedIndex(-1);
    }
  }, []);

  return {
    focusedIndex,
    setFocusedIndex,
    containerProps: {
      tabIndex: 0,
      onKeyDown: handleKeyDown,
      onBlur: handleBlur,
    },
    isFocused: (index: number) => index === focusedIndex,
  };
}
