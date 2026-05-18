import { useState, useCallback } from "react";

const STORAGE_KEY = "scanorama_recent_pages";
const MAX_RECENT = 5;

export interface RecentPage {
  label: string;
  url: string;
  type: "recent";
}

function readFromStorage(): RecentPage[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    return JSON.parse(raw) as RecentPage[];
  } catch {
    return [];
  }
}

function writeToStorage(pages: RecentPage[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(pages));
  } catch {
    // localStorage may be unavailable in some environments — silently ignore.
  }
}

/**
 * Stores and retrieves recent page visits from localStorage.
 * Key: "scanorama_recent_pages", max 5 entries, newest first.
 */
export function useRecentPages() {
  const [recentPages, setRecentPages] = useState<RecentPage[]>(readFromStorage);

  const addRecentPage = useCallback((page: RecentPage) => {
    setRecentPages((prev) => {
      // Deduplicate by URL — remove existing entry if present.
      const filtered = prev.filter((p) => p.url !== page.url);
      const updated = [page, ...filtered].slice(0, MAX_RECENT);
      writeToStorage(updated);
      return updated;
    });
  }, []);

  return { recentPages, addRecentPage };
}
