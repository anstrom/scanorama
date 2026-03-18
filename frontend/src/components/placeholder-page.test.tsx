import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { PlaceholderPage } from "./placeholder-page";

describe("PlaceholderPage", () => {
  it("renders the provided message", () => {
    render(<PlaceholderPage message="Coming soon." />);
    expect(screen.getByText("Coming soon.")).toBeInTheDocument();
  });

  it("renders different messages correctly", () => {
    const { rerender } = render(
      <PlaceholderPage message="Network management coming soon." />,
    );
    expect(
      screen.getByText("Network management coming soon."),
    ).toBeInTheDocument();

    rerender(<PlaceholderPage message="Admin panel coming soon." />);
    expect(screen.getByText("Admin panel coming soon.")).toBeInTheDocument();
  });

  it("renders the message inside a paragraph element", () => {
    render(<PlaceholderPage message="Scan profiles coming soon." />);
    const p = screen.getByText("Scan profiles coming soon.");
    expect(p.tagName).toBe("P");
  });

  it("wraps content in a container div", () => {
    const { container } = render(
      <PlaceholderPage message="Schedule management coming soon." />,
    );
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.tagName).toBe("DIV");
  });

  it("renders an empty message without crashing", () => {
    render(<PlaceholderPage message="" />);
    // Should render without throwing.
    expect(document.body).toBeInTheDocument();
  });
});
