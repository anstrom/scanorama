import {
  useEffect,
  useRef,
  useState,
  useCallback,
  useMemo,
  useId,
} from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Computer,
  Globe,
  ScanLine,
  Settings,
  Clock,
  Search,
  X,
} from "lucide-react";
import { cn } from "../lib/utils";
import { useSearch, type SearchResultItem } from "../api/hooks/use-search";
import {
  useRecentPages,
  type RecentPage,
} from "../hooks/use-recent-pages";

// ── Types ──────────────────────────────────────────────────────────────────────

interface FlatResult {
  id: string;
  label: string;
  url: string;
  type: string;
}

interface GroupedSection {
  title: string;
  items: FlatResult[];
}

// ── Icons ──────────────────────────────────────────────────────────────────────

function ResultIcon({ type }: { type: string }) {
  const cls = "h-4 w-4 shrink-0 text-text-muted";
  switch (type) {
    case "host":
      return <Computer className={cls} />;
    case "network":
      return <Globe className={cls} />;
    case "scan":
      return <ScanLine className={cls} />;
    case "profile":
      return <Settings className={cls} />;
    case "recent":
      return <Clock className={cls} />;
    default:
      return <Search className={cls} />;
  }
}

// ── Helpers ────────────────────────────────────────────────────────────────────

const GROUP_TITLES: Record<string, string> = {
  hosts: "Hosts",
  networks: "Networks",
  scans: "Scans",
  profiles: "Profiles",
  recent: "Recent",
};

const GROUP_ORDER = ["hosts", "networks", "scans", "profiles"] as const;

function buildSections(
  query: string,
  searchResults: Record<string, SearchResultItem[]> | undefined,
  recentPages: RecentPage[],
): GroupedSection[] {
  if (!query || query.length < 2) {
    if (recentPages.length === 0) return [];
    // Map url → id so FlatResult keys are stable and unique.
    return [{ title: "Recent", items: recentPages.map((p) => ({ ...p, id: p.url })) }];
  }

  const sections: GroupedSection[] = [];
  for (const key of GROUP_ORDER) {
    const items = searchResults?.[key];
    if (items && items.length > 0) {
      sections.push({ title: GROUP_TITLES[key] ?? key, items });
    }
  }
  return sections;
}

function flattenSections(sections: GroupedSection[]): FlatResult[] {
  return sections.flatMap((s) => s.items);
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface ResultItemProps {
  item: FlatResult;
  isActive: boolean;
  onSelect: () => void;
  onHover: () => void;
  id: string;
}

function ResultItem({ item, isActive, onSelect, onHover, id }: ResultItemProps) {
  return (
    <li id={id} role="option" aria-selected={isActive}>
      <button
        type="button"
        className={cn(
          "flex items-center gap-3 w-full px-4 py-2.5 text-left rounded-md",
          "text-sm text-text-primary transition-colors",
          isActive
            ? "bg-primary/10 text-primary"
            : "hover:bg-surface-raised",
        )}
        onClick={onSelect}
        onMouseEnter={onHover}
      >
        <ResultIcon type={item.type} />
        <span className="flex-1 truncate">{item.label}</span>
      </button>
    </li>
  );
}

// ── Debounce ───────────────────────────────────────────────────────────────────

function useDebounce(value: string, ms: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return debounced;
}

// ── Main component ─────────────────────────────────────────────────────────────

export interface CommandPaletteProps {
  onClose: () => void;
}

const DEBOUNCE_MS = 300;

export function CommandPalette({ onClose }: CommandPaletteProps) {
  const dialogId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const navigate = useNavigate();

  const [inputValue, setInputValue] = useState("");
  const [activeIndex, setActiveIndex] = useState(-1);

  const { recentPages, addRecentPage } = useRecentPages();

  // Debounce the query so the API is called at most once per 300ms.
  const debouncedQuery = useDebounce(inputValue, DEBOUNCE_MS);
  const { data: searchData, isLoading } = useSearch(debouncedQuery);

  // Build the flat list used for keyboard navigation.
  const sections = useMemo(
    () => buildSections(inputValue, searchData?.results, recentPages),
    [inputValue, searchData, recentPages],
  );
  const flatItems = useMemo(() => flattenSections(sections), [sections]);

  // Focus the input on mount.
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Reset active index when the flat item list changes.
  useEffect(() => {
    setActiveIndex(-1);
  }, [debouncedQuery]);

  // Navigate to the selected item and close the palette.
  const selectItem = useCallback(
    (item: FlatResult) => {
      addRecentPage({ label: item.label, url: item.url, type: "recent" });
      void navigate({ to: item.url as "/" });
      onClose();
    },
    [addRecentPage, navigate, onClose],
  );

  // Keyboard handler for the list.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActiveIndex((prev) =>
            prev < flatItems.length - 1 ? prev + 1 : 0,
          );
          break;
        case "ArrowUp":
          e.preventDefault();
          setActiveIndex((prev) =>
            prev > 0 ? prev - 1 : flatItems.length - 1,
          );
          break;
        case "Enter":
          e.preventDefault();
          if (activeIndex >= 0 && flatItems[activeIndex]) {
            selectItem(flatItems[activeIndex]);
          }
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
        default:
          break;
      }
    },
    [flatItems, activeIndex, selectItem, onClose],
  );

  // Scroll the active item into view.
  useEffect(() => {
    if (activeIndex < 0 || !listRef.current) return;
    const el = listRef.current.querySelector(
      `[id="${dialogId}-item-${activeIndex}"]`,
    );
    if (el && typeof el.scrollIntoView === "function") {
      el.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex, dialogId]);

  // showEmpty: debounce has settled, API responded, and no results found.
  // Comparing inputValue === debouncedQuery prevents a flash of "no results"
  // during the 300ms debounce window before the query fires.
  const showEmpty =
    debouncedQuery.length >= 2 &&
    inputValue === debouncedQuery &&
    !isLoading &&
    flatItems.length === 0;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-50"
        onClick={onClose}
        aria-hidden="true"
        data-testid="palette-backdrop"
      />

      {/* Dialog */}
      <div
        role="combobox"
        aria-expanded="true"
        aria-haspopup="listbox"
        aria-controls={`${dialogId}-listbox`}
        aria-activedescendant={
          activeIndex >= 0 ? `${dialogId}-item-${activeIndex}` : undefined
        }
        className={cn(
          "fixed z-[60] inset-x-0 top-[10%] mx-auto",
          "w-full max-w-xl",
          "bg-surface border border-border rounded-lg shadow-2xl",
          "flex flex-col overflow-hidden",
        )}
      >
        {/* Search input row */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border">
          <Search className="h-4 w-4 shrink-0 text-text-muted" />
          <input
            ref={inputRef}
            type="text"
            role="searchbox"
            aria-label="Search"
            aria-autocomplete="list"
            className={cn(
              "flex-1 bg-transparent outline-none",
              "text-sm text-text-primary placeholder:text-text-muted",
            )}
            placeholder="Search hosts, networks, scans, profiles…"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          {isLoading && (
            <span className="text-xs text-text-muted animate-pulse">
              Searching…
            </span>
          )}
          <button
            type="button"
            onClick={onClose}
            aria-label="Close search"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Results */}
        <div className="overflow-y-auto max-h-96">
          {showEmpty ? (
            <p className="px-4 py-8 text-sm text-center text-text-muted">
              No results for &ldquo;{inputValue}&rdquo;
            </p>
          ) : (
            <ul
              ref={listRef}
              id={`${dialogId}-listbox`}
              role="listbox"
              aria-label="Search results"
              className="py-2"
            >
              {sections.map((section) => (
                <li key={section.title}>
                  <div className="px-4 pt-3 pb-1">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                      {section.title}
                    </span>
                  </div>
                  <ul>
                    {section.items.map((item) => {
                      const flatIdx = flatItems.indexOf(item);
                      return (
                        <ResultItem
                          key={`${item.type}-${item.id}`}
                          id={`${dialogId}-item-${flatIdx}`}
                          item={item}
                          isActive={flatIdx === activeIndex}
                          onSelect={() => selectItem(item)}
                          onHover={() => setActiveIndex(flatIdx)}
                        />
                      );
                    })}
                  </ul>
                </li>
              ))}
            </ul>
          )}

          {flatItems.length === 0 && !showEmpty && !isLoading && inputValue.length < 2 && (
            <p className="px-4 py-8 text-sm text-center text-text-muted">
              Type to search…
            </p>
          )}
        </div>

        {/* Footer hint */}
        <div className="px-4 py-2 border-t border-border flex items-center gap-4 text-[11px] text-text-muted">
          <span>
            <kbd className="font-mono">↑↓</kbd> navigate
          </span>
          <span>
            <kbd className="font-mono">↵</kbd> open
          </span>
          <span>
            <kbd className="font-mono">Esc</kbd> close
          </span>
        </div>
      </div>
    </>
  );
}
