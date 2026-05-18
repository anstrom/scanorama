import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { KeyboardShortcutHelp } from "./keyboard-shortcut-help";

describe("KeyboardShortcutHelp", () => {
  it("renders the dialog with its title", () => {
    render(<KeyboardShortcutHelp onClose={vi.fn()} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByText("Keyboard shortcuts"),
    ).toBeInTheDocument();
  });

  it("renders all navigation bindings", () => {
    render(<KeyboardShortcutHelp onClose={vi.fn()} />);

    expect(screen.getByText("Go to Hosts")).toBeInTheDocument();
    expect(screen.getByText("Go to Scans")).toBeInTheDocument();
    expect(screen.getByText("Go to Networks")).toBeInTheDocument();
    expect(screen.getByText("Go to Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Go to Admin")).toBeInTheDocument();
  });

  it("renders all action bindings", () => {
    render(<KeyboardShortcutHelp onClose={vi.fn()} />);

    expect(screen.getByText("New scan")).toBeInTheDocument();
    expect(screen.getByText("Toggle this help")).toBeInTheDocument();
    expect(screen.getByText("Close overlay")).toBeInTheDocument();
  });

  it("renders section headings", () => {
    render(<KeyboardShortcutHelp onClose={vi.fn()} />);

    expect(screen.getByText("Navigation")).toBeInTheDocument();
    expect(screen.getByText("Actions")).toBeInTheDocument();
  });

  it("calls onClose when the close button is clicked", () => {
    const onClose = vi.fn();
    render(<KeyboardShortcutHelp onClose={onClose} />);

    fireEvent.click(screen.getByRole("button", { name: /close dialog/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the backdrop is clicked", () => {
    const onClose = vi.fn();
    render(<KeyboardShortcutHelp onClose={onClose} />);

    // The backdrop is the div with aria-hidden="true" and bg-black/50
    const backdrop = document.querySelector("[aria-hidden='true']") as HTMLElement;
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose on Escape key", () => {
    const onClose = vi.fn();
    render(<KeyboardShortcutHelp onClose={onClose} />);

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("displays key badges for g h binding", () => {
    render(<KeyboardShortcutHelp onClose={vi.fn()} />);

    // Multiple 'g' badges appear (one per g-prefixed binding)
    const gBadges = screen.getAllByText("g");
    expect(gBadges.length).toBeGreaterThanOrEqual(1);

    // 'h' appears as key badge for the "Go to Hosts" binding
    expect(screen.getByText("h")).toBeInTheDocument();
  });
});
