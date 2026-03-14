import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ScansPage } from "./scans";

vi.mock("../api/hooks/use-scans", () => ({
  useScans: vi.fn(),
  useScanResults: vi.fn(),
}));

import { useScans, useScanResults } from "../api/hooks/use-scans";

const mockUseScans = vi.mocked(useScans);
const mockUseScanResults = vi.mocked(useScanResults);

const mockScans = [
  {
    id: "scan-1",
    status: "completed" as const,
    targets: ["192.168.1.0/24"],
    hosts_discovered: 25,
    ports_scanned: 2500,
    duration: "14m30s",
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    completed_at: new Date().toISOString(),
    profile_id: "profile-abc",
    error_message: undefined,
  },
  {
    id: "scan-2",
    status: "running" as const,
    targets: ["10.0.0.0/8", "172.16.0.0/12"],
    hosts_discovered: 10,
    ports_scanned: undefined,
    duration: undefined,
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    completed_at: undefined,
    profile_id: undefined,
    error_message: undefined,
  },
  {
    id: "scan-3",
    status: "failed" as const,
    targets: undefined,
    hosts_discovered: undefined,
    ports_scanned: undefined,
    duration: undefined,
    started_at: undefined,
    created_at: new Date().toISOString(),
    completed_at: undefined,
    profile_id: undefined,
    error_message: "Connection refused",
  },
];

const mockResultsData = {
  scan_id: "scan-1",
  total_hosts: 2,
  total_ports: 4,
  open_ports: 4,
  closed_ports: 0,
  generated_at: new Date().toISOString(),
  results: [
    {
      id: "r-1",
      host_ip: "192.168.1.1",
      hostname: "router.local",
      port: 80,
      protocol: "tcp",
      state: "open",
      service: "http",
    },
    {
      id: "r-2",
      host_ip: "192.168.1.2",
      hostname: undefined,
      port: 443,
      protocol: "tcp",
      state: "open",
      service: "https",
    },
  ],
  summary: {
    scan_id: "scan-1",
    total_hosts: 2,
    total_ports: 4,
    open_ports: 4,
    closed_ports: 0,
    duration: "14m30s",
  },
};

function makeUseScansResult(overrides = {}) {
  return {
    data: {
      data: mockScans,
      pagination: { page: 1, page_size: 25, total_items: 3, total_pages: 1 },
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useScans>;
}

function makeUseScanResultsResult(overrides = {}) {
  return {
    data: mockResultsData,
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useScanResults>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseScans.mockReturnValue(makeUseScansResult());
  mockUseScanResults.mockReturnValue(makeUseScanResultsResult());
});

describe("ScansPage", () => {
  // ── Filter controls ──────────────────────────────────────────
  it("renders the status filter select", () => {
    render(<ScansPage />);
    expect(
      screen.getByRole("combobox", { name: /filter by status/i }),
    ).toBeInTheDocument();
  });

  it("status select has all expected options", () => {
    render(<ScansPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    const options = within(select).getAllByRole("option");
    const values = options.map((o) => (o as HTMLOptionElement).value);
    expect(values).toEqual([
      "all",
      "pending",
      "running",
      "completed",
      "failed",
      "cancelled",
    ]);
  });

  // ── Loading state ────────────────────────────────────────────
  it("renders skeleton rows when loading", () => {
    mockUseScans.mockReturnValue(
      makeUseScansResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<ScansPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(8);
  });

  it("does not show empty message while loading", () => {
    mockUseScans.mockReturnValue(
      makeUseScansResult({ isLoading: true, data: undefined }),
    );
    render(<ScansPage />);
    expect(screen.queryByText("No scans found.")).not.toBeInTheDocument();
  });

  // ── Empty state ──────────────────────────────────────────────
  it("shows 'No scans found.' when the scan list is empty", () => {
    mockUseScans.mockReturnValue(
      makeUseScansResult({
        data: {
          data: [],
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 0,
            total_pages: 0,
          },
        },
      }),
    );
    render(<ScansPage />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  // ── Table structure ───────────────────────────────────────────
  it("renders all column headers", () => {
    render(<ScansPage />);
    expect(
      screen.getByRole("columnheader", { name: "Targets" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Hosts" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Ports" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Duration" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Started" }),
    ).toBeInTheDocument();
  });

  it("renders a row for each scan", () => {
    render(<ScansPage />);
    // 1 header row + 3 data rows
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
  });

  // ── Scan data ─────────────────────────────────────────────────
  it("renders scan targets", () => {
    render(<ScansPage />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.0/8, 172.16.0.0/12")).toBeInTheDocument();
  });

  it("renders StatusBadge for each scan", () => {
    render(<ScansPage />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
  });

  it("renders numeric hosts_discovered and ports_scanned", () => {
    render(<ScansPage />);
    expect(screen.getByText("25")).toBeInTheDocument();
    expect(screen.getByText("2500")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
  });

  it("renders duration when present", () => {
    render(<ScansPage />);
    expect(screen.getByText("14m30s")).toBeInTheDocument();
  });

  // ── Em-dash for missing fields ────────────────────────────────
  it("shows em-dash for missing targets", () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // scan-3 is the last data row (index 3)
    const cells = within(rows[3]).getAllByRole("cell");
    // Targets is index 0
    expect(cells[0]).toHaveTextContent("—");
  });

  it("shows em-dash for missing ports_scanned", () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // scan-2 is index 2
    const cells = within(rows[2]).getAllByRole("cell");
    // Ports is index 3
    expect(cells[3]).toHaveTextContent("—");
  });

  it("shows em-dash for missing duration", () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // scan-2 is index 2
    const cells = within(rows[2]).getAllByRole("cell");
    // Duration is index 4
    expect(cells[4]).toHaveTextContent("—");
  });

  it("shows em-dash for missing started_at", () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // scan-3 is index 3 — no started_at
    const cells = within(rows[3]).getAllByRole("cell");
    // Started is index 5
    expect(cells[5]).toHaveTextContent("—");
  });

  // ── Status filter interaction ─────────────────────────────────
  it("calls useScans with the selected status filter", async () => {
    render(<ScansPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await userEvent.selectOptions(select, "running");
    expect(mockUseScans).toHaveBeenCalledWith(
      expect.objectContaining({ status: "running" }),
    );
  });

  it("calls useScans without status when 'all' is selected", async () => {
    render(<ScansPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await userEvent.selectOptions(select, "completed");
    await userEvent.selectOptions(select, "all");
    const lastCall =
      mockUseScans.mock.calls[mockUseScans.mock.calls.length - 1][0];
    expect(lastCall).not.toHaveProperty("status");
  });

  // ── Detail panel: open on row click ──────────────────────────
  it("opens the detail panel when a scan row is clicked", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // Click scan-1 row (index 1)
    await userEvent.click(rows[1]);
    expect(screen.getByRole("dialog", { name: /scan details/i })).toBeInTheDocument();
  });

  it("shows the scan targets in the detail panel header", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  it("shows the scan status badge in the detail panel", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    // completed appears in the table AND in the panel header badge
    const badges = within(dialog).getAllByText("completed");
    expect(badges.length).toBeGreaterThanOrEqual(1);
  });

  it("shows the scan ID in the detail panel metadata", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("scan-1")).toBeInTheDocument();
  });

  it("shows the profile ID in the detail panel when present", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("profile-abc")).toBeInTheDocument();
  });

  it("shows the error message in the detail panel when present", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    // scan-3 has error_message
    await userEvent.click(rows[3]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("Connection refused")).toBeInTheDocument();
  });

  // ── Detail panel: results ─────────────────────────────────────
  it("shows scan results in the detail panel", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("192.168.1.1")).toBeInTheDocument();
    expect(within(dialog).getByText("router.local")).toBeInTheDocument();
  });

  it("shows loading skeleton in panel while results are loading", async () => {
    mockUseScanResults.mockReturnValue(
      makeUseScanResultsResult({ isLoading: true, data: undefined }),
    );
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    const skeletons = dialog.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows 'No results found.' when results array is empty", async () => {
    mockUseScanResults.mockReturnValue(
      makeUseScanResultsResult({
        data: { ...mockResultsData, results: [] },
      }),
    );
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", { name: /scan details/i });
    expect(within(dialog).getByText("No results found.")).toBeInTheDocument();
  });

  // ── Detail panel: close ───────────────────────────────────────
  it("closes the detail panel when the close button is clicked", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    expect(screen.getByRole("dialog", { name: /scan details/i })).toBeInTheDocument();

    const closeButton = screen.getByRole("button", { name: /close panel/i });
    await userEvent.click(closeButton);
    expect(
      screen.queryByRole("dialog", { name: /scan details/i }),
    ).not.toBeInTheDocument();
  });

  it("closes the detail panel when the backdrop is clicked", async () => {
    render(<ScansPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    expect(screen.getByRole("dialog", { name: /scan details/i })).toBeInTheDocument();

    // The backdrop is the fixed overlay behind the dialog
    const backdrop = document.querySelector(".fixed.inset-0.bg-black\\/40");
    expect(backdrop).not.toBeNull();
    await userEvent.click(backdrop!);
    expect(
      screen.queryByRole("dialog", { name: /scan details/i }),
    ).not.toBeInTheDocument();
  });

  // ── Pagination bar ────────────────────────────────────────────
  it("shows pagination when there are multiple pages", () => {
    mockUseScans.mockReturnValue(
      makeUseScansResult({
        data: {
          data: mockScans,
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 50,
            total_pages: 2,
          },
        },
      }),
    );
    render(<ScansPage />);
    expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();
  });

  it("does not show pagination when the scan list is empty", () => {
    mockUseScans.mockReturnValue(
      makeUseScansResult({
        data: {
          data: [],
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 0,
            total_pages: 0,
          },
        },
      }),
    );
    render(<ScansPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });
});
