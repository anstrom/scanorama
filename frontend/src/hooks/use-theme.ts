import { useState, useEffect } from "react";

export type Theme = "dark" | "light";

const STORAGE_KEY = "theme";
const LIGHT_QUERY = "(prefers-color-scheme: light)";
const DATA_ATTR = "light";

function applyTheme(theme: Theme): void {
  if (theme === "light") {
    document.documentElement.setAttribute("data-theme", DATA_ATTR);
  } else {
    document.documentElement.removeAttribute("data-theme");
  }
}

function getInitialTheme(): Theme {
  const stored = globalThis.localStorage?.getItem(STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia(LIGHT_QUERY).matches ? "light" : "dark";
}

export interface UseThemeResult {
  theme: Theme;
  toggleTheme: () => void;
}

export function useTheme(): UseThemeResult {
  const [theme, setTheme] = useState<Theme>(() => getInitialTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  function toggleTheme(): void {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    globalThis.localStorage?.setItem(STORAGE_KEY, next);
  }

  return { theme, toggleTheme };
}
