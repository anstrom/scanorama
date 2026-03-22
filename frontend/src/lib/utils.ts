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
