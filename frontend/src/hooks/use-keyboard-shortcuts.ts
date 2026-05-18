import { useEffect, useCallback, useRef, useState } from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";

/** Duration in ms to wait for a second key after `g` before cancelling. */
export const G_TIMEOUT_MS = 1000;

/** Returns true when focus is inside an interactive text input element. */
function isInTextInput(): boolean {
  const tag = document.activeElement?.tagName?.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

export interface UseKeyboardShortcutsResult {
  showHelp: boolean;
  setShowHelp: (v: boolean) => void;
}

/**
 * Sets up the global keyboard shortcut listener.
 *
 * Bindings:
 *   g h  → /hosts
 *   g s  → /scans
 *   g n  → /networks
 *   g d  → / (dashboard)
 *   g a  → /admin
 *   n    → navigate to /scans or emit new-scan-requested if already there
 *   ?    → toggle help overlay
 *
 * Shortcuts are suppressed while focus is inside an <input>, <textarea>,
 * or <select>. The `g` prefix has a 1-second timeout — if no second key
 * arrives within 1s the pending `g` is cancelled.
 */
export function useKeyboardShortcuts(): UseKeyboardShortcutsResult {
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const [showHelp, setShowHelp] = useState(false);
  const pendingG = useRef(false);
  const gTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cancelPendingG = useCallback(() => {
    if (gTimer.current !== null) {
      clearTimeout(gTimer.current);
      gTimer.current = null;
    }
    pendingG.current = false;
  }, []);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Cmd+K (macOS) or Ctrl+K (Windows/Linux) opens the command palette.
      if ((e.metaKey || e.ctrlKey) && e.key === "k" && !e.altKey) {
        e.preventDefault();
        document.dispatchEvent(new CustomEvent("search-requested"));
        return;
      }

      // Ignore other modifier-key combos (Ctrl, Alt, Meta) — let the browser handle them.
      if (e.ctrlKey || e.altKey || e.metaKey) return;

      // Suppress shortcuts when typing in an input field.
      if (isInTextInput()) {
        cancelPendingG();
        return;
      }

      const key = e.key;

      // ── Handle pending `g` prefix ──────────────────────────────────────────
      if (pendingG.current) {
        cancelPendingG();

        switch (key) {
          case "h":
            e.preventDefault();
            void navigate({ to: "/hosts" });
            return;
          case "s":
            e.preventDefault();
            void navigate({ to: "/scans" });
            return;
          case "n":
            e.preventDefault();
            void navigate({ to: "/networks" });
            return;
          case "d":
            e.preventDefault();
            void navigate({ to: "/" });
            return;
          case "a":
            e.preventDefault();
            void navigate({ to: "/admin" });
            return;
          default:
            // Unrecognised second key — just drop the pending `g`.
            return;
        }
      }

      // ── Top-level bindings ─────────────────────────────────────────────────
      switch (key) {
        case "g":
          e.preventDefault();
          pendingG.current = true;
          gTimer.current = setTimeout(cancelPendingG, G_TIMEOUT_MS);
          return;

        case "n":
          e.preventDefault();
          if (pathname === "/scans") {
            document.dispatchEvent(new CustomEvent("new-scan-requested"));
          } else {
            void navigate({ to: "/scans" });
          }
          return;

        case "?":
          e.preventDefault();
          setShowHelp((prev) => !prev);
          return;

        case "Escape":
          if (showHelp) {
            e.preventDefault();
            setShowHelp(false);
          }
          return;
      }
    },
    [navigate, pathname, showHelp, cancelPendingG],
  );

  useEffect(() => {
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      cancelPendingG();
    };
  }, [handleKeyDown, cancelPendingG]);

  return { showHelp, setShowHelp };
}
