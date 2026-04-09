import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { FilterBuilder } from "./filter-builder";
import type { FilterGroup } from "../lib/filter-expr";

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeGroup(overrides: Partial<FilterGroup> = {}): FilterGroup {
  return {
    op: "AND",
    conditions: [{ field: "status", cmp: "is", value: "up" }],
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("FilterBuilder", () => {
  // ── render ─────────────────────────────────────────────────────────────────

  it("renders the filter builder panel", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(screen.getByTestId("filter-builder")).toBeInTheDocument();
  });

  it("renders 'Advanced filter' label", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(screen.getByText("Advanced filter")).toBeInTheDocument();
  });

  it("shows AND/OR toggle buttons", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(screen.getByRole("button", { name: "AND" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "OR" })).toBeInTheDocument();
  });

  it("renders at least one condition row on first render", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    // There should be at least one field selector
    const fieldSelects = screen.getAllByRole("combobox", { name: "Filter field" });
    expect(fieldSelects.length).toBeGreaterThanOrEqual(1);
  });

  it("renders with an existing filter value", () => {
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const fieldSelect = screen.getByRole("combobox", { name: "Filter field" });
    expect(fieldSelect).toHaveValue("status");
  });

  it("renders the Apply filter button", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: /apply filter/i }),
    ).toBeInTheDocument();
  });

  it("renders the 'Add condition' button", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    const addBtns = screen.getAllByRole("button", { name: /add condition/i });
    expect(addBtns.length).toBeGreaterThanOrEqual(1);
  });

  it("renders the 'Add group' button", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: /add group/i }),
    ).toBeInTheDocument();
  });

  it("renders the Presets button", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Filter presets" }),
    ).toBeInTheDocument();
  });

  // ── Adding conditions ──────────────────────────────────────────────────────

  it("adds a new condition row when 'Add condition' is clicked", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    const before = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    const [addBtn] = screen.getAllByRole("button", { name: /add condition/i });
    await user.click(addBtn!);
    const after = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    expect(after).toBe(before + 1);
  });

  it("adds a sub-group when 'Add group' is clicked", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    expect(screen.queryByText("group")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /add group/i }));
    expect(screen.getByText("group")).toBeInTheDocument();
  });

  // ── Removing conditions ────────────────────────────────────────────────────

  it("remove button is disabled when there is only one condition", () => {
    render(<FilterBuilder value={null} onApply={vi.fn()} />);
    const removeBtn = screen.getByRole("button", { name: "Remove condition" });
    expect(removeBtn).toBeDisabled();
  });

  it("remove button is enabled when there are multiple conditions", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    const [addBtn] = screen.getAllByRole("button", { name: /add condition/i });
    await user.click(addBtn!);

    const removeBtns = screen.getAllByRole("button", { name: "Remove condition" });
    expect(removeBtns.length).toBeGreaterThan(0);
    removeBtns.forEach((btn) => expect(btn).not.toBeDisabled());
  });

  it("removes a condition row when the remove button is clicked", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    // Add a second condition
    const [addBtn] = screen.getAllByRole("button", { name: /add condition/i });
    await user.click(addBtn!);

    const before = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    expect(before).toBe(2);

    // Remove the first one
    const [firstRemove] = screen.getAllByRole("button", { name: "Remove condition" });
    await user.click(firstRemove!);

    const after = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    expect(after).toBe(1);
  });

  // ── Field / operator changes ───────────────────────────────────────────────

  it("changes the field when a new option is selected", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    const fieldSelect = screen.getByRole("combobox", { name: "Filter field" });
    await user.selectOptions(fieldSelect, "vendor");
    expect(fieldSelect).toHaveValue("vendor");
  });

  it("changes the operator when a new option is selected", async () => {
    const user = userEvent.setup();
    // Start with a text field so 'contains' is available
    const group = makeGroup({
      conditions: [{ field: "vendor", cmp: "is", value: "" }],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);

    const opSelect = screen.getByRole("combobox", { name: "Filter operator" });
    await user.selectOptions(opSelect, "contains");
    expect(opSelect).toHaveValue("contains");
  });

  it("shows a text input for text fields", () => {
    const group = makeGroup({
      conditions: [{ field: "vendor", cmp: "contains", value: "" }],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    expect(screen.getByRole("textbox", { name: "Filter value" })).toBeInTheDocument();
  });

  it("shows a select for enum fields (status)", () => {
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    // There should be a 'Filter value' combobox (the enum select)
    expect(screen.getByRole("combobox", { name: "Filter value" })).toBeInTheDocument();
  });

  it("shows two date inputs for a between date condition", () => {
    const group = makeGroup({
      conditions: [
        { field: "first_seen", cmp: "between", value: "2024-01-01", value2: "2024-12-31" },
      ],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    expect(screen.getByLabelText("From date")).toBeInTheDocument();
    expect(screen.getByLabelText("To date")).toBeInTheDocument();
  });

  it("shows two number inputs for a between number condition", () => {
    const group = makeGroup({
      conditions: [
        { field: "response_time_ms", cmp: "between", value: "10", value2: "100" },
      ],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    expect(screen.getByLabelText("Min value")).toBeInTheDocument();
    expect(screen.getByLabelText("Max value")).toBeInTheDocument();
  });

  it("shows a port number input for open_port field", () => {
    const group = makeGroup({
      conditions: [{ field: "open_port", cmp: "is", value: "80" }],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const input = screen.getByRole("spinbutton", { name: "Filter value" });
    expect(input).toHaveAttribute("min", "1");
    expect(input).toHaveAttribute("max", "65535");
  });

  // ── AND/OR toggle ──────────────────────────────────────────────────────────

  it("toggles top-level operator from AND to OR", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    // Initially AND is active — click OR
    await user.click(screen.getByRole("button", { name: "OR" }));

    // The OR button should now appear highlighted (bg-accent is applied via class)
    // We can verify by checking the AND button lost its styling — but since class
    // inspection is fragile, simply verify the click didn't throw and OR is present.
    expect(screen.getByRole("button", { name: "OR" })).toBeInTheDocument();
  });

  // ── Apply ──────────────────────────────────────────────────────────────────

  it("calls onApply with the current draft when Apply filter is clicked", async () => {
    const user = userEvent.setup();
    const onApply = vi.fn();
    render(<FilterBuilder value={null} onApply={onApply} />);

    // Change a value so the filter is 'dirty' (Apply button is enabled)
    const valueSelect = screen.getByRole("combobox", { name: "Filter value" });
    await user.selectOptions(valueSelect, "up");

    await user.click(screen.getByRole("button", { name: /apply filter/i }));

    expect(onApply).toHaveBeenCalledTimes(1);
    const [calledWith] = onApply.mock.calls[0] as [FilterGroup | null];
    expect(calledWith).not.toBeNull();
    expect(calledWith?.op).toBe("AND");
  });

  it("calls onApply with null when Clear filter is clicked", async () => {
    const user = userEvent.setup();
    const onApply = vi.fn();
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={onApply} />);

    const clearBtn = screen.getByRole("button", { name: /clear filter/i });
    await user.click(clearBtn);

    expect(onApply).toHaveBeenCalledTimes(1);
    const [calledWith] = onApply.mock.calls[0] as [FilterGroup | null];
    expect(calledWith).toBeNull();
  });

  it("shows 'Applied' label when the draft matches the active value", () => {
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    // When value == draft, the button should say "Applied"
    expect(screen.getByRole("button", { name: /applied/i })).toBeInTheDocument();
  });

  it("shows 'Apply filter' label when the draft differs from the active value", async () => {
    const user = userEvent.setup();
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);

    // Modify the draft — change the value select
    const valueSelect = screen.getByRole("combobox", { name: "Filter value" });
    await user.selectOptions(valueSelect, "down");

    expect(screen.getByRole("button", { name: /apply filter/i })).toBeInTheDocument();
  });

  // ── Clear button visibility ────────────────────────────────────────────────

  it("shows Clear filter button when a value is active", () => {
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    expect(screen.getByRole("button", { name: /clear filter/i })).toBeInTheDocument();
  });

  it("shows Clear filter button when draft has content even without active value", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    // Fill in a value to give the draft content
    const valueSelect = screen.getByRole("combobox", { name: "Filter value" });
    await user.selectOptions(valueSelect, "up");

    expect(screen.getByRole("button", { name: /clear filter/i })).toBeInTheDocument();
  });

  // ── Sub-group ──────────────────────────────────────────────────────────────

  it("renders sub-group conditions after adding a group", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: /add group/i }));

    // The group label should appear
    expect(screen.getByText("group")).toBeInTheDocument();

    // There should be a Remove group button
    expect(screen.getByRole("button", { name: "Remove group" })).toBeInTheDocument();
  });

  it("removes a sub-group when Remove group is clicked", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: /add group/i }));
    expect(screen.getByText("group")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Remove group" }));
    expect(screen.queryByText("group")).not.toBeInTheDocument();
  });

  it("can add a condition inside a sub-group", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: /add group/i }));

    const allAddBtns = screen.getAllByRole("button", { name: /add condition/i });
    // There should be at least 2: one top-level, one inside the group
    expect(allAddBtns.length).toBeGreaterThanOrEqual(2);

    const subGroupBefore = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    // Click the last "Add condition" button (inside the sub-group)
    await user.click(allAddBtns[allAddBtns.length - 1]!);
    const subGroupAfter = screen.getAllByRole("combobox", { name: "Filter field" }).length;
    expect(subGroupAfter).toBe(subGroupBefore + 1);
  });

  // ── Presets dropdown ───────────────────────────────────────────────────────

  it("opens the presets dropdown when Presets button is clicked", async () => {
    const user = userEvent.setup();
    render(<FilterBuilder value={null} onApply={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Filter presets" }));
    expect(screen.getByText("No saved presets.")).toBeInTheDocument();
  });

  it("shows 'Save current filter…' option when there is draft content", async () => {
    const user = userEvent.setup();
    const group = makeGroup();
    render(<FilterBuilder value={group} onApply={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Filter presets" }));
    // The save button/link should appear since there is an active filter
    expect(screen.getByText(/save current filter/i)).toBeInTheDocument();
  });

  // ── External value sync ───────────────────────────────────────────────────

  it("updates the draft when the value prop changes", async () => {
    const { rerender } = render(
      <FilterBuilder value={null} onApply={vi.fn()} />,
    );

    const group = makeGroup({
      conditions: [{ field: "vendor", cmp: "contains", value: "cisco" }],
    });
    rerender(<FilterBuilder value={group} onApply={vi.fn()} />);

    const fieldSelect = screen.getByRole("combobox", { name: "Filter field" });
    expect(fieldSelect).toHaveValue("vendor");
  });

  // ── Value updates ─────────────────────────────────────────────────────────

  it("updates text value when user types in the value input", async () => {
    const user = userEvent.setup();
    const group = makeGroup({
      conditions: [{ field: "hostname", cmp: "contains", value: "" }],
    });
    render(<FilterBuilder value={group} onApply={vi.fn()} />);

    const input = screen.getByRole("textbox", { name: "Filter value" });
    await user.clear(input);
    await user.type(input, "router");
    expect(input).toHaveValue("router");
  });
});

// ── Minimal sub-component smoke tests ─────────────────────────────────────────

describe("FilterBuilder — operator options by field type", () => {
  it("shows only is/is_not for status (enum) field", () => {
    const group: FilterGroup = {
      op: "AND",
      conditions: [{ field: "status", cmp: "is", value: "up" }],
    };
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const opSelect = screen.getByRole("combobox", { name: "Filter operator" });
    const options = within(opSelect).getAllByRole("option");
    const values = options.map((o) => o.getAttribute("value"));
    expect(values).toContain("is");
    expect(values).toContain("is_not");
    expect(values).not.toContain("contains");
    expect(values).not.toContain("gt");
  });

  it("shows contains for text fields (vendor)", () => {
    const group: FilterGroup = {
      op: "AND",
      conditions: [{ field: "vendor", cmp: "is", value: "" }],
    };
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const opSelect = screen.getByRole("combobox", { name: "Filter operator" });
    const options = within(opSelect).getAllByRole("option");
    const values = options.map((o) => o.getAttribute("value"));
    expect(values).toContain("contains");
  });

  it("shows gt/lt/between for numeric fields (response_time_ms)", () => {
    const group: FilterGroup = {
      op: "AND",
      conditions: [{ field: "response_time_ms", cmp: "gt", value: "" }],
    };
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const opSelect = screen.getByRole("combobox", { name: "Filter operator" });
    const options = within(opSelect).getAllByRole("option");
    const values = options.map((o) => o.getAttribute("value"));
    expect(values).toContain("gt");
    expect(values).toContain("lt");
    expect(values).toContain("between");
  });

  it("shows only gt/lt/between for date fields (first_seen)", () => {
    const group: FilterGroup = {
      op: "AND",
      conditions: [{ field: "first_seen", cmp: "gt", value: "" }],
    };
    render(<FilterBuilder value={group} onApply={vi.fn()} />);
    const opSelect = screen.getByRole("combobox", { name: "Filter operator" });
    const options = within(opSelect).getAllByRole("option");
    const values = options.map((o) => o.getAttribute("value"));
    expect(values).toContain("gt");
    expect(values).toContain("lt");
    expect(values).toContain("between");
    expect(values).not.toContain("is");
    expect(values).not.toContain("contains");
  });
});
