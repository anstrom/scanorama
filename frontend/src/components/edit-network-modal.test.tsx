import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { EditNetworkModal } from "./edit-network-modal";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-networks", () => ({
  useUpdateNetwork: vi.fn(),
}));

import { useUpdateNetwork } from "../api/hooks/use-networks";

const mockUseUpdateNetwork = vi.mocked(useUpdateNetwork);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockNetwork = {
  id: "net-1",
  name: "Office LAN",
  cidr: "192.168.1.0/24",
  description: "Main office network",
  discovery_method: "ping",
  scan_enabled: true,
  is_active: true,
};

function setupDefaultMocks() {
  mockUseUpdateNetwork.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ ...mockNetwork }),
    isPending: false,
  } as unknown as ReturnType<typeof useUpdateNetwork>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("EditNetworkModal", () => {
  it("renders the dialog with the Edit Network heading", () => {
    render(
      <EditNetworkModal
        network={mockNetwork}
        onClose={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Edit Network" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("pre-populates the name and CIDR fields from the network prop", () => {
    render(
      <EditNetworkModal
        network={mockNetwork}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Name")).toHaveValue("Office LAN");
    expect(screen.getByLabelText("CIDR block")).toHaveValue("192.168.1.0/24");
  });

  it("shows a validation error when CIDR is invalid", async () => {
    const user = userEvent.setup();
    render(
      <EditNetworkModal
        network={mockNetwork}
        onClose={vi.fn()}
      />,
    );

    const cidrInput = screen.getByLabelText("CIDR block");
    await user.clear(cidrInput);
    await user.type(cidrInput, "not-valid-cidr");
    await user.click(screen.getByRole("button", { name: /save changes/i }));

    expect(
      screen.getByText(/CIDR block must be in valid notation/i),
    ).toBeInTheDocument();
  });

  it("calls mutateAsync with correct networkId and body on valid submit", async () => {
    const user = userEvent.setup();
    const mockMutateAsync = vi.fn().mockResolvedValue({ ...mockNetwork });
    mockUseUpdateNetwork.mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateNetwork>);

    render(
      <EditNetworkModal
        network={mockNetwork}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          networkId: "net-1",
          body: expect.objectContaining({
            name: "Office LAN",
            cidr: "192.168.1.0/24",
          }),
        }),
      );
    });
  });

  it("calls onClose when the Cancel button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<EditNetworkModal network={mockNetwork} onClose={onClose} />);

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
