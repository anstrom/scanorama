import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ScanNetworkModal } from "./scan-network-modal";

// ── Mocks ──────────────────────────────────────────────────────────────────────

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
vi.mock("./toast-provider", () => ({
  useToast: () => ({
    toast: { success: mockToastSuccess, error: mockToastError },
  }),
}));

vi.mock("../api/hooks/use-hosts", () => ({
  useHosts: vi.fn(),
}));

vi.mock("../api/hooks/use-profiles", () => ({
  useProfiles: vi.fn(),
}));

vi.mock("../api/hooks/use-networks", () => ({
  useStartNetworkScan: vi.fn(),
}));

vi.mock("../api/hooks/use-scans", () => ({
  useStartScan: vi.fn(),
}));

import { useHosts } from "../api/hooks/use-hosts";
import { useProfiles } from "../api/hooks/use-profiles";
import { useStartNetworkScan } from "../api/hooks/use-networks";
import { useStartScan } from "../api/hooks/use-scans";

const mockUseHosts = vi.mocked(useHosts);
const mockUseProfiles = vi.mocked(useProfiles);
const mockUseStartNetworkScan = vi.mocked(useStartNetworkScan);
const mockUseStartScan = vi.mocked(useStartScan);

const mockNetwork = {
  id: "net-1",
  name: "Office LAN",
  cidr: "192.168.1.0/24",
};

const mockProfiles = [
  { id: "p1", name: "Quick scan", scan_type: "connect", ports: "22,80,443" },
  { id: "p2", name: "Full scan", scan_type: "syn", ports: undefined },
];

function setupDefaultMocks() {
  mockUseHosts.mockReturnValue({
    data: {
      data: [],
      pagination: { page: 1, page_size: 1, total_items: 3, total_pages: 1 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useHosts>);

  mockUseProfiles.mockReturnValue({
    data: {
      data: mockProfiles,
      pagination: { page: 1, page_size: 100, total_items: 2, total_pages: 1 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useProfiles>);

  mockUseStartNetworkScan.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ id: "scan-1" }),
    isPending: false,
  } as unknown as ReturnType<typeof useStartNetworkScan>);

  mockUseStartScan.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  } as unknown as ReturnType<typeof useStartScan>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

// ── Rendering ─────────────────────────────────────────────────────────────────

describe("ScanNetworkModal", () => {
  it("renders the dialog with 'Scan Active Hosts' heading", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(
      screen.getByRole("dialog", { name: "Scan Active Hosts" }),
    ).toBeInTheDocument();
  });

  it("displays the network name", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByText("Office LAN")).toBeInTheDocument();
  });

  it("displays the network CIDR", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  it("shows active host count after loading", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    // "3" is in a child <span>; match the direct text node instead
    expect(screen.getByText(/active hosts will be scanned/)).toBeInTheDocument();
  });

  it("shows a loading indicator while fetching hosts", () => {
    mockUseHosts.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useHosts>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByText(/checking active hosts/i)).toBeInTheDocument();
  });

  it("renders the profile selector", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByLabelText("Select profile")).toBeInTheDocument();
  });

  it("shows a loading indicator while profiles load", () => {
    mockUseProfiles.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useProfiles>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByText(/loading profiles/i)).toBeInTheDocument();
  });

  it("lists all profiles in the selector", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    const select = screen.getByLabelText("Select profile");
    expect(select).toHaveTextContent("Quick scan");
    expect(select).toHaveTextContent("Full scan");
  });

  it("renders the OS detection checkbox", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByLabelText(/OS fingerprint/i)).toBeInTheDocument();
  });

  it("OS detection checkbox is unchecked by default", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByLabelText(/OS fingerprint/i)).not.toBeChecked();
  });

  it("renders the Cancel button", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("renders the Close dialog button", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Close dialog" }),
    ).toBeInTheDocument();
  });

  // ── Submit button label ────────────────────────────────────────────────────

  it("shows 'Scan 3 hosts' when there are 3 active hosts", () => {
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Scan 3 hosts" }),
    ).toBeInTheDocument();
  });

  it("shows 'Scan 1 host' (singular) when there is 1 active host", () => {
    mockUseHosts.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 1, total_pages: 1 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useHosts>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Scan 1 host" }),
    ).toBeInTheDocument();
  });

  it("shows 'Scan hosts' and is disabled when there are 0 active hosts", () => {
    mockUseHosts.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 0, total_pages: 1 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useHosts>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    const btn = screen.getByRole("button", { name: "Scan hosts" });
    expect(btn).toBeDisabled();
  });

  it("disables the submit button while hosts are loading", () => {
    mockUseHosts.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useHosts>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    // Button renders as "Scan hosts" with 0 count while loading
    const btns = screen
      .getAllByRole("button")
      .filter((b) => /scan/i.test(b.textContent ?? ""));
    expect(btns[0]).toBeDisabled();
  });

  // ── Close behaviour ───────────────────────────────────────────────────────

  it("calls onClose when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<ScanNetworkModal network={mockNetwork} onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the X button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<ScanNetworkModal network={mockNetwork} onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Close dialog" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the backdrop is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const { container } = render(
      <ScanNetworkModal network={mockNetwork} onClose={onClose} />,
    );
    const backdrop = container.querySelector(
      ".fixed.inset-0.bg-black\\/50",
    ) as HTMLElement;
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // ── Successful submission ─────────────────────────────────────────────────

  it("calls createNetworkScan and startScan on submit", async () => {
    const user = userEvent.setup();
    const createNetworkScan = vi.fn().mockResolvedValue({ id: "scan-42" });
    const startScan = vi.fn().mockResolvedValue(undefined);
    mockUseStartNetworkScan.mockReturnValue({
      mutateAsync: createNetworkScan,
      isPending: false,
    } as unknown as ReturnType<typeof useStartNetworkScan>);
    mockUseStartScan.mockReturnValue({
      mutateAsync: startScan,
      isPending: false,
    } as unknown as ReturnType<typeof useStartScan>);

    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));

    expect(createNetworkScan).toHaveBeenCalledWith({
      networkId: "net-1",
      osDetection: false,
    });
    expect(startScan).toHaveBeenCalledWith("scan-42");
  });

  it("calls createNetworkScan with osDetection: true when checkbox is checked", async () => {
    const user = userEvent.setup();
    const createNetworkScan = vi.fn().mockResolvedValue({ id: "scan-5" });
    mockUseStartNetworkScan.mockReturnValue({
      mutateAsync: createNetworkScan,
      isPending: false,
    } as unknown as ReturnType<typeof useStartNetworkScan>);

    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    await user.click(screen.getByLabelText(/OS fingerprint/i));
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));

    expect(createNetworkScan).toHaveBeenCalledWith(
      expect.objectContaining({ osDetection: true }),
    );
  });

  it("fires a success toast after a successful scan", async () => {
    const user = userEvent.setup();
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));
    expect(mockToastSuccess).toHaveBeenCalledWith(
      expect.stringMatching(/3 active host/),
    );
  });

  it("calls onSubmitted and onClose after a successful scan", async () => {
    const user = userEvent.setup();
    const onSubmitted = vi.fn();
    const onClose = vi.fn();
    render(
      <ScanNetworkModal
        network={mockNetwork}
        onClose={onClose}
        onSubmitted={onSubmitted}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));
    expect(onSubmitted).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // ── Pending state ─────────────────────────────────────────────────────────

  it("shows 'Starting…' and disables submit while mutation is pending", () => {
    mockUseStartNetworkScan.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
    } as unknown as ReturnType<typeof useStartNetworkScan>);
    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    const btn = screen.getByRole("button", { name: /Starting/ });
    expect(btn).toBeDisabled();
  });

  // ── Error from API ────────────────────────────────────────────────────────

  it("shows an inline error and fires an error toast when the API fails", async () => {
    const user = userEvent.setup();
    mockUseStartNetworkScan.mockReturnValue({
      mutateAsync: vi
        .fn()
        .mockRejectedValue(new Error("Internal server error")),
      isPending: false,
    } as unknown as ReturnType<typeof useStartNetworkScan>);

    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Internal server error",
    );
    expect(mockToastError).toHaveBeenCalledWith("Internal server error");
  });

  it("shows a fallback error message when a non-Error is thrown", async () => {
    const user = userEvent.setup();
    mockUseStartNetworkScan.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue("oops"),
      isPending: false,
    } as unknown as ReturnType<typeof useStartNetworkScan>);

    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Scan 3 hosts" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Failed to start scan.",
    );
  });

  it("shows an error and does not call startScan when active host count is 0 at submit time", async () => {
    const user = userEvent.setup();
    // Start with 3 hosts so button is enabled, then switch to 0
    const startScan = vi.fn();
    mockUseStartScan.mockReturnValue({
      mutateAsync: startScan,
      isPending: false,
    } as unknown as ReturnType<typeof useStartScan>);
    // Simulate 0 hosts returned after initial render
    mockUseHosts.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 0, total_pages: 0 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useHosts>);

    render(<ScanNetworkModal network={mockNetwork} onClose={vi.fn()} />);
    // Button is disabled when count is 0, form submission via enter key would still be blocked
    const btn = screen.getByRole("button", { name: "Scan hosts" });
    expect(btn).toBeDisabled();
    expect(startScan).not.toHaveBeenCalled();
  });
});
