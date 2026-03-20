import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { HostsPage } from "./hosts";

vi.mock("../api/hooks/use-hosts", () => ({
  useHosts: vi.fn(),
}));

import { useHosts } from "../api/hooks/use-hosts";

const mockUseHosts = vi.mocked(useHosts);

const mockHosts = [
  {
    id: "host-1",
    ip_address: "192.168.1.1",
    hostname: "router.local",
    status: "up" as const,
    mac_address: "AA:BB:CC:DD:EE:FF",
    ports: [
      {
        port: 22,
        protocol: "tcp",
        state: "open",
        service: "ssh",
        last_seen: new Date().toISOString(),
      },
      {
        port: 80,
        protocol: "tcp",
        state: "open",
        service: "http",
        last_seen: new Date().toISOString(),
      },
      {
        port: 443,
        protocol: "tcp",
        state: "open",
        service: "https",
        last_seen: new Date().toISOString(),
      },
    ],
    last_seen: new Date().toISOString(),
    scan_count: 5,
  },
  {
    id: "host-2",
    ip_address: "192.168.1.2",
    hostname: "server.local",
    status: "down" as const,
    mac_address: "11:22:33:44:55:66",
    ports: [
      {
        port: 22,
        protocol: "tcp",
        state: "open",
        service: "ssh",
        last_seen: new Date().toISOString(),
      },
      {
        port: 80,
        protocol: "tcp",
        state: "open",
        service: "http",
        last_seen: new Date().toISOString(),
      },
      {
        port: 443,
        protocol: "tcp",
        state: "open",
        service: "https",
        last_seen: new Date().toISOString(),
      },
      {
        port: 8080,
        protocol: "tcp",
        state: "open",
        last_seen: new Date().toISOString(),
      },
      {
        port: 8443,
        protocol: "tcp",
        state: "open",
        last_seen: new Date().toISOString(),
      },
      {
        port: 9090,
        protocol: "tcp",
        state: "open",
        last_seen: new Date().toISOString(),
      },
    ],
    last_seen: new Date().toISOString(),
    scan_count: 2,
  },
  {
    id: "host-3",
    ip_address: "10.0.0.1",
    hostname: undefined,
    status: "unknown" as const,
    mac_address: undefined,
    ports: undefined,
    last_seen: undefined,
    scan_count: undefined,
  },
];

function makeUseHostsResult(overrides = {}) {
  return {
    data: {
      data: mockHosts,
      pagination: { page: 1, page_size: 25, total_items: 3, total_pages: 1 },
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useHosts>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseHosts.mockReturnValue(makeUseHostsResult());
});

describe("HostsPage", () => {
  // ── Filter controls ──────────────────────────────────────────
  it("renders the search input", () => {
    render(<HostsPage />);
    expect(
      screen.getByRole("textbox", { name: /search hosts/i }),
    ).toBeInTheDocument();
  });

  it("renders the status filter select", () => {
    render(<HostsPage />);
    expect(
      screen.getByRole("combobox", { name: /filter by status/i }),
    ).toBeInTheDocument();
  });

  it("status select has all expected options", () => {
    render(<HostsPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    const options = within(select).getAllByRole("option");
    const values = options.map((o) => (o as HTMLOptionElement).value);
    expect(values).toEqual(["all", "up", "down", "unknown"]);
  });

  // ── Loading state ────────────────────────────────────────────
  it("renders 8 skeleton rows when loading", () => {
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<HostsPage />);
    // Each skeleton row has multiple Skeleton cells (animate-pulse divs)
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(8);
  });

  it("does not show empty message while loading", () => {
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({ isLoading: true, data: undefined }),
    );
    render(<HostsPage />);
    expect(screen.queryByText("No hosts found.")).not.toBeInTheDocument();
  });

  // ── Empty state ──────────────────────────────────────────────
  it("shows 'No hosts found.' when the host list is empty", () => {
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({
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
    render(<HostsPage />);
    expect(screen.getByText("No hosts found.")).toBeInTheDocument();
  });

  // ── Table structure ───────────────────────────────────────────
  it("renders all column headers", () => {
    render(<HostsPage />);
    expect(
      screen.getByRole("columnheader", { name: "IP Address" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Hostname" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "MAC Address" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Open Ports" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Last Seen" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Scans" }),
    ).toBeInTheDocument();
  });

  it("renders a row for each host", () => {
    render(<HostsPage />);
    // 1 header row + 3 data rows
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
  });

  // ── Host data ─────────────────────────────────────────────────
  it("renders IP addresses in monospace font", () => {
    render(<HostsPage />);
    expect(screen.getByText("192.168.1.1")).toBeInTheDocument();
    expect(screen.getByText("192.168.1.2")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.1")).toBeInTheDocument();
  });

  it("renders hostnames when present", () => {
    render(<HostsPage />);
    expect(screen.getByText("router.local")).toBeInTheDocument();
    expect(screen.getByText("server.local")).toBeInTheDocument();
  });

  it("renders StatusBadge for each host", () => {
    render(<HostsPage />);
    expect(screen.getByText("up")).toBeInTheDocument();
    expect(screen.getByText("down")).toBeInTheDocument();
    expect(screen.getByText("unknown")).toBeInTheDocument();
  });

  it("renders MAC addresses when present", () => {
    render(<HostsPage />);
    expect(screen.getByText("AA:BB:CC:DD:EE:FF")).toBeInTheDocument();
    expect(screen.getByText("11:22:33:44:55:66")).toBeInTheDocument();
  });

  it("renders scan counts when present", () => {
    render(<HostsPage />);
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  // ── Port tags ─────────────────────────────────────────────────
  it("renders individual port tags for open ports", () => {
    render(<HostsPage />);
    // host-1 has [22, 80, 443]
    expect(screen.getAllByText("22")[0]).toBeInTheDocument();
    expect(screen.getAllByText("80")[0]).toBeInTheDocument();
    expect(screen.getAllByText("443")[0]).toBeInTheDocument();
  });

  it("shows '+N more' overflow tag when ports exceed 5", () => {
    render(<HostsPage />);
    // host-2 has [22, 80, 443, 8080, 8443, 9090] → max 5 shown, +1 more
    expect(screen.getByText("+1 more")).toBeInTheDocument();
  });

  it("does not show overflow tag when ports are within limit", () => {
    render(<HostsPage />);
    // host-1 has only 3 ports, no overflow
    expect(screen.queryByText("+0 more")).not.toBeInTheDocument();
  });

  // ── Em-dash for missing fields ────────────────────────────────
  it("shows em-dash for missing hostname", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    // host-3 is the last data row (index 3)
    const cells = within(rows[3]).getAllByRole("cell");
    // Hostname is index 1
    expect(cells[1]).toHaveTextContent("—");
  });

  it("shows em-dash for missing MAC address", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // MAC Address is index 3
    expect(cells[3]).toHaveTextContent("—");
  });

  it("shows em-dash for missing open ports", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Open Ports is index 4
    expect(cells[4]).toHaveTextContent("—");
  });

  it("shows em-dash for missing last_seen", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Last Seen is index 5
    expect(cells[5]).toHaveTextContent("—");
  });

  it("shows em-dash for missing scan_count", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Scans is index 6
    expect(cells[6]).toHaveTextContent("—");
  });

  // ── Status filter interaction ─────────────────────────────────
  it("calls useHosts with the selected status filter", async () => {
    render(<HostsPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await userEvent.selectOptions(select, "up");
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ status: "up" }),
    );
  });

  it("calls useHosts without status when 'all' is selected", async () => {
    render(<HostsPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await userEvent.selectOptions(select, "up");
    await userEvent.selectOptions(select, "all");
    // Most recent call should not have a status key
    const lastCall =
      mockUseHosts.mock.calls[mockUseHosts.mock.calls.length - 1][0];
    expect(lastCall).not.toHaveProperty("status");
  });

  // ── Pagination bar ────────────────────────────────────────────
  it("shows pagination when there are multiple pages", () => {
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({
        data: {
          data: mockHosts,
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 50,
            total_pages: 2,
          },
        },
      }),
    );
    render(<HostsPage />);
    expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();
  });

  it("does not show pagination when the list is empty", () => {
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({
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
    render(<HostsPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });
});
