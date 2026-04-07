import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ColumnToggle } from "./column-toggle";
import type { ColumnDef } from "./column-toggle";

// ── Fixtures ──────────────────────────────────────────────────────────────────

const baseColumns: ColumnDef[] = [
  { key: "name", label: "Name" },
  { key: "status", label: "Status" },
  { key: "ip", label: "IP Address", alwaysVisible: true },
];

const baseVisibility: Record<string, boolean> = {
  name: true,
  status: false,
  ip: true,
};

/** Click the gear button to open the dropdown. */
async function openDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Toggle columns" }));
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("ColumnToggle", () => {
  // ── render ────────────────────────────────────────────────────────────────

  it("renders the Toggle columns button", () => {
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Toggle columns" }),
    ).toBeInTheDocument();
  });

  it("dropdown is closed initially", () => {
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  // ── open / close ──────────────────────────────────────────────────────────

  it("clicking the button opens the dropdown and shows all column labels", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("IP Address")).toBeInTheDocument();
  });

  it("clicking the button a second time closes the dropdown", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    expect(screen.getByRole("menu")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Toggle columns" }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("clicking outside the dropdown closes it", async () => {
    const user = userEvent.setup();
    render(
      <div>
        <p data-testid="outside">Outside element</p>
        <ColumnToggle
          columns={baseColumns}
          visibility={baseVisibility}
          onToggle={vi.fn()}
        />
      </div>,
    );
    await openDropdown(user);
    expect(screen.getByRole("menu")).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByTestId("outside"));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("clicking inside the dropdown does not close it", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    fireEvent.mouseDown(screen.getByRole("menu"));
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  // ── toggle callback ───────────────────────────────────────────────────────

  it("clicking a visible column checkbox calls onToggle with the column key", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={onToggle}
      />,
    );
    await openDropdown(user);
    // "name" column: checked, not disabled — first checkbox in the list
    const [nameCheckbox] = screen.getAllByRole("checkbox");
    await user.click(nameCheckbox!);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle).toHaveBeenCalledWith("name");
  });

  it("clicking a hidden column checkbox calls onToggle with the column key", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={onToggle}
      />,
    );
    await openDropdown(user);
    // "status" column: unchecked, not disabled — second checkbox in the list
    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[1]!);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle).toHaveBeenCalledWith("status");
  });

  // ── always-visible (disabled) columns ────────────────────────────────────

  it("always-visible columns have their checkboxes disabled", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    // "ip" column has alwaysVisible: true — third checkbox in the list
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[2]).toBeDisabled();
  });

  it("clicking a disabled always-visible checkbox does not call onToggle", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={onToggle}
      />,
    );
    await openDropdown(user);
    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[2]!);
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("always-visible menuitemcheckbox has aria-disabled=true", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    const menuItems = screen.getAllByRole("menuitemcheckbox");
    const ipItem = menuItems.find((el) =>
      el.textContent?.includes("IP Address"),
    );
    expect(ipItem).toHaveAttribute("aria-disabled", "true");
  });

  // ── checkbox checked state ────────────────────────────────────────────────

  it("unchecked columns show an unchecked checkbox", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    // "status" is visibility: false — second checkbox
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[1]).not.toBeChecked();
  });

  it("checked columns show a checked checkbox", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    // "name" is visibility: true — first checkbox
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[0]).toBeChecked();
  });

  it("defaults to checked when a column key is absent from visibility", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={[{ key: "brand-new", label: "Brand New" }]}
        visibility={{}}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    const [checkbox] = screen.getAllByRole("checkbox");
    expect(checkbox).toBeChecked();
  });

  // ── ARIA attributes ───────────────────────────────────────────────────────

  it("button has aria-expanded=false when the dropdown is closed", () => {
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Toggle columns" }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("button has aria-expanded=true when the dropdown is open", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    expect(
      screen.getByRole("button", { name: "Toggle columns" }),
    ).toHaveAttribute("aria-expanded", "true");
  });

  it("dropdown has role=menu", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("each column row has role=menuitemcheckbox", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    const items = screen.getAllByRole("menuitemcheckbox");
    expect(items).toHaveLength(baseColumns.length);
  });

  it("checked menuitemcheckbox has aria-checked=true", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    const menuItems = screen.getAllByRole("menuitemcheckbox");
    const nameItem = menuItems.find((el) => el.textContent?.includes("Name"));
    expect(nameItem).toHaveAttribute("aria-checked", "true");
  });

  it("unchecked menuitemcheckbox has aria-checked=false", async () => {
    const user = userEvent.setup();
    render(
      <ColumnToggle
        columns={baseColumns}
        visibility={baseVisibility}
        onToggle={vi.fn()}
      />,
    );
    await openDropdown(user);
    const menuItems = screen.getAllByRole("menuitemcheckbox");
    const statusItem = menuItems.find((el) =>
      el.textContent?.includes("Status"),
    );
    expect(statusItem).toHaveAttribute("aria-checked", "false");
  });
});
