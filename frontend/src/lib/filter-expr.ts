// Filter expression types for the advanced filter builder.
// These mirror the FilterExpr types accepted by the backend's ?filter= query param.

// -- Core types ----------------------------------------------------------------

export type FilterCmp =
  | "is"
  | "is_not"
  | "contains"
  | "gt"
  | "lt"
  | "between";

export type FilterGroupOp = "AND" | "OR";

/** A leaf condition: field op value */
export interface FilterCondition {
  field: string;
  cmp: FilterCmp;
  value: string;
  value2?: string; // only for "between"
}

/** A group: AND/OR of sub-expressions */
export interface FilterGroup {
  op: FilterGroupOp;
  conditions: FilterExpr[];
}

export type FilterExpr = FilterCondition | FilterGroup;

export function isFilterGroup(expr: FilterExpr): expr is FilterGroup {
  return "op" in expr && "conditions" in expr;
}

export function isFilterCondition(expr: FilterExpr): expr is FilterCondition {
  return "field" in expr && "cmp" in expr;
}

// -- Field metadata -----------------------------------------------------------

export type FilterFieldType =
  | "enum"
  | "text"
  | "number"
  | "number_count"
  | "date"
  | "port"
  | "tag"
  | "group";

export interface FilterFieldMeta {
  field: string;
  label: string;
  type: FilterFieldType;
  values?: string[];
}

export const FILTER_FIELDS: FilterFieldMeta[] = [
  { field: "status", label: "Status", type: "enum", values: ["up", "down", "unknown", "gone"] },
  { field: "os_family", label: "OS Family", type: "text" },
  { field: "vendor", label: "Vendor", type: "text" },
  { field: "hostname", label: "Hostname", type: "text" },
  { field: "response_time_ms", label: "Response Time (ms)", type: "number" },
  { field: "first_seen", label: "First Seen", type: "date" },
  { field: "last_seen", label: "Last Seen", type: "date" },
  { field: "open_port", label: "Open Port", type: "port" },
  { field: "scan_count", label: "Scan Count", type: "number_count" },
  { field: "tags", label: "Tags", type: "tag" },
  { field: "group", label: "Group", type: "group" },
];

export function getFieldMeta(field: string): FilterFieldMeta | undefined {
  return FILTER_FIELDS.find((f) => f.field === field);
}

export function getOperatorsForType(type: FilterFieldType): FilterCmp[] {
  switch (type) {
    case "enum":   return ["is", "is_not"];
    case "text":   return ["is", "is_not", "contains"];
    case "number":
    case "number_count": return ["is", "is_not", "gt", "lt", "between"];
    case "date":   return ["gt", "lt", "between"];
    case "port":   return ["is", "is_not"];
    case "tag":    return ["contains", "is_not"];
    case "group":  return ["is", "is_not"];
  }
}

export const CMP_LABELS: Record<FilterCmp, string> = {
  is: "is",
  is_not: "is not",
  contains: "contains",
  gt: ">",
  lt: "<",
  between: "between",
};

export function defaultOperatorForType(type: FilterFieldType): FilterCmp {
  if (type === "date") return "gt";
  if (type === "tag") return "contains";
  return "is";
}

export function blankCondition(field: string): FilterCondition {
  const meta = getFieldMeta(field) ?? FILTER_FIELDS[0]!;
  return { field: meta.field, cmp: defaultOperatorForType(meta.type), value: "" };
}

// -- URL serialisation --------------------------------------------------------

export function serializeFilter(expr: FilterExpr | null): string | undefined {
  if (!expr) return undefined;
  try {
    return btoa(JSON.stringify(expr)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  } catch { return undefined; }
}

export function deserializeFilter(encoded: string | undefined): FilterExpr | null {
  if (!encoded) return null;
  try {
    return JSON.parse(atob(encoded.replace(/-/g, "+").replace(/_/g, "/"))) as FilterExpr;
  } catch { return null; }
}

// -- localStorage presets -----------------------------------------------------

export interface FilterPreset {
  id: string;
  name: string;
  expr: FilterExpr;
  createdAt: string;
}

const PRESETS_KEY = "scanorama:filter-presets:hosts";

export function loadFilterPresets(): FilterPreset[] {
  try {
    const raw = localStorage.getItem(PRESETS_KEY);
    return raw ? (JSON.parse(raw) as FilterPreset[]) : [];
  } catch { return []; }
}

export function saveFilterPreset(name: string, expr: FilterExpr): FilterPreset {
  const preset: FilterPreset = { id: crypto.randomUUID(), name: name.trim(), expr, createdAt: new Date().toISOString() };
  const all = loadFilterPresets();
  all.unshift(preset);
  localStorage.setItem(PRESETS_KEY, JSON.stringify(all.slice(0, 20)));
  return preset;
}

export function deleteFilterPreset(id: string): void {
  localStorage.setItem(PRESETS_KEY, JSON.stringify(loadFilterPresets().filter((p) => p.id !== id)));
}
