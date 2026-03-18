import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Button } from "./button";

describe("Button", () => {
  // ── rendering ──────────────────────────────────────────────────────────────

  it("renders children", () => {
    render(<Button>Click me</Button>);
    expect(
      screen.getByRole("button", { name: "Click me" }),
    ).toBeInTheDocument();
  });

  it("renders as a <button> element", () => {
    render(<Button>Go</Button>);
    expect(screen.getByRole("button")).toBeInstanceOf(HTMLButtonElement);
  });

  // ── variants ───────────────────────────────────────────────────────────────

  it("applies primary variant classes by default", () => {
    render(<Button>Primary</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/bg-accent/);
  });

  it("applies secondary variant classes", () => {
    render(<Button variant="secondary">Secondary</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/border-border/);
  });

  it("applies ghost variant classes", () => {
    render(<Button variant="ghost">Ghost</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/hover:bg-surface-raised/);
  });

  it("applies danger variant classes", () => {
    render(<Button variant="danger">Delete</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/bg-danger/);
  });

  // ── sizes ──────────────────────────────────────────────────────────────────

  it("applies sm size classes by default", () => {
    render(<Button>Small</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/text-xs/);
  });

  it("applies md size classes", () => {
    render(<Button size="md">Medium</Button>);
    const btn = screen.getByRole("button");
    expect(btn.className).toMatch(/text-sm/);
  });

  // ── loading state ──────────────────────────────────────────────────────────

  it("shows spinner and disables button when loading", () => {
    render(<Button loading>Save</Button>);
    const btn = screen.getByRole("button");
    expect(btn).toBeDisabled();
    // Spinner SVG is present; children text is still in the DOM but spinner replaces icon slot
    const svg = btn.querySelector("svg");
    expect(svg).toBeInTheDocument();
    // SVGAnimatedString — use getAttribute or check the element's class list
    expect(svg!.classList.contains("animate-spin")).toBe(true);
  });

  it("still renders children text while loading", () => {
    render(<Button loading>Saving…</Button>);
    expect(screen.getByText("Saving…")).toBeInTheDocument();
  });

  it("is not disabled when loading is false", () => {
    render(<Button loading={false}>Ready</Button>);
    expect(screen.getByRole("button")).not.toBeDisabled();
  });

  // ── disabled ───────────────────────────────────────────────────────────────

  it("is disabled when disabled prop is set", () => {
    render(<Button disabled>Can't click</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("is disabled when both disabled and loading are set", () => {
    render(
      <Button disabled loading>
        Busy
      </Button>,
    );
    expect(screen.getByRole("button")).toBeDisabled();
  });

  // ── icon slot ──────────────────────────────────────────────────────────────

  it("renders the icon when provided and not loading", () => {
    const TestIcon = () => <svg data-testid="test-icon" />;
    render(<Button icon={<TestIcon />}>With icon</Button>);
    expect(screen.getByTestId("test-icon")).toBeInTheDocument();
  });

  it("hides the icon and shows spinner when loading", () => {
    const TestIcon = () => <svg data-testid="test-icon" />;
    render(
      <Button icon={<TestIcon />} loading>
        Loading
      </Button>,
    );
    expect(screen.queryByTestId("test-icon")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button").querySelector(".animate-spin"),
    ).toBeInTheDocument();
  });

  // ── click handler ──────────────────────────────────────────────────────────

  it("calls onClick when clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(<Button onClick={handleClick}>Click</Button>);
    await user.click(screen.getByRole("button"));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it("does not call onClick when disabled", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(
      <Button disabled onClick={handleClick}>
        Disabled
      </Button>,
    );
    await user.click(screen.getByRole("button"));
    expect(handleClick).not.toHaveBeenCalled();
  });

  it("does not call onClick when loading", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(
      <Button loading onClick={handleClick}>
        Loading
      </Button>,
    );
    await user.click(screen.getByRole("button"));
    expect(handleClick).not.toHaveBeenCalled();
  });

  // ── type attribute ─────────────────────────────────────────────────────────

  it("respects an explicit type=submit", () => {
    render(<Button type="submit">Submit</Button>);
    expect(screen.getByRole("button")).toHaveAttribute("type", "submit");
  });

  it("respects an explicit type=button", () => {
    render(<Button type="button">Button</Button>);
    expect(screen.getByRole("button")).toHaveAttribute("type", "button");
  });

  // ── extra className ────────────────────────────────────────────────────────

  it("merges a custom className", () => {
    render(<Button className="my-custom-class">Styled</Button>);
    expect(screen.getByRole("button").className).toMatch(/my-custom-class/);
  });

  // ── forwarded HTML attributes ──────────────────────────────────────────────

  it("forwards arbitrary HTML attributes", () => {
    render(<Button aria-label="close dialog">X</Button>);
    expect(
      screen.getByRole("button", { name: "close dialog" }),
    ).toBeInTheDocument();
  });
});
