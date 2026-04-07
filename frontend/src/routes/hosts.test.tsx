import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { HostsPage } from "./hosts";

vi.mock("../api/hooks/use-hosts", () => ({
  useHosts: vi.fn(),
  useHost: vi.fn(),
  useHostScans: vi.fn(),
  useUpdateHost: vi.fn(),
  useDeleteHost: vi.fn(),
}));

import {
  useHosts,
  useHost,
  useHostScans,
  useUpdateHost,
  useDeleteHost,
} from "../api/hooks/use-hosts";

const mockUseHosts = vi.mocked(useHosts);
const mockUseHost = vi.mocked(useHost);
const mockUseHostScans = vi.mocked(useHostScans);
const mockUseUpdateHost = vi.mocked(useUpdateHost);
const mockUseDeleteHost = vi.mocked(useDeleteHost);

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
vi.mock("../components/toast-provider", () => ({
  useToast: () => ({
    toast: { success: mockToastSuccess, error: mockToastError },
  }),
}));

vi.mock("../components", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../components")>();
  return {
    ...actual,
    RunScanModal: ({ onClose }: { onClose: () => void }) => (
      <div data-testid="run-scan-modal">
        <button onClick={onClose}>Close scan modal</button>
      </div>
    ),
  };
});

const mockHosts = [
  {
    id: "host-1",
    ip_address: "192.168.1.1",
    hostname: "router.local",
    status: "up" as const,
    mac_address: "AA:BB:CC:DD:EE:FF",
    os_family: "Linux",
    os_name: "Linux 5.4",
    total_ports: 3,
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
    os_family: undefined,
    os_name: undefined,
    total_ports: undefined,
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

const mockFullHost = {
  id: "host-1",
  ip_address: "192.168.1.1",
  hostname: "router.local",
  status: "up" as const,
  mac_address: "AA:BB:CC:DD:EE:FF",
  os_family: "Linux",
  os_name: "Linux 5.4",
  os_version_detail: "5.4",
  os_confidence: 95,
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
      state: "closed",
      service: "https",
      last_seen: new Date().toISOString(),
    },
  ],
  first_seen: new Date().toISOString(),
  last_seen: new Date().toISOString(),
  scan_count: 5,
};

function makeUseHostResult(overrides = {}) {
  return {
    data: mockFullHost,
    isLoading: false,
    isError: false,
    error: null,
    ...overrides,
  } as unknown as ReturnType<typeof useHost>;
}

function makeUseHostScansResult(overrides = {}) {
  return {
    data: {
      data: [
        {
          id: "scan-1",
          name: "quick-scan",
          scan_type: "connect",
          status: "completed",
          targets: ["192.168.1.1"],
          started_at: new Date().toISOString(),
          created_at: new Date().toISOString(),
        },
      ],
      pagination: { page: 1, page_size: 5, total_items: 1, total_pages: 1 },
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useHostScans>;
}

function makeMutationResult(overrides = {}) {
  return {
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
    ...overrides,
  } as unknown as ReturnType<typeof useUpdateHost>;
}

function makeDeleteMutationResult(overrides = {}) {
  return {
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
    ...overrides,
  } as unknown as ReturnType<typeof useDeleteHost>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseHosts.mockReturnValue(makeUseHostsResult());
  mockUseHost.mockReturnValue(makeUseHostResult());
  mockUseHostScans.mockReturnValue(makeUseHostScansResult());
  mockUseUpdateHost.mockReturnValue(makeMutationResult());
  mockUseDeleteHost.mockReturnValue(makeDeleteMutationResult());
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
    expect(
      screen.getByRole("columnheader", { name: "OS" }),
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

  // ── Open port count ───────────────────────────────────────────
  // Column order: IP=0, Hostname=1, Status=2, OS=3, MAC=4,
  //               Vendor=5, Open Ports=6, Last Seen=7, Scans=8, action=9

  it("shows open port count when total_ports is set", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    // host-1 has total_ports: 3 — Open Ports column is index 6
    const cells = within(rows[1]).getAllByRole("cell");
    expect(cells[6]).toHaveTextContent("3");
  });

  it("shows em-dash for open ports when total_ports is missing", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    // host-3 has no total_ports
    const cells = within(rows[3]).getAllByRole("cell");
    expect(cells[6]).toHaveTextContent("—");
  });

  // ── OS column ─────────────────────────────────────────────────
  // Column order: IP=0, Hostname=1, Status=2, OS=3, MAC=4,
  //               Vendor=5, Open Ports=6, Last Seen=7, Scans=8, action=9

  it("renders os_family when present", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    // host-1 has os_family "Linux"
    expect(within(rows[1]).getByText("Linux")).toBeInTheDocument();
  });

  it("shows em-dash for missing os_family", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // host-3 has no os_family — OS column is index 3
    expect(cells[3]).toHaveTextContent("—");
  });

  // ── Em-dash for missing fields ────────────────────────────────

  it("shows em-dash for missing hostname", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Hostname is index 1
    expect(cells[1]).toHaveTextContent("—");
  });

  it("shows em-dash for missing MAC address", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // MAC Address is index 4
    expect(cells[4]).toHaveTextContent("—");
  });

  it("shows em-dash for missing open ports", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Open Ports is index 6
    expect(cells[6]).toHaveTextContent("—");
  });

  it("shows em-dash for missing last_seen", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Last Seen is index 7
    expect(cells[7]).toHaveTextContent("—");
  });

  it("shows em-dash for missing scan_count", () => {
    render(<HostsPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    // Scans is index 8
    expect(cells[8]).toHaveTextContent("—");
  });

  // ── Sort interaction ──────────────────────────────────────────

  it("calls useHosts with default sort (last_seen desc)", () => {
    render(<HostsPage />);
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: "last_seen", sort_order: "desc" }),
    );
  });

  it("calls useHosts with updated sort_by when a sortable header is clicked", async () => {
    render(<HostsPage />);
    const ipHeader = screen.getByRole("columnheader", { name: "IP Address" });
    await userEvent.click(ipHeader);
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: "ip_address", sort_order: "desc" }),
    );
  });

  it("toggles sort_order when the active sort column is clicked again", async () => {
    render(<HostsPage />);
    const ipHeader = screen.getByRole("columnheader", { name: "IP Address" });
    await userEvent.click(ipHeader);
    await userEvent.click(ipHeader);
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: "ip_address", sort_order: "asc" }),
    );
  });

  it("calls useHosts with sort_by open_ports when Open Ports header is clicked", async () => {
    render(<HostsPage />);
    await userEvent.click(
      screen.getByRole("columnheader", { name: "Open Ports" }),
    );
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: "open_ports", sort_order: "desc" }),
    );
  });

  it("calls useHosts with sort_by scan_count when Scans header is clicked", async () => {
    render(<HostsPage />);
    await userEvent.click(screen.getByRole("columnheader", { name: "Scans" }));
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: "scan_count", sort_order: "desc" }),
    );
  });

  // ── OS filter interaction ─────────────────────────────────────

  it("renders the OS filter select", () => {
    render(<HostsPage />);
    expect(
      screen.getByRole("combobox", { name: /filter by os/i }),
    ).toBeInTheDocument();
  });

  it("calls useHosts with os filter when an OS family is selected", async () => {
    render(<HostsPage />);
    const osSelect = screen.getByRole("combobox", { name: /filter by os/i });
    await userEvent.selectOptions(osSelect, "Linux");
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ os: "Linux" }),
    );
  });

  it("does not pass os param when 'All OS' is selected", async () => {
    render(<HostsPage />);
    const osSelect = screen.getByRole("combobox", { name: /filter by os/i });
    await userEvent.selectOptions(osSelect, "Linux");
    await userEvent.selectOptions(osSelect, "");
    const lastCall =
      mockUseHosts.mock.calls[mockUseHosts.mock.calls.length - 1][0];
    expect(lastCall).not.toHaveProperty("os");
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

  // ── Scan modal ────────────────────────────────────────────────

  it("opens the scan modal when 'New scan' is clicked", async () => {
    render(<HostsPage />);
    await userEvent.click(screen.getByRole("button", { name: /new scan/i }));
    expect(screen.getByTestId("run-scan-modal")).toBeInTheDocument();
  });

  it("closes the scan modal when it calls onClose", async () => {
    render(<HostsPage />);
    await userEvent.click(screen.getByRole("button", { name: /new scan/i }));
    await userEvent.click(
      screen.getByRole("button", { name: /close scan modal/i }),
    );
    expect(screen.queryByTestId("run-scan-modal")).not.toBeInTheDocument();
  });

  it("opens the scan modal pre-filled when the per-row Scan button is clicked", async () => {
    render(<HostsPage />);
    const scanButton = screen.getByRole("button", {
      name: /scan 192\.168\.1\.1/i,
    });
    await userEvent.click(scanButton);
    expect(screen.getByTestId("run-scan-modal")).toBeInTheDocument();
  });

  it("does not open the detail panel when the per-row Scan button is clicked", async () => {
    render(<HostsPage />);
    const scanButton = screen.getByRole("button", {
      name: /scan 192\.168\.1\.1/i,
    });
    await userEvent.click(scanButton);
    expect(
      screen.queryByRole("dialog", { name: /host details/i }),
    ).not.toBeInTheDocument();
  });

  // ── Pagination controls ───────────────────────────────────────

  it("advances to the next page when Next is clicked", async () => {
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
    await userEvent.click(screen.getByRole("button", { name: /next/i }));
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2 }),
    );
  });

  it("goes back to the previous page when Prev is clicked", async () => {
    // Start the mock on page 2 so Prev is enabled
    mockUseHosts.mockReturnValue(
      makeUseHostsResult({
        data: {
          data: mockHosts,
          pagination: {
            page: 2,
            page_size: 25,
            total_items: 50,
            total_pages: 2,
          },
        },
      }),
    );
    render(<HostsPage />);
    await userEvent.click(screen.getByRole("button", { name: /next/i }));
    await userEvent.click(screen.getByRole("button", { name: /prev/i }));
    expect(mockUseHosts).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1 }),
    );
  });

  // ── HostDetailPanel ───────────────────────────────────────────

  describe("HostDetailPanel", () => {
    async function openPanel() {
      render(<HostsPage />);
      const rows = screen.getAllByRole("row");
      await userEvent.click(rows[1]); // host-1
      return screen.getByRole("dialog", { name: /host details/i });
    }

    it("opens when a table row is clicked", async () => {
      const panel = await openPanel();
      expect(panel).toBeInTheDocument();
    });

    it("shows the host IP address in the panel header", async () => {
      const panel = await openPanel();
      // The IP appears in multiple places inside the panel (header, Identity row,
      // scan-history targets); getAllByText avoids the "multiple elements" error.
      expect(within(panel).getAllByText("192.168.1.1").length).toBeGreaterThan(
        0,
      );
    });

    it("closes when the close button is clicked", async () => {
      await openPanel();
      await userEvent.click(
        screen.getByRole("button", { name: /close panel/i }),
      );
      expect(
        screen.queryByRole("dialog", { name: /host details/i }),
      ).not.toBeInTheDocument();
    });

    it("closes when the backdrop is clicked", async () => {
      render(<HostsPage />);
      await userEvent.click(screen.getAllByRole("row")[1]);
      // Target the backdrop div specifically (SVG icons also carry aria-hidden)
      const backdrop = document.querySelector(
        "div[aria-hidden='true']",
      ) as Element;
      await userEvent.click(backdrop);
      expect(
        screen.queryByRole("dialog", { name: /host details/i }),
      ).not.toBeInTheDocument();
    });

    it("displays OS family and name from the full host response", async () => {
      const panel = await openPanel();
      expect(within(panel).getByText("Linux")).toBeInTheDocument();
      expect(within(panel).getByText("Linux 5.4")).toBeInTheDocument();
    });

    it("hides the OS section when no OS data is present", async () => {
      mockUseHost.mockReturnValue(
        makeUseHostResult({
          data: {
            ...mockFullHost,
            os_family: undefined,
            os_name: undefined,
            os_version_detail: undefined,
          } as unknown as typeof mockFullHost,
        }),
      );
      const panel = await openPanel();
      expect(within(panel).queryByText("OS Detection")).not.toBeInTheDocument();
    });

    it("shows open ports from the full host response", async () => {
      const panel = await openPanel();
      // port 22 is open
      expect(within(panel).getByText("22")).toBeInTheDocument();
      expect(within(panel).getByText("ssh")).toBeInTheDocument();
    });

    it("shows closed ports in the closed/filtered section", async () => {
      const panel = await openPanel();
      // port 443 has state: "closed"
      expect(
        within(panel).getByText(/closed \/ filtered/i),
      ).toBeInTheDocument();
      expect(within(panel).getByText("443")).toBeInTheDocument();
    });

    it("shows scan history entries", async () => {
      const panel = await openPanel();
      // The scan history renders the scan's status badge ("completed") which is
      // unique in the panel — the host's own status badge shows "up".
      expect(within(panel).getByText("completed")).toBeInTheDocument();
    });

    it("advances to the next scan history page when Next is clicked", async () => {
      mockUseHostScans.mockReturnValue({
        data: {
          data: [
            {
              id: "scan-1",
              name: "scan-one",
              scan_type: "connect",
              status: "completed",
              targets: ["192.168.1.1"],
              started_at: new Date().toISOString(),
              created_at: new Date().toISOString(),
            },
          ],
          pagination: {
            page: 1,
            page_size: 5,
            total_items: 10,
            total_pages: 2,
          },
        },
        isLoading: false,
      } as unknown as ReturnType<typeof useHostScans>);

      const panel = await openPanel();
      const nextBtn = within(panel).getByRole("button", { name: /next/i });
      await userEvent.click(nextBtn);

      expect(mockUseHostScans).toHaveBeenCalledWith(
        "host-1",
        expect.objectContaining({ page: 2 }),
      );
    });

    it("hides scan history pagination when there is only one page", async () => {
      // Default makeUseHostScansResult returns total_pages: 1
      const panel = await openPanel();
      // Pagination buttons should not be rendered
      expect(
        within(panel).queryByRole("button", { name: /← prev/i }),
      ).not.toBeInTheDocument();
    });

    // ── Inline hostname editing ───────────────────────────────────

    it("shows a pencil button to edit the hostname", async () => {
      const panel = await openPanel();
      expect(
        within(panel).getByRole("button", { name: /edit hostname/i }),
      ).toBeInTheDocument();
    });

    it("opens the hostname input when the pencil is clicked", async () => {
      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /edit hostname/i }),
      );
      expect(within(panel).getByRole("textbox")).toBeInTheDocument();
    });

    it("calls updateHost with the new hostname when saved", async () => {
      const mutateAsync = vi.fn().mockResolvedValue({});
      mockUseUpdateHost.mockReturnValue(makeMutationResult({ mutateAsync }));

      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /edit hostname/i }),
      );
      const input = within(panel).getByRole("textbox");
      await userEvent.clear(input);
      await userEvent.type(input, "new-hostname");
      await userEvent.click(
        within(panel).getByRole("button", { name: /save hostname/i }),
      );

      expect(mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          body: expect.objectContaining({ hostname: "new-hostname" }),
        }),
      );
    });

    it("closes the hostname input without saving when cancelled", async () => {
      const mutateAsync = vi.fn();
      mockUseUpdateHost.mockReturnValue(makeMutationResult({ mutateAsync }));

      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /edit hostname/i }),
      );
      await userEvent.click(
        within(panel).getByRole("button", { name: /cancel/i }),
      );

      expect(mutateAsync).not.toHaveBeenCalled();
      expect(within(panel).queryByRole("textbox")).not.toBeInTheDocument();
    });

    // ── Delete flow ───────────────────────────────────────────────

    it("shows a delete button in the panel footer", async () => {
      const panel = await openPanel();
      // The initial delete trigger says "Delete host"
      expect(
        within(panel).getByRole("button", { name: /delete host/i }),
      ).toBeInTheDocument();
    });

    it("shows a confirmation prompt when delete is clicked", async () => {
      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /delete host/i }),
      );
      expect(
        within(panel).getByText(/permanently delete/i),
      ).toBeInTheDocument();
    });

    it("cancels delete when Cancel is clicked", async () => {
      const mutateAsync = vi.fn();
      mockUseDeleteHost.mockReturnValue(
        makeDeleteMutationResult({ mutateAsync }),
      );

      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /delete host/i }),
      );
      await userEvent.click(
        within(panel).getByRole("button", { name: /cancel/i }),
      );

      expect(mutateAsync).not.toHaveBeenCalled();
      expect(
        within(panel).queryByText(/permanently delete/i),
      ).not.toBeInTheDocument();
    });

    it("calls deleteHost and closes the panel on confirmation", async () => {
      const mutateAsync = vi.fn().mockResolvedValue({});
      mockUseDeleteHost.mockReturnValue(
        makeDeleteMutationResult({ mutateAsync }),
      );

      const panel = await openPanel();
      await userEvent.click(
        within(panel).getByRole("button", { name: /delete host/i }),
      );
      // The confirm button is a danger-variant Button labelled "Confirm"
      await userEvent.click(
        within(panel).getByRole("button", { name: /confirm/i }),
      );

      expect(mutateAsync).toHaveBeenCalledWith("host-1");
    });

    // ── Scan this host button ─────────────────────────────────────

    it("opens the scan modal when 'Scan this host' is clicked", async () => {
      await openPanel();
      await userEvent.click(
        screen.getByRole("button", { name: /scan this host/i }),
      );
      expect(screen.getByTestId("run-scan-modal")).toBeInTheDocument();
    });
  });
});
