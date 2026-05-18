import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DiscoveryPage } from "./discovery";

vi.mock("../api/hooks/use-discovery", () => ({
  useDiscoveryJobs: vi.fn(),
  useStartDiscovery: vi.fn(),
  useStopDiscovery: vi.fn(),
  useDiscoveryDiff: vi.fn(),
  useDiscoveryCompare: vi.fn(),
}));

vi.mock("../api/hooks/use-devices", () => ({
  useAcceptSuggestion: vi.fn(() => ({ mutateAsync: vi.fn().mockResolvedValue(undefined), isPending: false })),
  useDismissSuggestion: vi.fn(() => ({ mutateAsync: vi.fn().mockResolvedValue(undefined), isPending: false })),
}));

vi.mock("@tanstack/react-router", () => ({
  useSearch: vi.fn().mockReturnValue({}),
}));

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
vi.mock("../components/toast-provider", () => ({
  useToast: () => ({
    toast: { success: mockToastSuccess, error: mockToastError },
  }),
}));

vi.mock("../components/create-discovery-modal", () => ({
  CreateDiscoveryModal: ({ onClose }: { onClose: () => void }) => (
    <div data-testid="create-discovery-modal">
      <button type="button" onClick={onClose}>
        Close Modal
      </button>
    </div>
  ),
}));

import {
  useDiscoveryJobs,
  useStartDiscovery,
  useStopDiscovery,
  useDiscoveryDiff,
  useDiscoveryCompare,
} from "../api/hooks/use-discovery";

import {
  useAcceptSuggestion,
  useDismissSuggestion,
} from "../api/hooks/use-devices";

const mockUseAcceptSuggestion = vi.mocked(useAcceptSuggestion);
const mockUseDismissSuggestion = vi.mocked(useDismissSuggestion);

const mockUseDiscoveryJobs = vi.mocked(useDiscoveryJobs);
const mockUseStartDiscovery = vi.mocked(useStartDiscovery);
const mockUseStopDiscovery = vi.mocked(useStopDiscovery);
const mockUseDiscoveryDiff = vi.mocked(useDiscoveryDiff);
const mockUseDiscoveryCompare = vi.mocked(useDiscoveryCompare);

const mockJobs = [
  {
    id: "job-1",
    name: "Office LAN Discovery",
    networks: ["192.168.1.0/24"],
    method: "tcp" as const,
    status: "completed" as const,
    progress: 100,
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
  },
  {
    id: "job-2",
    name: "DMZ Discovery",
    networks: ["10.0.0.0/8"],
    method: "icmp" as const,
    status: "pending" as const,
    progress: 0,
    started_at: undefined,
    created_at: new Date().toISOString(),
  },
  {
    id: "job-3",
    name: undefined,
    networks: ["172.16.0.0/12"],
    method: "arp" as const,
    status: "running" as const,
    progress: 50,
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
  },
];

const startMutateMock = vi.fn();
const stopMutateMock = vi.fn();

function makeUseDiscoveryJobsResult(overrides = {}) {
  return {
    data: {
      data: mockJobs,
      pagination: { page: 1, page_size: 20, total_items: 3, total_pages: 1 },
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useDiscoveryJobs>;
}

function makeStartMutationResult(overrides = {}) {
  return {
    mutate: startMutateMock,
    mutateAsync: vi.fn(),
    isPending: false,
    ...overrides,
  } as unknown as ReturnType<typeof useStartDiscovery>;
}

function makeStopMutationResult(overrides = {}) {
  return {
    mutate: stopMutateMock,
    mutateAsync: vi.fn(),
    isPending: false,
    ...overrides,
  } as unknown as ReturnType<typeof useStopDiscovery>;
}

beforeEach(() => {
  vi.clearAllMocks();
  startMutateMock.mockReset();
  stopMutateMock.mockReset();
  mockUseDiscoveryJobs.mockReturnValue(makeUseDiscoveryJobsResult());
  mockUseStartDiscovery.mockReturnValue(makeStartMutationResult());
  mockUseStopDiscovery.mockReturnValue(makeStopMutationResult());
  mockUseDiscoveryDiff.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
  } as unknown as ReturnType<typeof useDiscoveryDiff>);
  mockUseDiscoveryCompare.mockReturnValue({
    data: undefined,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useDiscoveryCompare>);
  mockUseAcceptSuggestion.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  } as unknown as ReturnType<typeof useAcceptSuggestion>);
  mockUseDismissSuggestion.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  } as unknown as ReturnType<typeof useDismissSuggestion>);
});

describe("DiscoveryPage", () => {
  // ── Toolbar ───────────────────────────────────────────────────

  it("renders the 'New discovery job' button", () => {
    render(<DiscoveryPage />);
    expect(
      screen.getByRole("button", { name: /new discovery job/i }),
    ).toBeInTheDocument();
  });

  // ── Loading state ─────────────────────────────────────────────

  it("renders skeleton rows when loading", () => {
    mockUseDiscoveryJobs.mockReturnValue(
      makeUseDiscoveryJobsResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<DiscoveryPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("does not show empty message while loading", () => {
    mockUseDiscoveryJobs.mockReturnValue(
      makeUseDiscoveryJobsResult({ isLoading: true, data: undefined }),
    );
    render(<DiscoveryPage />);
    expect(
      screen.queryByText(/no discovery jobs found/i),
    ).not.toBeInTheDocument();
  });

  // ── Empty state ───────────────────────────────────────────────

  it("shows 'No discovery jobs found.' when the list is empty", () => {
    mockUseDiscoveryJobs.mockReturnValue(
      makeUseDiscoveryJobsResult({
        data: {
          data: [],
          pagination: {
            page: 1,
            page_size: 20,
            total_items: 0,
            total_pages: 1,
          },
        },
      }),
    );
    render(<DiscoveryPage />);
    expect(screen.getByText("No discovery jobs found.")).toBeInTheDocument();
  });

  // ── Column headers ────────────────────────────────────────────

  it("renders all expected column headers", () => {
    render(<DiscoveryPage />);
    const headers = [
      "Name",
      "Network",
      "Method",
      "Status",
      "Progress",
      "Started",
      "Created",
    ];
    for (const header of headers) {
      expect(
        screen.getByRole("columnheader", { name: header }),
      ).toBeInTheDocument();
    }
  });

  // ── Row data ──────────────────────────────────────────────────

  it("renders a row for each job", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // 1 header + 3 data rows
    expect(rows).toHaveLength(4);
  });

  it("renders job names", () => {
    render(<DiscoveryPage />);
    expect(screen.getByText("Office LAN Discovery")).toBeInTheDocument();
    expect(screen.getByText("DMZ Discovery")).toBeInTheDocument();
  });

  it("renders em-dash for missing job name", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-3 has no name (rows[3])
    const cells = within(rows[3]).getAllByRole("cell");
    expect(cells[0]).toHaveTextContent("—");
  });

  it("renders CIDRs in the network column", () => {
    render(<DiscoveryPage />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.0/8")).toBeInTheDocument();
    expect(screen.getByText("172.16.0.0/12")).toBeInTheDocument();
  });

  it("renders CIDRs with monospace font", () => {
    render(<DiscoveryPage />);
    const cidrCell = screen.getByText("192.168.1.0/24");
    expect(cidrCell).toHaveClass("font-mono");
  });

  it("renders method labels as uppercase strings", () => {
    render(<DiscoveryPage />);
    expect(screen.getByText("TCP")).toBeInTheDocument();
    expect(screen.getByText("ICMP")).toBeInTheDocument();
    expect(screen.getByText("ARP")).toBeInTheDocument();
  });

  it("renders StatusBadge for each job", () => {
    render(<DiscoveryPage />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("pending")).toBeInTheDocument();
    expect(screen.getByText("running")).toBeInTheDocument();
  });

  it("shows em-dash in Progress column for non-running jobs", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-1 is completed (rows[1]) — progress should show "—"
    const cells = within(rows[1]).getAllByRole("cell");
    expect(cells[4]).toHaveTextContent("—");
  });

  it("shows a progress bar in the Progress column for running jobs", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-3 is running (rows[3])
    const cells = within(rows[3]).getAllByRole("cell");
    // Should contain a div with rounded-full (progress bar) instead of "—"
    expect(cells[4].querySelector(".rounded-full")).toBeInTheDocument();
    expect(cells[4]).not.toHaveTextContent("—");
  });

  it("shows relative time for started_at when present", () => {
    render(<DiscoveryPage />);
    // job-1 and job-3 have started_at; expect at least one relative time
    const justNow = screen.getAllByText("just now");
    expect(justNow.length).toBeGreaterThanOrEqual(1);
  });

  it("shows em-dash for missing started_at", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-2 (pending) has no started_at — rows[2], Started column (index 5)
    const cells = within(rows[2]).getAllByRole("cell");
    expect(cells[5]).toHaveTextContent("—");
  });

  it("shows relative time for created_at when present", () => {
    render(<DiscoveryPage />);
    // All jobs have created_at
    const rows = screen.getAllByRole("row");
    const cells = within(rows[1]).getAllByRole("cell");
    expect(cells[6]).not.toHaveTextContent("—");
  });

  // ── Inline actions ────────────────────────────────────────────

  it("shows a Start button for pending jobs", () => {
    render(<DiscoveryPage />);
    expect(screen.getByRole("button", { name: /start/i })).toBeInTheDocument();
  });

  it("shows a Stop button for running jobs", () => {
    render(<DiscoveryPage />);
    expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument();
  });

  it("does not show Start or Stop button for completed jobs", () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-1 is completed (rows[1])
    expect(
      within(rows[1]).queryByRole("button", { name: /start/i }),
    ).not.toBeInTheDocument();
    expect(
      within(rows[1]).queryByRole("button", { name: /stop/i }),
    ).not.toBeInTheDocument();
  });

  it("clicking Start calls startDiscovery with the job id", async () => {
    render(<DiscoveryPage />);
    const startButton = screen.getByRole("button", { name: /start/i });
    await userEvent.click(startButton);
    expect(startMutateMock).toHaveBeenCalledWith("job-2");
  });

  it("clicking Stop calls stopDiscovery with the job id", async () => {
    render(<DiscoveryPage />);
    const stopButton = screen.getByRole("button", { name: /stop/i });
    await userEvent.click(stopButton);
    expect(stopMutateMock).toHaveBeenCalledWith("job-3");
  });

  it("clicking Start does not open the detail panel", async () => {
    render(<DiscoveryPage />);
    const startButton = screen.getByRole("button", { name: /start/i });
    await userEvent.click(startButton);
    expect(
      screen.queryByRole("dialog", { name: /discovery job details/i }),
    ).not.toBeInTheDocument();
  });

  // ── Detail panel ──────────────────────────────────────────────

  it("opens the detail panel when a row is clicked", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    expect(
      screen.getByRole("dialog", { name: /discovery job details/i }),
    ).toBeInTheDocument();
  });

  it("shows the job name in the detail panel header", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    expect(
      within(dialog).getByText("Office LAN Discovery"),
    ).toBeInTheDocument();
  });

  it("shows the network CIDR in the detail panel header", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    // CIDR appears in the panel header
    expect(
      within(dialog).getAllByText("192.168.1.0/24").length,
    ).toBeGreaterThanOrEqual(1);
  });

  it("shows the status badge in the detail panel", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const badges = within(dialog).getAllByText("completed");
    expect(badges.length).toBeGreaterThanOrEqual(1);
  });

  it("shows the job ID in the detail panel", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    expect(within(dialog).getByText("job-1")).toBeInTheDocument();
  });

  it("shows 'Discovery #ID' as title when job name is missing", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-3 has no name
    await userEvent.click(rows[3]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    expect(within(dialog).getByText("Discovery #job-3")).toBeInTheDocument();
  });

  it("closes the detail panel when the close button is clicked", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    expect(
      screen.getByRole("dialog", { name: /discovery job details/i }),
    ).toBeInTheDocument();

    const closeButton = screen.getByRole("button", { name: /close panel/i });
    await userEvent.click(closeButton);
    expect(
      screen.queryByRole("dialog", { name: /discovery job details/i }),
    ).not.toBeInTheDocument();
  });

  it("closes the detail panel when the backdrop is clicked", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    expect(
      screen.getByRole("dialog", { name: /discovery job details/i }),
    ).toBeInTheDocument();

    const backdrop = document.querySelector(".fixed.inset-0.bg-black\\/40");
    expect(backdrop).not.toBeNull();
    await userEvent.click(backdrop!);
    expect(
      screen.queryByRole("dialog", { name: /discovery job details/i }),
    ).not.toBeInTheDocument();
  });

  // ── Pagination ────────────────────────────────────────────────

  it("shows pagination when there are multiple pages", () => {
    mockUseDiscoveryJobs.mockReturnValue(
      makeUseDiscoveryJobsResult({
        data: {
          data: mockJobs,
          pagination: {
            page: 1,
            page_size: 20,
            total_items: 45,
            total_pages: 3,
          },
        },
      }),
    );
    render(<DiscoveryPage />);
    expect(
      screen.getByRole("button", { name: /previous page/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /next page/i }),
    ).toBeInTheDocument();
  });

  it("does not show pagination when there is only one page", () => {
    render(<DiscoveryPage />);
    expect(
      screen.queryByRole("button", { name: /previous page/i }),
    ).not.toBeInTheDocument();
  });

  // ── Changes tab ───────────────────────────────────────────────

  it("Changes tab is disabled for non-completed jobs", async () => {
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-2 is pending (rows[2])
    await userEvent.click(rows[2]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const changesTab = within(dialog).getByRole("button", { name: /changes/i });
    expect(changesTab).toBeDisabled();
  });

  it("Changes tab shows loading skeleton while diff loads", async () => {
    mockUseDiscoveryDiff.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as unknown as ReturnType<typeof useDiscoveryDiff>);
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-1 is completed (rows[1])
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const changesTab = within(dialog).getByRole("button", { name: /changes/i });
    await userEvent.click(changesTab);
    const skeletons = dialog.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("Changes tab renders new/gone/changed/unchanged sections", async () => {
    mockUseDiscoveryDiff.mockReturnValue({
      data: {
        job_id: "job-1",
        new_hosts: [
          {
            id: "h1",
            ip_address: "10.0.1.50",
            hostname: "router-1",
            status: "up",
            vendor: "Cisco Systems",
            last_seen: new Date().toISOString(),
            first_seen: new Date().toISOString(),
          },
        ],
        gone_hosts: [
          {
            id: "h2",
            ip_address: "10.0.1.10",
            hostname: "old-box",
            status: "down",
            last_seen: new Date(Date.now() - 3 * 86_400_000).toISOString(),
            first_seen: new Date().toISOString(),
          },
        ],
        changed_hosts: [
          {
            id: "h3",
            ip_address: "10.0.1.20",
            hostname: "server-1",
            status: "up",
            previous_status: "down",
            last_seen: new Date(Date.now() - 3_600_000).toISOString(),
            first_seen: new Date().toISOString(),
          },
        ],
        unchanged_count: 42,
      },
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useDiscoveryDiff>);
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const changesTab = within(dialog).getByRole("button", { name: /changes/i });
    await userEvent.click(changesTab);
    expect(within(dialog).getByText(/new \(1\)/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/gone \(1\)/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/changed \(1\)/i)).toBeInTheDocument();
    expect(within(dialog).getByText("10.0.1.50")).toBeInTheDocument();
    expect(within(dialog).getByText("42 hosts unchanged")).toBeInTheDocument();
    // status change arrow
    expect(within(dialog).getByText(/down → up/i)).toBeInTheDocument();
  });

  it("Changes tab shows empty state when no changes", async () => {
    mockUseDiscoveryDiff.mockReturnValue({
      data: {
        job_id: "job-1",
        new_hosts: [],
        gone_hosts: [],
        changed_hosts: [],
        unchanged_count: 0,
      },
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useDiscoveryDiff>);
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const changesTab = within(dialog).getByRole("button", { name: /changes/i });
    await userEvent.click(changesTab);
    expect(
      within(dialog).getByText(/no changes detected in this run/i),
    ).toBeInTheDocument();
  });

  // ── Compare panel ─────────────────────────────────────────────

  it("renders Compare runs button", () => {
    render(<DiscoveryPage />);
    expect(
      screen.getByRole("button", { name: /compare runs/i }),
    ).toBeInTheDocument();
  });

  it("shows compare panel when Compare runs clicked", async () => {
    render(<DiscoveryPage />);
    const compareButton = screen.getByRole("button", { name: /compare runs/i });
    await userEvent.click(compareButton);
    expect(screen.getByText("Compare Discovery Runs")).toBeInTheDocument();
  });

  it("shows run selectors in compare panel", async () => {
    render(<DiscoveryPage />);
    const compareButton = screen.getByRole("button", { name: /compare runs/i });
    await userEvent.click(compareButton);
    expect(
      screen.getByRole("combobox", { name: /baseline run a/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: /current run b/i }),
    ).toBeInTheDocument();
  });

  it("hides compare panel when close button clicked", async () => {
    render(<DiscoveryPage />);
    const compareButton = screen.getByRole("button", { name: /compare runs/i });
    await userEvent.click(compareButton);
    expect(screen.getByText("Compare Discovery Runs")).toBeInTheDocument();

    const closeButton = screen.getByRole("button", {
      name: /close compare panel/i,
    });
    await userEvent.click(closeButton);
    expect(
      screen.queryByText("Compare Discovery Runs"),
    ).not.toBeInTheDocument();
  });

  // ── Create modal ──────────────────────────────────────────────

  it("opens the create modal when 'New discovery job' is clicked", async () => {
    render(<DiscoveryPage />);
    const button = screen.getByRole("button", { name: /new discovery job/i });
    await userEvent.click(button);
    expect(screen.getByTestId("create-discovery-modal")).toBeInTheDocument();
  });

  it("closes the create modal when the modal's onClose is called", async () => {
    render(<DiscoveryPage />);
    const button = screen.getByRole("button", { name: /new discovery job/i });
    await userEvent.click(button);
    expect(screen.getByTestId("create-discovery-modal")).toBeInTheDocument();

    const closeButton = screen.getByRole("button", { name: /close modal/i });
    await userEvent.click(closeButton);
    expect(
      screen.queryByTestId("create-discovery-modal"),
    ).not.toBeInTheDocument();
  });

  // ── Suggestion cards ──────────────────────────────────────────

  async function openChangesTabWithDiff(suggestions = []) {
    const diffData = {
      job_id: "job-1",
      new_hosts: [],
      gone_hosts: [],
      changed_hosts: [],
      unchanged_count: 5,
      suggestions,
    };
    mockUseDiscoveryDiff.mockReturnValue({
      data: diffData,
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useDiscoveryDiff>);
    render(<DiscoveryPage />);
    const rows = screen.getAllByRole("row");
    // job-1 is completed
    await userEvent.click(rows[1]);
    const dialog = screen.getByRole("dialog", {
      name: /discovery job details/i,
    });
    const changesTab = within(dialog).getByRole("button", { name: /changes/i });
    await userEvent.click(changesTab);
    return dialog;
  }

  it("does not render suggestion cards when suggestions array is empty", async () => {
    const dialog = await openChangesTabWithDiff([]);
    expect(
      within(dialog).queryByTestId("suggestions-section"),
    ).not.toBeInTheDocument();
  });

  it("renders suggestion cards for active (non-dismissed) suggestions", async () => {
    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 85,
        confidence_reason: "MAC address matches known device",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
      {
        id: "s2",
        host_id: "h2",
        device_id: "d2",
        confidence_score: 45,
        confidence_reason: "IP in known subnet",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    expect(within(dialog).getByTestId("suggestions-section")).toBeInTheDocument();
    const cards = within(dialog).getAllByTestId("suggestion-card");
    expect(cards).toHaveLength(2);
    expect(within(dialog).getByText("MAC address matches known device")).toBeInTheDocument();
    expect(within(dialog).getByText("IP in known subnet")).toBeInTheDocument();
  });

  it("filters out dismissed suggestions", async () => {
    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 80,
        confidence_reason: "High confidence",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
      {
        id: "s2",
        host_id: "h2",
        device_id: "d2",
        confidence_score: 60,
        confidence_reason: "Should be hidden",
        dismissed: true,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    const cards = within(dialog).getAllByTestId("suggestion-card");
    expect(cards).toHaveLength(1);
    expect(within(dialog).queryByText("Should be hidden")).not.toBeInTheDocument();
  });

  it("shows confidence score badge on suggestion card", async () => {
    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 75,
        confidence_reason: "Good match",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    expect(within(dialog).getByText("75%")).toBeInTheDocument();
  });

  it("calls acceptSuggestion when Accept button is clicked", async () => {
    const acceptMutateAsync = vi.fn().mockResolvedValue(undefined);
    mockUseAcceptSuggestion.mockReturnValue({
      mutateAsync: acceptMutateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useAcceptSuggestion>);

    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 80,
        confidence_reason: "Good",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    const card = within(dialog).getByTestId("suggestion-card");
    await userEvent.click(within(card).getByRole("button", { name: /accept/i }));
    expect(acceptMutateAsync).toHaveBeenCalledWith("s1");
  });

  it("calls dismissSuggestion when Dismiss button is clicked", async () => {
    const dismissMutateAsync = vi.fn().mockResolvedValue(undefined);
    mockUseDismissSuggestion.mockReturnValue({
      mutateAsync: dismissMutateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useDismissSuggestion>);

    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 80,
        confidence_reason: "Good",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    const card = within(dialog).getByTestId("suggestion-card");
    await userEvent.click(within(card).getByRole("button", { name: /dismiss/i }));
    expect(dismissMutateAsync).toHaveBeenCalledWith("s1");
  });

  it("shows error toast when accept fails", async () => {
    mockUseAcceptSuggestion.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("Server error")),
      isPending: false,
    } as unknown as ReturnType<typeof useAcceptSuggestion>);

    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 80,
        confidence_reason: "Good",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    const card = within(dialog).getByTestId("suggestion-card");
    await userEvent.click(within(card).getByRole("button", { name: /accept/i }));
    expect(mockToastError).toHaveBeenCalledWith("Failed to accept suggestion.");
  });

  it("shows error toast when dismiss fails", async () => {
    mockUseDismissSuggestion.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("Server error")),
      isPending: false,
    } as unknown as ReturnType<typeof useDismissSuggestion>);

    const suggestions = [
      {
        id: "s1",
        host_id: "h1",
        device_id: "d1",
        confidence_score: 80,
        confidence_reason: "Good",
        dismissed: false,
        created_at: new Date().toISOString(),
      },
    ];
    const dialog = await openChangesTabWithDiff(suggestions);
    const card = within(dialog).getByTestId("suggestion-card");
    await userEvent.click(within(card).getByRole("button", { name: /dismiss/i }));
    expect(mockToastError).toHaveBeenCalledWith("Failed to dismiss suggestion.");
  });
});
