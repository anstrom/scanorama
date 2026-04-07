import { useState, useEffect, useRef } from "react";
import { Settings2 } from "lucide-react";
import { cn } from "../lib/utils";

export interface ColumnDef {
  key: string;
  label: string;
  alwaysVisible?: boolean;
}

export interface ColumnToggleProps {
  columns: ColumnDef[];
  visibility: Record<string, boolean>;
  onToggle: (key: string) => void;
}

export function ColumnToggle({ columns, visibility, onToggle }: ColumnToggleProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handler(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        aria-label="Toggle columns"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className={cn(
          "flex items-center gap-1 px-2 py-1.5 rounded border border-border text-xs",
          "text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors",
          open && "bg-surface-raised text-text-primary",
        )}
      >
        <Settings2 className="h-3.5 w-3.5" />
        <span className="hidden sm:inline">Columns</span>
      </button>

      {open && (
        <div
          role="menu"
          className={cn(
            "absolute right-0 top-full mt-1 z-20",
            "min-w-40 bg-surface border border-border rounded-lg shadow-lg",
            "py-1",
          )}
        >
          {columns.map((col) => {
            const checked = visibility[col.key] ?? true;
            const disabled = col.alwaysVisible === true;
            return (
              <label
                key={col.key}
                role="menuitemcheckbox"
                aria-checked={checked}
                aria-disabled={disabled}
                className={cn(
                  "flex items-center gap-2 px-3 py-1.5 text-xs cursor-pointer",
                  disabled
                    ? "text-text-muted cursor-not-allowed opacity-50"
                    : "text-text-primary hover:bg-surface-raised",
                )}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={disabled}
                  onChange={() => !disabled && onToggle(col.key)}
                  className="h-3 w-3 rounded border-border accent-accent"
                />
                {col.label}
              </label>
            );
          })}
        </div>
      )}
    </div>
  );
}
