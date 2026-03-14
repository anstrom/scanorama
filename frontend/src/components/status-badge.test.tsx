import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StatusBadge } from "./status-badge";

describe("StatusBadge", () => {
  it("renders the status text", () => {
    render(<StatusBadge status="up" />);
    expect(screen.getByText("up")).toBeInTheDocument();
  });

  it("applies success styling for 'up' status", () => {
    render(<StatusBadge status="up" />);
    const badge = screen.getByText("up");
    expect(badge).toHaveClass("text-success");
  });

  it("applies danger styling for 'failed' status", () => {
    render(<StatusBadge status="failed" />);
    const badge = screen.getByText("failed");
    expect(badge).toHaveClass("text-danger");
  });

  it("applies info styling for 'running' status", () => {
    render(<StatusBadge status="running" />);
    const badge = screen.getByText("running");
    expect(badge).toHaveClass("text-info");
  });

  it("applies warning styling for 'pending' status", () => {
    render(<StatusBadge status="pending" />);
    const badge = screen.getByText("pending");
    expect(badge).toHaveClass("text-warning");
  });

  it("applies muted styling for unknown statuses", () => {
    render(<StatusBadge status="something-weird" />);
    const badge = screen.getByText("something-weird");
    expect(badge).toHaveClass("text-text-muted");
  });

  it("normalizes status to lowercase for matching", () => {
    render(<StatusBadge status="RUNNING" />);
    const badge = screen.getByText("RUNNING");
    expect(badge).toHaveClass("text-info");
  });

  it("preserves original casing in displayed text", () => {
    render(<StatusBadge status="Completed" />);
    expect(screen.getByText("Completed")).toBeInTheDocument();
  });

  it("accepts additional className", () => {
    render(<StatusBadge status="up" className="ml-2" />);
    const badge = screen.getByText("up");
    expect(badge).toHaveClass("ml-2");
  });

  it("renders as an inline element with base styling", () => {
    render(<StatusBadge status="up" />);
    const badge = screen.getByText("up");
    expect(badge).toHaveClass("inline-flex", "rounded", "text-xs", "font-medium");
  });
});
