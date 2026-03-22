import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatRelativeTime(date: string | Date): string {
  const now = new Date();
  const then = new Date(date);
  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);

  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
  return then.toLocaleDateString();
}

export function formatAbsoluteTime(date: string | Date): string {
  return new Date(date).toLocaleString();
}

export function describeCron(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return expr;
  const [min, hr, dom, month, dow] = parts;

  const DAYS = [
    "Sunday",
    "Monday",
    "Tuesday",
    "Wednesday",
    "Thursday",
    "Friday",
    "Saturday",
  ];
  const allStar = (v: string) => v === "*";

  const timeStr = (h: string, m: string) =>
    `${h.padStart(2, "0")}:${m.padStart(2, "0")}`;

  if ([min, hr, dom, month, dow].every(allStar)) return "Every minute";
  if (allStar(min) && [hr, dom, month, dow].every(allStar))
    return "Every minute"; // same
  if (!allStar(hr) && allStar(min) && [dom, month, dow].every(allStar))
    return `Every day at ${timeStr(hr, "0")}`;
  if (!allStar(min) && !allStar(hr) && [dom, month, dow].every(allStar))
    return `Every day at ${timeStr(hr, min)}`;
  if (!allStar(min) && allStar(hr) && [dom, month, dow].every(allStar))
    return `Every hour at :${min.padStart(2, "0")}`;
  if (!allStar(dow) && /^\d$/.test(dow) && [dom, month].every(allStar)) {
    const day = DAYS[parseInt(dow)] ?? dow;
    if (!allStar(hr) && !allStar(min))
      return `Every ${day} at ${timeStr(hr, min)}`;
    if (!allStar(hr) && allStar(min))
      return `Every ${day} at ${timeStr(hr, "0")}`;
    return `Every ${day}`;
  }
  return expr;
}

/**
 * Validates an nmap-style port specification.
 * Returns an error message string, or null if the spec is valid.
 * Accepts: single ports (22), ranges (1-1024), comma-separated (22,80,443),
 * protocol-prefixed (T:22, U:53), and combinations.
 */
export function validatePortSpec(spec: string): string | null {
  const trimmed = spec.trim();
  if (!trimmed) return "Port specification is required.";

  // Strip optional leading protocol prefix for the whole spec (T: or U:)
  // Individual tokens can also have T:/U: prefixes
  const tokens = trimmed
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
  if (tokens.length === 0) return "Port specification is required.";

  for (const raw of tokens) {
    // Strip per-token protocol prefix
    const t = raw.replace(/^[TtUu]:/, "");

    // All-ports shorthand
    if (t === "-" || t === "*") continue;

    // Range: lo-hi
    const rangeMatch = t.match(/^(\d+)-(\d+)$/);
    if (rangeMatch) {
      const lo = parseInt(rangeMatch[1], 10);
      const hi = parseInt(rangeMatch[2], 10);
      if (lo < 1 || hi > 65535)
        return `Port range ${lo}-${hi}: values must be between 1 and 65535.`;
      if (lo > hi) return `Port range ${lo}-${hi}: start must not exceed end.`;
      continue;
    }

    // Single port
    const portMatch = t.match(/^\d+$/);
    if (portMatch) {
      const p = parseInt(t, 10);
      if (p < 1 || p > 65535) return `Port ${p}: must be between 1 and 65535.`;
      continue;
    }

    return `Invalid port token: "${raw}".`;
  }

  return null;
}

/**
 * Converts an unknown thrown value from an API call into a human-readable
 * error message. Handles both Error instances and raw API ErrorResponse objects.
 */
export function formatApiError(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (err && typeof err === "object") {
    const e = err as Record<string, unknown>;
    if (typeof e["message"] === "string" && e["message"]) return e["message"];
    if (typeof e["error"] === "string" && e["error"]) return e["error"];
  }
  return "An unexpected error occurred.";
}
