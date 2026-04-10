import { useState, useCallback, useEffect, useRef } from "react";
import {
  X,
  Plus,
  ChevronDown,
  Trash2,
  Filter,
  Save,
  BookmarkCheck,
} from "lucide-react";
import { cn } from "../lib/utils";
import { Button } from "./button";
import {
  FILTER_FIELDS,
  CMP_LABELS,
  getFieldMeta,
  getOperatorsForType,
  defaultOperatorForType,
  blankCondition,
  serializeFilter,
  loadFilterPresets,
  saveFilterPreset,
  deleteFilterPreset,
} from "../lib/filter-expr";
import type {
  FilterExpr,
  FilterGroup,
  FilterCondition,
  FilterCmp,
} from "../lib/filter-expr";

// ── Helper to check if an expression is a group ───────────────────────────────

function isGroup(expr: FilterExpr): expr is FilterGroup {
  return "op" in expr && "conditions" in expr;
}

// ── Value input ───────────────────────────────────────────────────────────────

interface GroupOption {
  id: string;
  name: string;
}

interface ValueInputProps {
  condition: FilterCondition;
  onChange: (updates: Partial<FilterCondition>) => void;
  tagSuggestions?: string[];
  groupOptions?: GroupOption[];
}

function ValueInput({ condition, onChange, tagSuggestions = [], groupOptions = [] }: ValueInputProps) {
  const meta = getFieldMeta(condition.field);
  const type = meta?.type ?? "text";
  const isBetween = condition.cmp === "between";
  const [tagDropOpen, setTagDropOpen] = useState(false);
  const tagRef = useRef<HTMLDivElement>(null);

  // Close tag dropdown on outside click
  useEffect(() => {
    if (!tagDropOpen) return;
    function handler(e: MouseEvent) {
      if (tagRef.current && !tagRef.current.contains(e.target as Node)) {
        setTagDropOpen(false);
      }
    }
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [tagDropOpen]);

  const inputClass = cn(
    "px-2 py-1 text-xs rounded border border-border",
    "bg-surface text-text-primary placeholder:text-text-muted",
    "focus:outline-none focus:ring-1 focus:ring-border",
  );

  if (type === "enum") {
    return (
      <select
        value={condition.value}
        onChange={(e) => onChange({ value: e.target.value })}
        className={cn(inputClass, "min-w-24")}
        aria-label="Filter value"
      >
        <option value="">Select…</option>
        {(meta?.values ?? []).map((v) => (
          <option key={v} value={v}>
            {v}
          </option>
        ))}
      </select>
    );
  }

  if (type === "date") {
    return (
      <div className="flex items-center gap-1">
        <input
          type="date"
          value={condition.value}
          onChange={(e) => onChange({ value: e.target.value })}
          className={inputClass}
          aria-label={isBetween ? "From date" : "Filter value"}
        />
        {isBetween && (
          <>
            <span className="text-text-muted text-xs">and</span>
            <input
              type="date"
              value={condition.value2 ?? ""}
              onChange={(e) => onChange({ value2: e.target.value })}
              className={inputClass}
              aria-label="To date"
            />
          </>
        )}
      </div>
    );
  }

  if (type === "number" || type === "number_count" || type === "port") {
    return (
      <div className="flex items-center gap-1">
        <input
          type="number"
          value={condition.value}
          onChange={(e) => onChange({ value: e.target.value })}
          min={type === "port" ? 1 : 0}
          max={type === "port" ? 65535 : undefined}
          placeholder={type === "port" ? "e.g. 80" : ""}
          className={cn(inputClass, "w-24")}
          aria-label={isBetween ? "Min value" : "Filter value"}
        />
        {isBetween && (
          <>
            <span className="text-text-muted text-xs">and</span>
            <input
              type="number"
              value={condition.value2 ?? ""}
              onChange={(e) => onChange({ value2: e.target.value })}
              min={0}
              className={cn(inputClass, "w-24")}
              aria-label="Max value"
            />
          </>
        )}
      </div>
    );
  }

  if (type === "group") {
    return (
      <select
        value={condition.value}
        onChange={(e) => onChange({ value: e.target.value })}
        className={cn(inputClass, "min-w-36")}
        aria-label="Filter value"
      >
        <option value="">Select group…</option>
        {groupOptions.map((g) => (
          <option key={g.id} value={g.name}>
            {g.name}
          </option>
        ))}
      </select>
    );
  }

  if (type === "tag") {
    const filtered = tagSuggestions.filter(
      (t) =>
        condition.value === "" ||
        t.toLowerCase().includes(condition.value.toLowerCase()),
    );
    return (
      <div ref={tagRef} className="relative">
        <input
          type="text"
          value={condition.value}
          onChange={(e) => {
            onChange({ value: e.target.value });
            setTagDropOpen(true);
          }}
          onFocus={() => setTagDropOpen(true)}
          onKeyDown={(e) => {
            if (e.key === "Escape") setTagDropOpen(false);
          }}
          placeholder="tag name…"
          className={cn(inputClass, "min-w-32")}
          aria-label="Tag value"
        />
        {tagDropOpen && filtered.length > 0 && (
          <div className="absolute left-0 top-full mt-1 z-30 w-48 bg-surface border border-border rounded-md shadow-lg py-1 max-h-40 overflow-y-auto">
            {filtered.map((t) => (
              <button
                key={t}
                type="button"
                onMouseDown={(e) => {
                  e.preventDefault();
                  onChange({ value: t });
                  setTagDropOpen(false);
                }}
                className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-raised text-text-primary"
              >
                {t}
              </button>
            ))}
          </div>
        )}
      </div>
    );
  }

  // text
  return (
    <input
      type="text"
      value={condition.value}
      onChange={(e) => onChange({ value: e.target.value })}
      placeholder="value…"
      className={cn(inputClass, "min-w-32")}
      aria-label="Filter value"
    />
  );
}

// ── Single condition row ──────────────────────────────────────────────────────

interface ConditionRowProps {
  condition: FilterCondition;
  onChange: (c: FilterCondition) => void;
  onRemove: () => void;
  isOnly: boolean;
  tagSuggestions?: string[];
  groupOptions?: GroupOption[];
}

function ConditionRow({
  condition,
  onChange,
  onRemove,
  isOnly,
  tagSuggestions,
  groupOptions,
}: ConditionRowProps) {
  const meta = getFieldMeta(condition.field);
  const ops = getOperatorsForType(meta?.type ?? "text");

  const selectClass = cn(
    "px-2 py-1 text-xs rounded border border-border",
    "bg-surface text-text-primary",
    "focus:outline-none focus:ring-1 focus:ring-border",
  );

  function handleFieldChange(field: string) {
    const newMeta = getFieldMeta(field);
    const newType = newMeta?.type ?? "text";
    const newOps = getOperatorsForType(newType);
    const cmp = newOps.includes(condition.cmp)
      ? condition.cmp
      : defaultOperatorForType(newType);
    const value = newType === "enum" ? (newMeta?.values?.[0] ?? "") : "";
    onChange({ field, cmp, value, value2: undefined });
  }

  function handleCmpChange(cmp: FilterCmp) {
    onChange({ ...condition, cmp, value2: undefined });
  }

  function handleValueChange(updates: Partial<FilterCondition>) {
    onChange({ ...condition, ...updates });
  }

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {/* Field selector */}
      <select
        value={condition.field}
        onChange={(e) => handleFieldChange(e.target.value)}
        className={cn(selectClass, "min-w-36")}
        aria-label="Filter field"
      >
        {FILTER_FIELDS.map((f) => (
          <option key={f.field} value={f.field}>
            {f.label}
          </option>
        ))}
      </select>

      {/* Operator selector */}
      <select
        value={condition.cmp}
        onChange={(e) => handleCmpChange(e.target.value as FilterCmp)}
        className={cn(selectClass, "min-w-24")}
        aria-label="Filter operator"
      >
        {ops.map((op) => (
          <option key={op} value={op}>
            {CMP_LABELS[op]}
          </option>
        ))}
      </select>

      {/* Value input */}
      <ValueInput condition={condition} onChange={handleValueChange} tagSuggestions={tagSuggestions} groupOptions={groupOptions} />

      {/* Remove button */}
      <button
        type="button"
        onClick={onRemove}
        disabled={isOnly}
        aria-label="Remove condition"
        className={cn(
          "p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors",
          isOnly && "opacity-30 cursor-not-allowed",
        )}
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}

// ── Group operator toggle ─────────────────────────────────────────────────────

interface OpToggleProps {
  op: "AND" | "OR";
  onChange: (op: "AND" | "OR") => void;
}

function OpToggle({ op, onChange }: OpToggleProps) {
  return (
    <div className="flex items-center gap-0.5 rounded border border-border overflow-hidden text-xs">
      {(["AND", "OR"] as const).map((o) => (
        <button
          key={o}
          type="button"
          onClick={() => onChange(o)}
          className={cn(
            "px-2 py-0.5 transition-colors",
            op === o
              ? "bg-accent text-white font-medium"
              : "bg-surface text-text-muted hover:text-text-primary hover:bg-surface-raised",
          )}
        >
          {o}
        </button>
      ))}
    </div>
  );
}

// ── Sub-group (one level of nesting) ─────────────────────────────────────────

interface SubGroupProps {
  group: FilterGroup;
  onChange: (g: FilterGroup) => void;
  onRemove: () => void;
  tagSuggestions?: string[];
  groupOptions?: GroupOption[];
}

function SubGroup({ group, onChange, onRemove, tagSuggestions, groupOptions }: SubGroupProps) {
  function updateCondition(idx: number, cond: FilterCondition) {
    const conditions = group.conditions.map((c, i) => (i === idx ? cond : c));
    onChange({ ...group, conditions });
  }

  function removeCondition(idx: number) {
    if (group.conditions.length <= 1) return;
    const conditions = group.conditions.filter((_, i) => i !== idx);
    onChange({ ...group, conditions });
  }

  function addCondition() {
    onChange({
      ...group,
      conditions: [
        ...group.conditions,
        blankCondition(FILTER_FIELDS[0]!.field),
      ],
    });
  }

  return (
    <div className="pl-4 border-l-2 border-accent/40 space-y-2 mt-1 mb-1">
      <div className="flex items-center gap-2">
        <OpToggle op={group.op} onChange={(op) => onChange({ ...group, op })} />
        <span className="text-xs text-text-muted">group</span>
        <button
          type="button"
          onClick={onRemove}
          aria-label="Remove group"
          className="ml-auto p-1 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
        >
          <Trash2 className="h-3 w-3" />
        </button>
      </div>

      {group.conditions.map((cond, idx) => {
        if (isGroup(cond)) return null; // no further nesting
        return (
          <ConditionRow
            key={idx}
            condition={cond as FilterCondition}
            onChange={(c) => updateCondition(idx, c)}
            onRemove={() => removeCondition(idx)}
            isOnly={group.conditions.length === 1}
            tagSuggestions={tagSuggestions}
            groupOptions={groupOptions}
          />
        );
      })}

      <button
        type="button"
        onClick={addCondition}
        className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary transition-colors"
      >
        <Plus className="h-3 w-3" />
        Add condition
      </button>
    </div>
  );
}

// ── Presets dropdown ──────────────────────────────────────────────────────────

interface PresetsDropdownProps {
  onLoad: (group: FilterGroup) => void;
  currentExpr: FilterGroup | null;
}

function PresetsDropdown({ onLoad, currentExpr }: PresetsDropdownProps) {
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [presetName, setPresetName] = useState("");
  const [presets, setPresets] = useState(loadFilterPresets);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handler(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setSaving(false);
      }
    }
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  function handleSave() {
    if (!currentExpr || !presetName.trim()) return;
    saveFilterPreset(presetName.trim(), currentExpr);
    setPresets(loadFilterPresets());
    setPresetName("");
    setSaving(false);
  }

  function handleDelete(id: string, e: React.MouseEvent) {
    e.stopPropagation();
    deleteFilterPreset(id);
    setPresets(loadFilterPresets());
  }

  const buttonClass = cn(
    "flex items-center gap-1 px-2 py-1.5 rounded border border-border text-xs",
    "text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors",
    open && "bg-surface-raised text-text-primary",
  );

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-label="Filter presets"
        aria-expanded={open}
        className={buttonClass}
      >
        <BookmarkCheck className="h-3.5 w-3.5" />
        <span>Presets</span>
        <ChevronDown
          className={cn("h-3 w-3 transition-transform", open && "rotate-180")}
        />
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 z-30 min-w-56 bg-surface border border-border rounded-lg shadow-lg py-1">
          {/* Save current as preset */}
          {currentExpr && (
            <div className="px-3 py-2 border-b border-border/60">
              {saving ? (
                <div className="flex items-center gap-1">
                  <input
                    type="text"
                    value={presetName}
                    onChange={(e) => setPresetName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") handleSave();
                      if (e.key === "Escape") setSaving(false);
                    }}
                    placeholder="Preset name…"
                    autoFocus
                    className={cn(
                      "flex-1 min-w-0 px-2 py-1 text-xs rounded border border-border",
                      "bg-surface text-text-primary placeholder:text-text-muted",
                      "focus:outline-none focus:ring-1 focus:ring-border",
                    )}
                  />
                  <button
                    type="button"
                    onClick={handleSave}
                    disabled={!presetName.trim()}
                    className="px-2 py-1 text-xs rounded bg-accent text-white disabled:opacity-40"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={() => setSaving(false)}
                    className="p-1 text-text-muted hover:text-text-primary"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setSaving(true)}
                  className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary transition-colors"
                >
                  <Save className="h-3 w-3" />
                  Save current filter…
                </button>
              )}
            </div>
          )}

          {/* Saved presets list */}
          {presets.length === 0 ? (
            <div className="px-3 py-2 text-xs text-text-muted">
              No saved presets.
            </div>
          ) : (
            presets.map((p) => (
              <div
                key={p.id}
                className="flex items-center gap-1 px-3 py-1.5 hover:bg-surface-raised group cursor-pointer"
                onClick={() => {
                  onLoad(p.expr as FilterGroup);
                  setOpen(false);
                }}
              >
                <span className="flex-1 text-xs text-text-primary truncate">
                  {p.name}
                </span>
                <button
                  type="button"
                  onClick={(e) => handleDelete(p.id, e)}
                  aria-label={`Delete preset "${p.name}"`}
                  className="opacity-0 group-hover:opacity-100 p-0.5 rounded text-text-muted hover:text-danger transition-all"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}

// ── Main FilterBuilder component ──────────────────────────────────────────────

export interface FilterBuilderProps {
  value: FilterGroup | null;
  onApply: (filter: FilterGroup | null) => void;
  tagSuggestions?: string[];
  groupOptions?: GroupOption[];
}

function makeDefaultGroup(): FilterGroup {
  return {
    op: "AND",
    conditions: [blankCondition(FILTER_FIELDS[0]!.field)],
  };
}

export function FilterBuilder({ value, onApply, tagSuggestions = [], groupOptions = [] }: FilterBuilderProps) {
  const [draft, setDraft] = useState<FilterGroup>(
    () => value ?? makeDefaultGroup(),
  );
  const allTags = tagSuggestions;

  // Sync draft when external value changes (e.g., cleared or loaded from URL)
  useEffect(() => {
    setDraft(value ?? makeDefaultGroup());
  }, [value]);

  // ── Top-level group helpers ────────────────────────────────────────────────

  const updateTopCondition = useCallback((idx: number, updated: FilterExpr) => {
    setDraft((prev) => ({
      ...prev,
      conditions: prev.conditions.map((c, i) => (i === idx ? updated : c)),
    }));
  }, []);

  const removeTopCondition = useCallback((idx: number) => {
    setDraft((prev) => {
      if (prev.conditions.length <= 1) return prev;
      return {
        ...prev,
        conditions: prev.conditions.filter((_, i) => i !== idx),
      };
    });
  }, []);

  const addCondition = useCallback(() => {
    setDraft((prev) => ({
      ...prev,
      conditions: [...prev.conditions, blankCondition(FILTER_FIELDS[0]!.field)],
    }));
  }, []);

  const addGroup = useCallback(() => {
    const subGroup: FilterGroup = {
      op: "OR",
      conditions: [blankCondition(FILTER_FIELDS[0]!.field)],
    };
    setDraft((prev) => ({
      ...prev,
      conditions: [...prev.conditions, subGroup],
    }));
  }, []);

  function handleApply() {
    onApply(draft);
  }

  function handleClear() {
    onApply(null);
  }

  function handleLoad(group: FilterGroup) {
    setDraft(group);
    onApply(group);
  }

  // Count leaf conditions to decide whether we can delete
  const topLeafCount = draft.conditions.filter((c) => !isGroup(c)).length;

  const hasContent =
    draft.conditions.length > 0 &&
    draft.conditions.some((c) => {
      if (isGroup(c)) {
        return c.conditions.some(
          (cc) => !isGroup(cc) && (cc as FilterCondition).value !== "",
        );
      }
      return (c as FilterCondition).value !== "";
    });

  // Check if filter is non-trivially different from active
  const serialized = serializeFilter(draft);
  const activeSerialized = serializeFilter(value);
  const isDirty = serialized !== activeSerialized;

  return (
    <div
      className="rounded-lg border border-border bg-surface p-3 space-y-3"
      data-testid="filter-builder"
    >
      {/* Header row */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <Filter className="h-3.5 w-3.5 text-text-muted" />
          <span className="text-xs font-medium text-text-secondary">
            Advanced filter
          </span>
        </div>

        <div className="flex items-center gap-1.5 text-xs text-text-muted">
          <span>Match</span>
          <OpToggle
            op={draft.op}
            onChange={(op) => setDraft((p) => ({ ...p, op }))}
          />
          <span>of the following</span>
        </div>

        <div className="sm:ml-auto flex items-center gap-2">
          <PresetsDropdown
            onLoad={handleLoad}
            currentExpr={hasContent ? draft : null}
          />
        </div>
      </div>

      {/* Conditions */}
      <div className="space-y-2">
        {draft.conditions.map((expr, idx) => {
          if (isGroup(expr)) {
            return (
              <SubGroup
                key={idx}
                group={expr}
                onChange={(g) => updateTopCondition(idx, g)}
                onRemove={() => removeTopCondition(idx)}
                tagSuggestions={allTags}
                groupOptions={groupOptions}
              />
            );
          }
          const cond = expr as FilterCondition;
          return (
            <ConditionRow
              key={idx}
              condition={cond}
              onChange={(c) => updateTopCondition(idx, c)}
              onRemove={() => removeTopCondition(idx)}
              isOnly={topLeafCount <= 1 && draft.conditions.length === 1}
              tagSuggestions={allTags}
              groupOptions={groupOptions}
            />
          );
        })}
      </div>

      {/* Add buttons */}
      <div className="flex items-center gap-3 pt-0.5">
        <button
          type="button"
          onClick={addCondition}
          className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary transition-colors"
        >
          <Plus className="h-3 w-3" />
          Add condition
        </button>
        <button
          type="button"
          onClick={addGroup}
          className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary transition-colors"
        >
          <Plus className="h-3 w-3" />
          Add group
        </button>
      </div>

      {/* Apply / Clear */}
      <div className="flex items-center gap-2 pt-1 border-t border-border/60">
        <Button
          onClick={handleApply}
          disabled={!isDirty && !hasContent}
          className="text-xs h-7 px-3"
        >
          {isDirty ? "Apply filter" : "Applied"}
        </Button>
        {(value || hasContent) && (
          <Button
            variant="ghost"
            onClick={handleClear}
            className="text-xs h-7 px-3"
          >
            Clear filter
          </Button>
        )}
      </div>
    </div>
  );
}
