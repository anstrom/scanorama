export const STATUS_COLORS = {
  up: "text-success",
  healthy: "text-success",
  completed: "text-success",
  open: "text-success",

  running: "text-info",
  pending: "text-warning",

  down: "text-danger",
  failed: "text-danger",
  error: "text-danger",

  unknown: "text-text-muted",
  filtered: "text-text-muted",
  closed: "text-text-muted",
  inactive: "text-text-muted",
} as const;

export const STATUS_BG_COLORS = {
  up: "bg-success/15 text-success",
  healthy: "bg-success/15 text-success",
  completed: "bg-success/15 text-success",

  running: "bg-info/15 text-info",
  pending: "bg-warning/15 text-warning",

  down: "bg-danger/15 text-danger",
  failed: "bg-danger/15 text-danger",

  unknown: "bg-text-muted/15 text-text-muted",
} as const;

export type StatusKey = keyof typeof STATUS_COLORS;
