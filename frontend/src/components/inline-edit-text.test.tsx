import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { InlineEditText } from "./inline-edit-text";

describe("InlineEditText", () => {
  // ── Display mode ─────────────────────────────────────────────

  it("renders the display value when not editing", () => {
    render(<InlineEditText value="router.local" onSave={vi.fn()} />);
    expect(screen.getByText("router.local")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("renders placeholder text when value is empty", () => {
    render(<InlineEditText value="" placeholder="No notes." onSave={vi.fn()} />);
    expect(screen.getByText("No notes.")).toBeInTheDocument();
  });

  it("renders default placeholder '—' when value is empty and no placeholder given", () => {
    render(<InlineEditText value="" onSave={vi.fn()} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("shows an edit button on hover", () => {
    render(<InlineEditText value="test" onSave={vi.fn()} />);
    // The clickable text area is the single accessible Edit button; the pencil is aria-hidden
    expect(screen.getByRole("button", { name: /^edit$/i })).toBeInTheDocument();
  });

  // ── Enter edit mode ─────────────────────────────────────────

  it("enters edit mode when the display text is clicked", async () => {
    render(<InlineEditText value="router.local" onSave={vi.fn()} />);
    // Click the first "Edit" button (the display text button)
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("pre-fills the input with the current value when entering edit mode", async () => {
    render(<InlineEditText value="router.local" onSave={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    expect(screen.getByRole("textbox")).toHaveValue("router.local");
  });

  // ── Single-line save ──────────────────────────────────────

  it("calls onSave with the trimmed input value when Enter is pressed", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const input = screen.getByRole("textbox");
    await userEvent.clear(input);
    await userEvent.type(input, "  new-hostname  ");
    await userEvent.keyboard("{Enter}");
    expect(onSave).toHaveBeenCalledWith("new-hostname");
  });

  it("calls onSave when the Save (check) button is clicked", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.clear(screen.getByRole("textbox"));
    await userEvent.type(screen.getByRole("textbox"), "saved");
    await userEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledWith("saved");
  });

  it("exits edit mode after a successful save", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.keyboard("{Enter}");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  // ── Cancel ────────────────────────────────────────────────

  it("cancels without calling onSave when Escape is pressed", async () => {
    const onSave = vi.fn();
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.type(screen.getByRole("textbox"), " modified");
    await userEvent.keyboard("{Escape}");
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("cancels without calling onSave when the Cancel button is clicked", async () => {
    const onSave = vi.fn();
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  // ── Error handling ─────────────────────────────────────────

  it("stays in edit mode and shows error message when onSave rejects", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("Network error"));
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.keyboard("{Enter}");
    // Should remain in edit mode
    expect(screen.getByRole("textbox")).toBeInTheDocument();
    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  // ── Multiline mode ─────────────────────────────────────────

  it("renders a textarea in multiline mode", async () => {
    render(<InlineEditText value="notes" onSave={vi.fn()} multiline />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    expect(screen.getByRole("textbox")).toBeInTheDocument();
    // Textarea has rows attribute
    expect(screen.getByRole("textbox").tagName).toBe("TEXTAREA");
  });

  it("saves on Ctrl+Enter in multiline mode", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditText value="initial" onSave={onSave} multiline />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const textarea = screen.getByRole("textbox");
    await userEvent.clear(textarea);
    await userEvent.type(textarea, "updated notes");
    await userEvent.keyboard("{Control>}{Enter}{/Control}");
    expect(onSave).toHaveBeenCalledWith("updated notes");
  });

  it("does not save on plain Enter in multiline mode", async () => {
    const onSave = vi.fn();
    render(<InlineEditText value="initial" onSave={onSave} multiline />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.keyboard("{Enter}");
    expect(onSave).not.toHaveBeenCalled();
    // Still in edit mode
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("cancels on Escape in multiline mode", async () => {
    const onSave = vi.fn();
    render(<InlineEditText value="initial" onSave={onSave} multiline />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.keyboard("{Escape}");
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  // ── Blur behaviour ─────────────────────────────────────────

  it("blur cancels editing when not saving and no error", async () => {
    render(<InlineEditText value="original" onSave={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    expect(screen.getByRole("textbox")).toBeInTheDocument();
    // Fire a synthetic blur event — should cancel editing
    fireEvent.blur(screen.getByRole("textbox"));
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("stays in edit mode on blur when a save error is shown", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("Network error"));
    render(<InlineEditText value="old" onSave={onSave} />);
    await userEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await userEvent.keyboard("{Enter}");
    // Error is shown — blur should NOT cancel editing
    expect(screen.getByText("Network error")).toBeInTheDocument();
    fireEvent.blur(screen.getByRole("textbox"));
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  // ── Disabled state ──────────────────────────────────────────

  it("does not enter edit mode when disabled", async () => {
    render(<InlineEditText value="locked" onSave={vi.fn()} disabled />);
    // Only the display button is in the DOM when disabled (no pencil button rendered)
    const btn = screen.getByRole("button", { name: /^edit$/i });
    await userEvent.click(btn);
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
});
