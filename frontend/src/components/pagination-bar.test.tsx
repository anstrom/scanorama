import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PaginationBar } from "./pagination-bar";

describe("PaginationBar", () => {
  // 1. Renders page info
  it("renders the current page and total pages", () => {
    render(
      <PaginationBar page={3} totalPages={10} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByText("Page 3 of 10")).toBeInTheDocument();
  });

  it("renders page 1 of 1 correctly", () => {
    render(
      <PaginationBar page={1} totalPages={1} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByText("Page 1 of 1")).toBeInTheDocument();
  });

  // 2. Prev button disabled on page 1
  it("disables the Previous button when on page 1", () => {
    render(
      <PaginationBar page={1} totalPages={5} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();
  });

  it("enables the Previous button when not on page 1", () => {
    render(
      <PaginationBar page={2} totalPages={5} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "Previous page" })).not.toBeDisabled();
  });

  // 3. Next button disabled on last page
  it("disables the Next button when on the last page", () => {
    render(
      <PaginationBar page={5} totalPages={5} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();
  });

  it("enables the Next button when not on the last page", () => {
    render(
      <PaginationBar page={3} totalPages={5} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "Next page" })).not.toBeDisabled();
  });

  // 4. Both buttons disabled on page 1 of 1
  it("disables both buttons when there is only one page", () => {
    render(
      <PaginationBar page={1} totalPages={1} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();
  });

  // 5. Callback: onPrev called when Previous clicked
  it("calls onPrev when the Previous button is clicked", async () => {
    const onPrev = vi.fn();
    render(
      <PaginationBar page={3} totalPages={5} onPrev={onPrev} onNext={vi.fn()} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  // 6. Callback: onNext called when Next clicked
  it("calls onNext when the Next button is clicked", async () => {
    const onNext = vi.fn();
    render(
      <PaginationBar page={3} totalPages={5} onPrev={vi.fn()} onNext={onNext} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  // 7. Callback: onPrev NOT called when Previous is disabled
  it("does not call onPrev when the Previous button is disabled", async () => {
    const onPrev = vi.fn();
    render(
      <PaginationBar page={1} totalPages={5} onPrev={onPrev} onNext={vi.fn()} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(onPrev).not.toHaveBeenCalled();
  });

  // 8. Callback: onNext NOT called when Next is disabled
  it("does not call onNext when the Next button is disabled", async () => {
    const onNext = vi.fn();
    render(
      <PaginationBar page={5} totalPages={5} onPrev={vi.fn()} onNext={onNext} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(onNext).not.toHaveBeenCalled();
  });

  // 9. Renders Previous and Next button labels
  it("renders Previous and Next buttons", () => {
    render(
      <PaginationBar page={2} totalPages={4} onPrev={vi.fn()} onNext={vi.fn()} />,
    );
    expect(screen.getByText("Previous")).toBeInTheDocument();
    expect(screen.getByText("Next")).toBeInTheDocument();
  });

  // 10. Accepts optional className
  it("applies an optional className to the container", () => {
    const { container } = render(
      <PaginationBar
        page={1}
        totalPages={3}
        onPrev={vi.fn()}
        onNext={vi.fn()}
        className="mt-4"
      />,
    );
    expect(container.firstChild).toHaveClass("mt-4");
  });
});
