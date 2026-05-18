import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ExportButton } from "./export-button";

describe("ExportButton", () => {
  let originalLocationDescriptor: PropertyDescriptor | undefined;
  const hrefSetter = vi.fn();

  beforeEach(() => {
    // Save the original descriptor so we can restore it in afterEach.
    originalLocationDescriptor = Object.getOwnPropertyDescriptor(window, "location");
    const originalHref = window.location.href;
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        ...window.location,
        get href() {
          return originalHref;
        },
        set href(url: string) {
          hrefSetter(url);
        },
      },
    });
  });

  afterEach(() => {
    if (originalLocationDescriptor) {
      Object.defineProperty(window, "location", originalLocationDescriptor);
    }
    hrefSetter.mockClear();
  });

  // ── rendering ──────────────────────────────────────────────────────────────

  it("renders the Export label", () => {
    render(<ExportButton basePath="/api/v1/hosts/export" params={{}} />);
    expect(screen.getByRole("button", { name: "Export CSV" })).toBeInTheDocument();
  });

  it("renders a custom label", () => {
    render(
      <ExportButton basePath="/api/v1/hosts/export" params={{}} label="Download" />,
    );
    expect(screen.getByRole("button", { name: "Download CSV" })).toBeInTheDocument();
  });

  it("renders the chevron button for the format picker", () => {
    render(<ExportButton basePath="/api/v1/hosts/export" params={{}} />);
    expect(
      screen.getByRole("button", { name: "Export format picker" }),
    ).toBeInTheDocument();
  });

  // ── primary click → CSV ────────────────────────────────────────────────────

  it("clicking the primary button sets window.location.href to the CSV export URL", async () => {
    const user = userEvent.setup();
    render(
      <ExportButton
        basePath="/api/v1/hosts/export"
        params={{ sort_by: "last_seen", sort_order: "desc", status: "up" }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Export CSV" }));

    expect(hrefSetter).toHaveBeenCalledOnce();
    const url = hrefSetter.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/hosts/export");
    expect(url).toContain("format=csv");
    expect(url).toContain("sort_by=last_seen");
    expect(url).toContain("status=up");
  });

  // ── dropdown menu ──────────────────────────────────────────────────────────

  it("opens the dropdown when the chevron is clicked", async () => {
    const user = userEvent.setup();
    render(<ExportButton basePath="/api/v1/hosts/export" params={{}} />);

    // Menu should not be visible initially.
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Export format picker" }));

    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Export CSV" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Export JSON" })).toBeInTheDocument();
  });

  it("clicking 'Export CSV' from the menu sets href with format=csv", async () => {
    const user = userEvent.setup();
    render(
      <ExportButton basePath="/api/v1/scans/export" params={{ sort_by: "created_at" }} />,
    );

    await user.click(screen.getByRole("button", { name: "Export format picker" }));
    await user.click(screen.getByRole("menuitem", { name: "Export CSV" }));

    expect(hrefSetter).toHaveBeenCalledOnce();
    const url = hrefSetter.mock.calls[0][0] as string;
    expect(url).toContain("format=csv");
    expect(url).toContain("/api/v1/scans/export");
  });

  it("clicking 'Export JSON' from the menu sets href with format=json", async () => {
    const user = userEvent.setup();
    render(
      <ExportButton basePath="/api/v1/hosts/export" params={{ sort_by: "ip" }} />,
    );

    await user.click(screen.getByRole("button", { name: "Export format picker" }));
    await user.click(screen.getByRole("menuitem", { name: "Export JSON" }));

    expect(hrefSetter).toHaveBeenCalledOnce();
    const url = hrefSetter.mock.calls[0][0] as string;
    expect(url).toContain("format=json");
  });

  it("closes the menu after selecting a format", async () => {
    const user = userEvent.setup();
    render(<ExportButton basePath="/api/v1/hosts/export" params={{}} />);

    await user.click(screen.getByRole("button", { name: "Export format picker" }));
    await user.click(screen.getByRole("menuitem", { name: "Export JSON" }));

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  // ── params filtering ───────────────────────────────────────────────────────

  it("omits undefined params from the URL", async () => {
    const user = userEvent.setup();
    render(
      <ExportButton
        basePath="/api/v1/hosts/export"
        params={{ sort_by: "last_seen", status: undefined }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Export CSV" }));

    const url = hrefSetter.mock.calls[0][0] as string;
    expect(url).not.toContain("status");
    expect(url).toContain("sort_by=last_seen");
  });
});
