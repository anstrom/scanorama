export const STATUS_BG_COLORS = {
  up: "bg-success/15 text-success",
  healthy: "bg-success/15 text-success",
  completed: "bg-success/15 text-success",
  open: "bg-success/15 text-success",
  success: "bg-success/15 text-success",

  running: "bg-info/15 text-info",

  pending: "bg-warning/15 text-warning",
  degraded: "bg-warning/15 text-warning",

  down: "bg-danger/15 text-danger",
  failed: "bg-danger/15 text-danger",
  error: "bg-danger/15 text-danger",
  cancelled: "bg-danger/15 text-danger",
  stopped: "bg-danger/15 text-danger",

  unknown: "bg-text-muted/15 text-text-muted",
  filtered: "bg-text-muted/15 text-text-muted",
  closed: "bg-text-muted/15 text-text-muted",
  inactive: "bg-text-muted/15 text-text-muted",
} as const;

export type StatusKey = keyof typeof STATUS_BG_COLORS;
