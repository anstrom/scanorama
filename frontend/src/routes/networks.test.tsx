import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { NetworksPage } from "./networks";

// ── Mock hooks ────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-networks", () => ({
  useNetworks: vi.fn(),
  useNetworkExclusions: vi.fn(),
  useEnableNetwork: vi.fn(),
  useDisableNetwork: vi.fn(),
  useRenameNetwork: vi.fn(),
  useDeleteNetwork: vi.fn(),
  useDeleteExclusion: vi.fn(),
}));

vi.mock("../components/add-exclusion-modal", () => ({
  AddExclusionModal: ({
    onClose,
  }: {
    networkId?: string;
    onClose: () => void;
    onCreated?: () => void;
  }) => (
    <div role="dialog" aria-label="Add Exclusion">
      <button type="button" onClick={onClose}>
        Close exclusion modal
      </button>
    </div>
  ),
}));

import {
  useNetworks,
  useNetworkExclusions,
  useEnableNetwork,
  useDisableNetwork,
  useRenameNetwork,
  useDeleteNetwork,
  useDeleteExclusion,
} from "../api/hooks/use-networks";

const mockUseNetworks = vi.mocked(useNetworks);
const mockUseNetworkExclusions = vi.mocked(useNetworkExclusions);
const mockUseEnableNetwork = vi.mocked(useEnableNetwork);
const mockUseDisableNetwork = vi.mocked(useDisableNetwork);
const mockUseRenameNetwork = vi.mocked(useRenameNetwork);
const mockUseDeleteNetwork = vi.mocked(useDeleteNetwork);
const mockUseDeleteExclusion = vi.mocked(useDeleteExclusion);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockNetworks = [
  {
    id: "net-1",
    name: "Office LAN",
    cidr: "192.168.1.0/24",
    is_active: true,
    host_count: 25,
    active_host_count: 20,
    discovery_method: "ping",
    scan_enabled: true,
    description: "Main office network",
    created_by: "admin",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-06-01T00:00:00Z",
    last_discovery: new Date(Date.now() - 3_600_000).toISOString(), // 1h ago
    last_scan: undefined,
  },
  {
    id: "net-2",
    name: "DMZ",
    cidr: "10.0.0.0/8",
    is_active: false,
    host_count: 5,
    active_host_count: 2,
    discovery_method: "tcp",
    scan_enabled: false,
    description: undefined,
    created_by: "admin",
    created_at: "2024-02-01T00:00:00Z",
    updated_at: "2024-06-02T00:00:00Z",
    last_discovery: undefined,
    last_scan: undefined,
  },
  {
    id: "net-3",
    name: "Wireless",
    cidr: "172.16.0.0/16",
    is_active: true,
    host_count: undefined,
    active_host_count: undefined,
    discovery_method: "arp",
    scan_enabled: true,
    description: undefined,
    created_by: undefined,
    created_at: "2024-03-01T00:00:00Z",
    updated_at: undefined,
    last_discovery: undefined,
    last_scan: undefined,
  },
];

const mockPagination = {
  page: 1,
  page_size: 25,
  total_items: 3,
  total_pages: 1,
};

const idleMutation = {
  mutateAsync: vi.fn(),
  mutate: vi.fn(),
  isPending: false,
  isSuccess: false,
  isError: false,
};

function makeUseNetworksResult(overrides = {}) {
  return {
    data: {
      data: mockNetworks,
      pagination: mockPagination,
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useNetworks>;
}

function makeUseNetworkExclusionsResult(overrides = {}) {
  return {
    data: [],
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useNetworkExclusions>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseNetworks.mockReturnValue(makeUseNetworksResult());
  mockUseNetworkExclusions.mockReturnValue(makeUseNetworkExclusionsResult());
  mockUseEnableNetwork.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useEnableNetwork>,
  );
  mockUseDisableNetwork.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDisableNetwork>,
  );
  mockUseRenameNetwork.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useRenameNetwork>,
  );
  mockUseDeleteNetwork.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDeleteNetwork>,
  );
  mockUseDeleteExclusion.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDeleteExclusion>,
  );
});

// ── Filter controls ───────────────────────────────────────────────────────────

describe("NetworksPage — toolbar", () => {
  it("renders the name search input", () => {
    render(<NetworksPage />);
    expect(
      screen.getByRole("textbox", { name: /search networks/i }),
    ).toBeInTheDocument();
  });

  it("renders the show inactive checkbox", () => {
    render(<NetworksPage />);
    expect(
      screen.getByRole("checkbox", { name: /show inactive/i }),
    ).toBeInTheDocument();
  });

  it("show inactive checkbox is unchecked by default", () => {
    render(<NetworksPage />);
    const checkbox = screen.getByRole("checkbox", { name: /show inactive/i });
    expect(checkbox).not.toBeChecked();
  });

  it("renders the Add Network button", () => {
    render(<NetworksPage />);
    expect(
      screen.getByRole("button", { name: /add network/i }),
    ).toBeInTheDocument();
  });

  it("passes show_inactive to useNetworks when checkbox is toggled", async () => {
    render(<NetworksPage />);
    const checkbox = screen.getByRole("checkbox", { name: /show inactive/i });
    await userEvent.click(checkbox);
    const lastCall =
      mockUseNetworks.mock.calls[mockUseNetworks.mock.calls.length - 1][0];
    expect(lastCall).toHaveProperty("show_inactive", true);
  });

  it("removes show_inactive from params when checkbox is untoggled", async () => {
    render(<NetworksPage />);
    const checkbox = screen.getByRole("checkbox", { name: /show inactive/i });
    await userEvent.click(checkbox); // check
    await userEvent.click(checkbox); // uncheck
    const lastCall =
      mockUseNetworks.mock.calls[mockUseNetworks.mock.calls.length - 1][0];
    expect(lastCall).not.toHaveProperty("show_inactive");
  });
});

// ── Loading state ─────────────────────────────────────────────────────────────

describe("NetworksPage — loading", () => {
  it("renders 8 skeleton rows when loading", () => {
    mockUseNetworks.mockReturnValue(
      makeUseNetworksResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<NetworksPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(8);
  });

  it("does not show 'No networks found.' while loading", () => {
    mockUseNetworks.mockReturnValue(
      makeUseNetworksResult({ isLoading: true, data: undefined }),
    );
    render(<NetworksPage />);
    expect(screen.queryByText("No networks found.")).not.toBeInTheDocument();
  });
});

// ── Empty state ───────────────────────────────────────────────────────────────

describe("NetworksPage — empty state", () => {
  it("shows 'No networks found.' when the list is empty", () => {
    mockUseNetworks.mockReturnValue(
      makeUseNetworksResult({
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
    render(<NetworksPage />);
    expect(screen.getByText("No networks found.")).toBeInTheDocument();
  });
});

// ── Table structure ───────────────────────────────────────────────────────────

describe("NetworksPage — table structure", () => {
  it("renders all column headers", () => {
    render(<NetworksPage />);
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "CIDR" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Hosts" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Active" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Discovery" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Last Discovery" }),
    ).toBeInTheDocument();
  });

  it("renders one data row per network", () => {
    render(<NetworksPage />);
    // 1 header row + 3 data rows
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
  });
});

// ── Network data ──────────────────────────────────────────────────────────────

describe("NetworksPage — row data", () => {
  it("renders network names", () => {
    render(<NetworksPage />);
    expect(screen.getByText("Office LAN")).toBeInTheDocument();
    expect(screen.getByText("DMZ")).toBeInTheDocument();
    expect(screen.getByText("Wireless")).toBeInTheDocument();
  });

  it("renders CIDR blocks in monospace", () => {
    render(<NetworksPage />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.0/8")).toBeInTheDocument();
    expect(screen.getByText("172.16.0.0/16")).toBeInTheDocument();
  });

  it("renders host counts", () => {
    render(<NetworksPage />);
    expect(screen.getByText("25")).toBeInTheDocument(); // host_count for Office LAN
    expect(screen.getByText("5")).toBeInTheDocument(); // host_count for DMZ
  });

  it("renders active host counts", () => {
    render(<NetworksPage />);
    expect(screen.getByText("20")).toBeInTheDocument(); // active_host_count for Office LAN
    expect(screen.getByText("2")).toBeInTheDocument(); // active_host_count for DMZ
  });

  it("renders discovery method labels", () => {
    render(<NetworksPage />);
    expect(screen.getByText("Ping")).toBeInTheDocument();
    expect(screen.getByText("TCP")).toBeInTheDocument();
    expect(screen.getByText("ARP")).toBeInTheDocument();
  });

  it("renders 'active' badge for active networks", () => {
    render(<NetworksPage />);
    const badges = screen.getAllByText("active");
    expect(badges.length).toBeGreaterThanOrEqual(1);
  });

  it("renders 'inactive' badge for inactive networks", () => {
    render(<NetworksPage />);
    expect(screen.getByText("inactive")).toBeInTheDocument();
  });

  it("shows em-dash for missing host_count", () => {
    render(<NetworksPage />);
    const rows = screen.getAllByRole("row");
    // Wireless (net-3) is the 3rd data row (index 3)
    const cells = within(rows[3]).getAllByRole("cell");
    // Hosts is index 2
    expect(cells[2]).toHaveTextContent("—");
  });

  it("shows em-dash for missing last_discovery", () => {
    render(<NetworksPage />);
    const rows = screen.getAllByRole("row");
    // DMZ (net-2) is the 2nd data row (index 2), has no last_discovery
    const cells = within(rows[2]).getAllByRole("cell");
    // Last Discovery is index 6
    expect(cells[6]).toHaveTextContent("—");
  });

  it("shows relative time for last_discovery when present", () => {
    render(<NetworksPage />);
    const rows = screen.getAllByRole("row");
    // Office LAN (net-1) is the 1st data row (index 1), has last_discovery ~1h ago
    const cells = within(rows[1]).getAllByRole("cell");
    // Should contain "ago" or "just now"
    expect(cells[6].textContent).toMatch(/ago|just now/i);
  });
});

// ── Pagination ────────────────────────────────────────────────────────────────

describe("NetworksPage — pagination", () => {
  it("shows pagination when there are multiple pages", () => {
    mockUseNetworks.mockReturnValue(
      makeUseNetworksResult({
        data: {
          data: mockNetworks,
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 60,
            total_pages: 3,
          },
        },
      }),
    );
    render(<NetworksPage />);
    expect(screen.getByText("Page 1 of 3")).toBeInTheDocument();
  });

  it("does not show pagination when there is only one page", () => {
    render(<NetworksPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });

  it("does not show pagination when the list is empty", () => {
    mockUseNetworks.mockReturnValue(
      makeUseNetworksResult({
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
    render(<NetworksPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });
});

// ── Detail panel ──────────────────────────────────────────────────────────────

describe("NetworksPage — detail panel", () => {
  it("opens the detail panel when a row is clicked", async () => {
    render(<NetworksPage />);
    const row = screen.getByText("Office LAN").closest("tr")!;
    await userEvent.click(row);
    expect(
      screen.getByRole("dialog", { name: /network details/i }),
    ).toBeInTheDocument();
  });

  it("shows the network CIDR in the detail panel header", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    expect(within(panel).getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  it("shows the network description in the detail panel", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    expect(within(panel).getByText("Main office network")).toBeInTheDocument();
  });

  it("closes the detail panel when the close button is clicked", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    const closeBtn = within(panel).getByRole("button", {
      name: /close panel/i,
    });
    await userEvent.click(closeBtn);
    expect(
      screen.queryByRole("dialog", { name: /network details/i }),
    ).not.toBeInTheDocument();
  });

  it("shows Enable button for inactive networks", async () => {
    render(<NetworksPage />);
    // DMZ is inactive
    await userEvent.click(screen.getByText("DMZ").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    expect(
      within(panel).getByRole("button", { name: /enable/i }),
    ).toBeInTheDocument();
  });

  it("shows Disable button for active networks", async () => {
    render(<NetworksPage />);
    // Office LAN is active
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    expect(
      within(panel).getByRole("button", { name: /disable/i }),
    ).toBeInTheDocument();
  });

  it("shows the rename button (pencil icon) in the detail panel header", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    expect(
      within(panel).getByRole("button", { name: /rename network/i }),
    ).toBeInTheDocument();
  });

  it("shows a text input when the rename button is clicked", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    await userEvent.click(
      within(panel).getByRole("button", { name: /rename network/i }),
    );
    expect(within(panel).getByRole("textbox")).toBeInTheDocument();
  });

  it("shows Delete link and then confirm prompt on click", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByText("Office LAN").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /network details/i });
    const deleteBtn = within(panel).getByRole("button", { name: /delete/i });
    await userEvent.click(deleteBtn);
    expect(within(panel).getByText("Delete this network?")).toBeInTheDocument();
  });
});

// ── Add network modal ─────────────────────────────────────────────────────────

vi.mock("../components/add-network-modal", () => ({
  AddNetworkModal: ({
    onClose,
  }: {
    onClose: () => void;
    onCreated?: () => void;
  }) => (
    <div role="dialog" aria-label="Add Network">
      <button type="button" onClick={onClose}>
        Close add modal
      </button>
    </div>
  ),
}));

describe("NetworksPage — add network modal", () => {
  it("opens AddNetworkModal when Add network is clicked", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByRole("button", { name: /add network/i }));
    expect(
      screen.getByRole("dialog", { name: /add network/i }),
    ).toBeInTheDocument();
  });

  it("closes AddNetworkModal when its onClose fires", async () => {
    render(<NetworksPage />);
    await userEvent.click(screen.getByRole("button", { name: /add network/i }));
    const modal = screen.getByRole("dialog", { name: /add network/i });
    await userEvent.click(
      within(modal).getByRole("button", { name: /close add modal/i }),
    );
    expect(
      screen.queryByRole("dialog", { name: /add network/i }),
    ).not.toBeInTheDocument();
  });
});
