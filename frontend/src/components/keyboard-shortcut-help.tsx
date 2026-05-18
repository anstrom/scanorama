import { useEffect, useId } from "react";
import { X } from "lucide-react";
import { cn } from "../lib/utils";

interface Binding {
  keys: string[];
  description: string;
}

const NAV_BINDINGS: Binding[] = [
  { keys: ["g", "h"], description: "Go to Hosts" },
  { keys: ["g", "s"], description: "Go to Scans" },
  { keys: ["g", "n"], description: "Go to Networks" },
  { keys: ["g", "d"], description: "Go to Dashboard" },
  { keys: ["g", "a"], description: "Go to Admin" },
];

const ACTION_BINDINGS: Binding[] = [
  { keys: ["n"], description: "New scan" },
  { keys: ["?"], description: "Toggle this help" },
  { keys: ["Esc"], description: "Close overlay" },
];

function KeyBadge({ label }: { label: string }) {
  return (
    <kbd
      className={cn(
        "inline-flex items-center justify-center",
        "min-w-[1.5rem] px-1.5 h-6",
        "rounded border border-border bg-surface-raised",
        "font-mono text-[11px] text-text-primary",
        "shadow-sm",
      )}
    >
      {label}
    </kbd>
  );
}

function BindingRow({ binding }: { binding: Binding }) {
  return (
    <li className="flex items-center gap-3">
      <span className="flex items-center gap-1 shrink-0">
        {binding.keys.map((k, i) => (
          <KeyBadge key={i} label={k} />
        ))}
      </span>
      <span className="text-xs text-text-secondary">{binding.description}</span>
    </li>
  );
}

function BindingSection({
  title,
  bindings,
}: {
  title: string;
  bindings: Binding[];
}) {
  return (
    <div className="space-y-2.5">
      <h3 className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
        {title}
      </h3>
      <ul className="space-y-2">
        {bindings.map((b, i) => (
          <BindingRow key={i} binding={b} />
        ))}
      </ul>
    </div>
  );
}

export interface KeyboardShortcutHelpProps {
  onClose: () => void;
}

export function KeyboardShortcutHelp({ onClose }: KeyboardShortcutHelpProps) {
  const id = useId();

  // Dismiss on Escape is handled by the global shortcut hook, but we also
  // handle it here so the overlay is self-contained when rendered standalone.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        className={cn(
          "fixed z-50 inset-0 m-auto",
          "w-full max-w-sm h-fit",
          "bg-surface border border-border rounded-lg shadow-xl",
          "flex flex-col",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border">
          <h2
            id={`${id}-title`}
            className="text-sm font-semibold text-text-primary"
          >
            Keyboard shortcuts
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body — two columns */}
        <div className="px-5 py-4 grid grid-cols-2 gap-6">
          <BindingSection title="Navigation" bindings={NAV_BINDINGS} />
          <BindingSection title="Actions" bindings={ACTION_BINDINGS} />
        </div>
      </div>
    </>
  );
}
