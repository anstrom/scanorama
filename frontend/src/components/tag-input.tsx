import { useState, useRef, useCallback, useEffect } from "react";
import { X } from "lucide-react";
import { cn } from "../lib/utils";

interface TagInputProps {
  tags: string[];
  allTags?: string[];
  onChange: (tags: string[]) => void;
  disabled?: boolean;
  placeholder?: string;
}

export function TagInput({
  tags,
  allTags = [],
  onChange,
  disabled = false,
  placeholder = "Add tag…",
}: TagInputProps) {
  const [input, setInput] = useState("");
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const suggestions = allTags.filter(
    (t) =>
      t.toLowerCase().includes(input.toLowerCase()) &&
      !tags.includes(t),
  );

  // All selectable items in the dropdown: "create" option first, then suggestions.
  const dropdownItems: string[] = [
    ...(input.trim() !== "" && !tags.includes(input.trim().toLowerCase())
      ? [input.trim().toLowerCase()]
      : []),
    ...suggestions,
  ];

  const addTag = useCallback(
    (tag: string) => {
      const trimmed = tag.trim().toLowerCase();
      if (!trimmed || tags.includes(trimmed)) return;
      onChange([...tags, trimmed]);
      setInput("");
      setOpen(false);
      setActiveIndex(-1);
    },
    [tags, onChange],
  );

  const removeTag = useCallback(
    (tag: string) => {
      onChange(tags.filter((t) => t !== tag));
    },
    [tags, onChange],
  );

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!open) setOpen(true);
      setActiveIndex((i) => Math.min(i + 1, dropdownItems.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex((i) => Math.max(i - 1, -1));
    } else if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      if (activeIndex >= 0 && dropdownItems[activeIndex]) {
        addTag(dropdownItems[activeIndex]!);
      } else if (input.trim()) {
        addTag(input);
      }
    } else if (e.key === "Backspace" && input === "" && tags.length > 0) {
      removeTag(tags[tags.length - 1]!);
    } else if (e.key === "Escape") {
      setOpen(false);
      setActiveIndex(-1);
    }
  }

  // Close dropdown on outside click
  useEffect(() => {
    function handler(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  return (
    <div ref={containerRef} className="relative">
      <div
        className={cn(
          "flex flex-wrap gap-1 p-1.5 rounded border border-border bg-surface",
          "min-h-7 cursor-text",
          disabled && "opacity-60 pointer-events-none",
        )}
        onClick={() => inputRef.current?.focus()}
      >
        {tags.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full text-[11px] font-medium bg-accent/15 text-accent border border-accent/20"
          >
            {tag}
            {!disabled && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  removeTag(tag);
                }}
                aria-label={`Remove tag "${tag}"`}
                className="ml-0.5 rounded-full hover:bg-accent/20 transition-colors"
              >
                <X className="h-2.5 w-2.5" />
              </button>
            )}
          </span>
        ))}

        {!disabled && (
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={(e) => {
              setInput(e.target.value);
              setOpen(true);
              setActiveIndex(-1);
            }}
            onFocus={() => setOpen(true)}
            onKeyDown={handleKeyDown}
            placeholder={tags.length === 0 ? placeholder : ""}
            className="flex-1 min-w-16 bg-transparent text-xs text-text-primary placeholder:text-text-muted outline-none px-0.5"
          />
        )}
      </div>

      {open && dropdownItems.length > 0 && (
        <div
          role="listbox"
          className="absolute left-0 top-full mt-1 z-30 w-full bg-surface border border-border rounded-md shadow-lg py-1 max-h-40 overflow-y-auto"
        >
          {dropdownItems.map((item, idx) => {
            const isCreate = idx === 0 && input.trim() !== "" && !tags.includes(input.trim().toLowerCase()) && item === input.trim().toLowerCase();
            const isActive = idx === activeIndex;
            return (
              <button
                key={`${isCreate ? "create" : "suggest"}-${item}`}
                type="button"
                role="option"
                aria-selected={isActive}
                onMouseDown={(e) => {
                  e.preventDefault();
                  addTag(item);
                }}
                onMouseEnter={() => setActiveIndex(idx)}
                className={cn(
                  "w-full text-left px-3 py-1.5 text-xs",
                  isActive ? "bg-surface-raised" : "hover:bg-surface-raised",
                  isCreate ? "flex items-center gap-1.5" : "text-text-primary",
                )}
              >
                {isCreate ? (
                  <>
                    <span className="text-text-muted">Create</span>
                    <span className="font-medium text-accent">"{item}"</span>
                  </>
                ) : (
                  item
                )}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
