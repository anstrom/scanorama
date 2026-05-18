import { useState, useRef, useEffect } from "react";
import { Download, ChevronDown } from "lucide-react";
import { cn } from "../lib/utils";

export interface ExportButtonProps {
  /** API path for the export endpoint, e.g. "/api/v1/hosts/export". */
  basePath: string;
  /** Current filter/sort params to append to the download URL. */
  params: Record<string, string | number | undefined>;
  /** Button label (default: "Export"). */
  label?: string;
}

type ExportFormat = "csv" | "json";

/** Builds the full download URL from a base path and a param map. */
function buildExportURL(
  basePath: string,
  params: Record<string, string | number | undefined>,
  format: ExportFormat,
): string {
  const qs = new URLSearchParams();
  qs.set("format", format);
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "" && value !== null) {
      qs.set(key, String(value));
    }
  }
  return `${basePath}?${qs.toString()}`;
}

/**
 * ExportButton renders a split-button that downloads the table data as CSV or
 * JSON. Clicking the primary area downloads CSV; the chevron opens a small menu
 * so the user can choose JSON instead.
 *
 * The download is triggered by navigating window.location.href so the browser
 * handles the file-save dialog natively — no fetch + Blob needed.
 */
export function ExportButton({
  basePath,
  params,
  label = "Export",
}: ExportButtonProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close the dropdown when the user clicks outside.
  useEffect(() => {
    if (!open) return;
    function handleOutside(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleOutside);
    return () => document.removeEventListener("mousedown", handleOutside);
  }, [open]);

  function triggerDownload(format: ExportFormat) {
    const url = buildExportURL(basePath, params, format);
    window.location.href = url;
    setOpen(false);
  }

  const baseBtn = cn(
    "inline-flex items-center justify-center rounded transition-colors",
    "border border-border text-text-secondary",
    "hover:text-text-primary hover:bg-surface-raised",
    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
    "px-3 py-1.5 text-xs gap-1.5",
    "disabled:cursor-not-allowed",
  );

  return (
    <div ref={containerRef} className="relative inline-flex">
      {/* Primary: Export CSV */}
      <button
        type="button"
        onClick={() => triggerDownload("csv")}
        className={cn(baseBtn, "rounded-r-none border-r-0")}
        aria-label={`${label} CSV`}
      >
        <Download className="h-3.5 w-3.5 shrink-0" />
        {label}
      </button>

      {/* Chevron: opens format picker */}
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={cn(baseBtn, "rounded-l-none px-1.5")}
        aria-label="Export format picker"
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <ChevronDown className="h-3 w-3 shrink-0" />
      </button>

      {/* Dropdown menu */}
      {open && (
        <div
          role="menu"
          className={cn(
            "absolute right-0 top-full mt-1 z-30",
            "w-36 bg-surface border border-border rounded-md shadow-lg py-1",
          )}
        >
          <button
            role="menuitem"
            type="button"
            onClick={() => triggerDownload("csv")}
            className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-raised"
          >
            Export CSV
          </button>
          <button
            role="menuitem"
            type="button"
            onClick={() => triggerDownload("json")}
            className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-raised"
          >
            Export JSON
          </button>
        </div>
      )}
    </div>
  );
}
