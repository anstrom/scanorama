import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DeviceDetailPage } from "./devices";

// ── Mock hooks ────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-devices", () => ({
  useDevice: vi.fn(),
  useDeleteDevice: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useDetachHost: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useUpdateDevice: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock("../components/toast-provider", () => ({
  useToast: () => ({ toast: { success: vi.fn(), error: vi.fn() } }),
}));

import { useDevice } from "../api/hooks/use-devices";
const mockUseDevice = vi.mocked(useDevice);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockDevice = {
  id: "d1",
  name: "Lab Pi",
  notes: "Raspberry Pi 4 in the lab",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-06-01T00:00:00Z",
  known_macs: [
    { id: "m1", mac_address: "AA:BB:CC:DD:EE:FF", first_seen: "2024-01-01T00:00:00Z", last_seen: "2024-06-01T00:00:00Z" },
  ],
  known_names: [
    { id: "n1", name: "lab-pi.local", source: "mdns", first_seen: "2024-01-01T00:00:00Z", last_seen: "2024-06-01T00:00:00Z" },
  ],
  hosts: [
    { id: "h1", ip_address: "192.168.1.50", hostname: "lab-pi", status: "up", mac_address: undefined, os_family: "Linux", vendor: "Raspberry Pi Foundation", first_seen: "2024-01-01T00:00:00Z", last_seen: "2024-06-01T00:00:00Z" },
  ],
};

function setupDefaultMocks() {
  mockUseDevice.mockReturnValue({
    data: mockDevice,
    isLoading: false,
    isError: false,
  } as ReturnType<typeof useDevice>);
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("DeviceDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupDefaultMocks();
  });

  it("shows loading skeleton while data is fetching", () => {
    mockUseDevice.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as ReturnType<typeof useDevice>);

    render(<DeviceDetailPage id="d1" />);
    expect(document.querySelector(".animate-pulse")).not.toBeNull();
  });

  it("shows error state when the device fails to load", () => {
    mockUseDevice.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    } as ReturnType<typeof useDevice>);

    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByText("Failed to load device.")).toBeInTheDocument();
  });

  it("renders device name, notes, and meta in the happy path", () => {
    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Lab Pi");
    expect(screen.getByText("Raspberry Pi 4 in the lab")).toBeInTheDocument();
  });

  it("renders known MAC addresses", () => {
    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByText("AA:BB:CC:DD:EE:FF")).toBeInTheDocument();
  });

  it("renders known names with source badge", () => {
    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByText("lab-pi.local")).toBeInTheDocument();
    expect(screen.getByText("mDNS")).toBeInTheDocument();
  });

  it("renders attached host IP and hostname", () => {
    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByText("192.168.1.50")).toBeInTheDocument();
    expect(screen.getByText("lab-pi")).toBeInTheDocument();
  });

  it("shows empty states when lists are empty", () => {
    mockUseDevice.mockReturnValue({
      data: { ...mockDevice, known_macs: [], known_names: [], hosts: [] },
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useDevice>);

    render(<DeviceDetailPage id="d1" />);
    expect(screen.getByText("No MACs recorded.")).toBeInTheDocument();
    expect(screen.getByText("No names recorded.")).toBeInTheDocument();
    expect(screen.getByText("No hosts attached.")).toBeInTheDocument();
  });

  it("shows delete confirm when Delete device is clicked", async () => {
    const user = userEvent.setup();
    render(<DeviceDetailPage id="d1" />);

    await user.click(screen.getByText("Delete device"));
    expect(screen.getByText("Delete this device?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /confirm/i })).toBeInTheDocument();
  });

  it("hides delete confirm when Cancel is clicked", async () => {
    const user = userEvent.setup();
    render(<DeviceDetailPage id="d1" />);

    await user.click(screen.getByText("Delete device"));
    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(screen.queryByText("Delete this device?")).not.toBeInTheDocument();
  });

  it("opens rename modal when Rename device button is clicked", async () => {
    const user = userEvent.setup();
    render(<DeviceDetailPage id="d1" />);

    await user.click(screen.getByRole("button", { name: "Rename device" }));
    expect(screen.getByRole("dialog", { name: "Rename device" })).toBeInTheDocument();
  });

  it("calls onClose when the close button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<DeviceDetailPage id="d1" onClose={onClose} />);

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
