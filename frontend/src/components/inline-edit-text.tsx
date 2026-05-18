import { useState, useRef, useEffect } from "react";
import { Pencil, Check, X } from "lucide-react";
import { cn } from "../lib/utils";

export interface InlineEditTextProps {
  value: string;
  placeholder?: string;
  multiline?: boolean;
  onSave: (newValue: string) => Promise<void>;
  disabled?: boolean;
  className?: string;
}

/**
 * InlineEditText renders a display value that can be clicked to enter an
 * inline edit mode. Pressing Enter (or Ctrl+Enter in multiline mode) saves;
 * Escape cancels. On save error the component stays in edit mode and reports
 * the error via a small message below the input.
 */
export function InlineEditText({
  value,
  placeholder = "—",
  multiline = false,
  onSave,
  disabled = false,
  className,
}: InlineEditTextProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [inputValue, setInputValue] = useState(value);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null);

  // Sync external value changes when not editing.
  useEffect(() => {
    if (!isEditing) {
      setInputValue(value);
    }
  }, [value, isEditing]);

  function startEditing() {
    if (disabled) return;
    setInputValue(value);
    setSaveError(null);
    setIsEditing(true);
  }

  function cancelEditing() {
    setIsEditing(false);
    setInputValue(value);
    setSaveError(null);
  }

  async function commitSave() {
    const trimmed = inputValue.trim();
    setSaveError(null);
    setIsSaving(true);
    try {
      await onSave(trimmed);
      setIsEditing(false);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Save failed.");
    } finally {
      setIsSaving(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      cancelEditing();
      return;
    }
    if (!multiline && e.key === "Enter") {
      e.preventDefault();
      void commitSave();
      return;
    }
    if (multiline && e.key === "Enter" && e.ctrlKey) {
      e.preventDefault();
      void commitSave();
    }
  }

  const sharedInputClass =
    "flex-1 px-2 py-0.5 text-xs rounded border border-border bg-surface text-text-primary focus:outline-none focus:ring-1 focus:ring-border min-w-0";

  if (isEditing) {
    return (
      <div className={cn("space-y-1", className)}>
        <div className="flex items-start gap-1.5">
          {multiline ? (
            <textarea
              ref={inputRef as React.RefObject<HTMLTextAreaElement>}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              onBlur={() => {
                if (!isSaving && !saveError) setIsEditing(false);
              }}
              autoFocus
              aria-label="Edit notes"
              rows={3}
              className={cn(sharedInputClass, "resize-y")}
            />
          ) : (
            <input
              ref={inputRef as React.RefObject<HTMLInputElement>}
              type="text"
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
              onBlur={() => {
                if (!isSaving && !saveError) setIsEditing(false);
              }}
              autoFocus
              aria-label="Edit value"
              className={sharedInputClass}
            />
          )}
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => void commitSave()}
            disabled={isSaving}
            aria-label="Save"
            className="p-0.5 rounded text-success hover:bg-surface-raised shrink-0"
          >
            <Check className="h-3 w-3" />
          </button>
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={cancelEditing}
            aria-label="Cancel"
            className="p-0.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised shrink-0"
          >
            <X className="h-3 w-3" />
          </button>
        </div>
        {saveError && (
          <p className="text-[11px] text-danger">{saveError}</p>
        )}
        {multiline && (
          <p className="text-[11px] text-text-muted">
            Ctrl+Enter to save · Esc to cancel
          </p>
        )}
      </div>
    );
  }

  const isEmpty = !value;
  return (
    <div className={cn("flex items-center gap-1.5 group min-w-0", className)}>
      <button
        type="button"
        onClick={startEditing}
        disabled={disabled}
        aria-label="Edit"
        className={cn(
          "text-left break-all min-w-0 flex-1",
          "text-xs",
          isEmpty
            ? "text-text-muted italic"
            : "text-text-secondary",
          !disabled && "hover:text-text-primary cursor-text",
          disabled && "cursor-default",
        )}
      >
        {isEmpty ? placeholder : value}
      </button>
      {!disabled && (
        <button
          type="button"
          onClick={startEditing}
          aria-hidden="true"
          tabIndex={-1}
          className="p-0.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
        >
          <Pencil className="h-2.5 w-2.5" />
        </button>
      )}
    </div>
  );
}
