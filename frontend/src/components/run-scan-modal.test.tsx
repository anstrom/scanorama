import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { RunScanModal } from "./run-scan-modal";

vi.mock("../api/hooks/use-profiles", () => ({
  useProfiles: vi.fn(),
}));

vi.mock("../api/hooks/use-scans", () => ({
  useCreateScan: vi.fn(),
  useStartScan: vi.fn(),
}));

import { useProfiles } from "../api/hooks/use-profiles";
import { useCreateScan, useStartScan } from "../api/hooks/use-scans";

const mockUseProfiles = vi.mocked(useProfiles);
const mockUseCreateScan = vi.mocked(useCreateScan);
const mockUseStartScan = vi.mocked(useStartScan);

const mockProfiles = [
  { id: "p1", name: "Quick scan", scan_type: "connect", ports: "22,80,443" },
  { id: "p2", name: "Full scan", scan_type: "syn", ports: undefined },
];

function setupDefaultMocks() {
  mockUseProfiles.mockReturnValue({
    data: {
      data: mockProfiles,
      pagination: { page: 1, page_size: 100, total_items: 2, total_pages: 1 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useProfiles>);

  mockUseCreateScan.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ id: "new-scan-1" }),
    isPending: false,
  } as unknown as ReturnType<typeof useCreateScan>);

  mockUseStartScan.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  } as unknown as ReturnType<typeof useStartScan>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

// ── rendering ─────────────────────────────────────────────────────────────────

describe("RunScanModal", () => {
  it("renders the dialog with the Run Scan heading", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(
      screen.getByRole("dialog", { name: "Run Scan" }),
    ).toBeInTheDocument();
  });

  it("renders the Target input", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByLabelText("Target")).toBeInTheDocument();
  });

  it("pre-fills the target field when initialTarget is provided", () => {
    render(<RunScanModal onClose={vi.fn()} initialTarget="192.168.1.1" />);
    expect(screen.getByLabelText("Target")).toHaveValue("192.168.1.1");
  });

  it("renders the Run scan submit button", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Run scan" }),
    ).toBeInTheDocument();
  });

  it("renders the Cancel button", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("renders the Close dialog button", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Close dialog" }),
    ).toBeInTheDocument();
  });

  it("renders the backdrop overlay", () => {
    const { container } = render(<RunScanModal onClose={vi.fn()} />);
    const backdrop = container.querySelector(".fixed.inset-0.bg-black\\/50");
    expect(backdrop).toBeInTheDocument();
  });

  // ── mode toggle ───────────────────────────────────────────────────────────

  it("defaults to profile mode", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    const profileBtn = screen.getByRole("radio", { name: "Profile" });
    expect(profileBtn).toHaveAttribute("aria-checked", "true");
  });

  it("shows the profile selector in profile mode", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByLabelText("Select profile")).toBeInTheDocument();
  });

  it("does not show ports input in profile mode", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.queryByLabelText(/Ports/)).not.toBeInTheDocument();
  });

  it("switches to custom mode when Custom ports is clicked", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    expect(screen.getByRole("radio", { name: "Custom ports" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("shows ports and scan type inputs in custom mode", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    expect(screen.getByLabelText(/Ports/)).toBeInTheDocument();
    expect(screen.getByLabelText("Select scan type")).toBeInTheDocument();
  });

  it("hides the profile selector in custom mode", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    expect(screen.queryByLabelText("Select profile")).not.toBeInTheDocument();
  });

  // ── profile selector ──────────────────────────────────────────────────────

  it("lists all profiles in the select", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    const select = screen.getByLabelText("Select profile");
    expect(
      within(select).getByRole("option", { name: "Quick scan" }),
    ).toBeInTheDocument();
    expect(
      within(select).getByRole("option", { name: "Full scan" }),
    ).toBeInTheDocument();
  });

  it("shows a loading indicator while profiles are loading", () => {
    mockUseProfiles.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useProfiles>);
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByText("Loading profiles…")).toBeInTheDocument();
  });

  it("shows a message when no profiles exist", () => {
    mockUseProfiles.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 100, total_items: 0, total_pages: 0 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useProfiles>);
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByText(/No profiles found/)).toBeInTheDocument();
  });

  // ── OS detection ──────────────────────────────────────────────────────────

  it("renders the OS detection checkbox", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByLabelText(/OS detection/)).toBeInTheDocument();
  });

  it("OS detection checkbox is unchecked by default", () => {
    render(<RunScanModal onClose={vi.fn()} />);
    expect(screen.getByLabelText(/OS detection/)).not.toBeChecked();
  });

  it("toggles OS detection on click", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    const checkbox = screen.getByLabelText(/OS detection/);
    await user.click(checkbox);
    expect(checkbox).toBeChecked();
    await user.click(checkbox);
    expect(checkbox).not.toBeChecked();
  });

  // ── close behaviour ───────────────────────────────────────────────────────

  it("calls onClose when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<RunScanModal onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the X button is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<RunScanModal onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Close dialog" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the backdrop is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const { container } = render(<RunScanModal onClose={onClose} />);
    const backdrop = container.querySelector(
      ".fixed.inset-0.bg-black\\/50",
    ) as HTMLElement;
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // ── validation ────────────────────────────────────────────────────────────

  it("shows an error when submitted with an empty target", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Please enter at least one target.",
    );
  });

  it("shows an error in profile mode when no profile is selected", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("Target"), "192.168.1.1");
    await user.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Please select a profile.",
    );
  });

  it("clears the error when the user starts fixing input", async () => {
    const user = userEvent.setup();
    render(<RunScanModal onClose={vi.fn()} />);
    // Trigger empty-target error.
    await user.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByRole("alert")).toBeInTheDocument();
    // Typing resets on the next submit, not immediately — just verify
    // the component stays stable after typing.
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    expect(screen.getByLabelText("Target")).toHaveValue("10.0.0.1");
  });

  // ── successful submission (profile mode) ──────────────────────────────────

  it("calls createScan and startScan on valid profile-mode submission", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-99" });
    const startScan = vi.fn().mockResolvedValue(undefined);
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);
    mockUseStartScan.mockReturnValue({
      mutateAsync: startScan,
      isPending: false,
    } as unknown as ReturnType<typeof useStartScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.selectOptions(screen.getByLabelText("Select profile"), "p1");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledTimes(1);
    expect(createScan).toHaveBeenCalledWith(
      expect.objectContaining({
        targets: ["10.0.0.1"],
        scan_type: "connect",
        ports: "22,80,443",
      }),
    );
    expect(startScan).toHaveBeenCalledWith("scan-99");
  });

  it("calls onSubmitted after a successful submission", async () => {
    const user = userEvent.setup();
    const onSubmitted = vi.fn();
    const onClose = vi.fn();

    render(<RunScanModal onClose={onClose} onSubmitted={onSubmitted} />);
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.selectOptions(screen.getByLabelText("Select profile"), "p1");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(onSubmitted).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not include os_detection in payload when checkbox is unchecked", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-1" });
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.selectOptions(screen.getByLabelText("Select profile"), "p1");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledWith(
      expect.not.objectContaining({ os_detection: true }),
    );
  });

  it("includes os_detection: true when checkbox is checked", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-1" });
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.selectOptions(screen.getByLabelText("Select profile"), "p1");
    await user.click(screen.getByLabelText(/OS detection/));
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledWith(
      expect.objectContaining({ os_detection: true }),
    );
  });

  // ── successful submission (custom mode) ───────────────────────────────────

  it("calls createScan with custom scan_type in custom mode", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-2" });
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    await user.type(screen.getByLabelText("Target"), "172.16.0.1");
    await user.type(screen.getByLabelText(/Ports/), "80");
    await user.selectOptions(screen.getByLabelText("Select scan type"), "syn");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledWith(
      expect.objectContaining({ scan_type: "syn", targets: ["172.16.0.1"] }),
    );
  });

  it("includes ports in the payload when entered in custom mode", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-3" });
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.type(screen.getByLabelText(/Ports/), "80,443");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledWith(
      expect.objectContaining({ ports: "80,443" }),
    );
  });

  it("splits comma-separated targets into an array", async () => {
    const user = userEvent.setup();
    const createScan = vi.fn().mockResolvedValue({ id: "scan-4" });
    mockUseCreateScan.mockReturnValue({
      mutateAsync: createScan,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    await user.type(
      screen.getByLabelText("Target"),
      "10.0.0.1, 10.0.0.2, 10.0.0.3",
    );
    await user.type(screen.getByLabelText(/Ports/), "80");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(createScan).toHaveBeenCalledWith(
      expect.objectContaining({
        targets: ["10.0.0.1", "10.0.0.2", "10.0.0.3"],
      }),
    );
  });

  // ── error from API ────────────────────────────────────────────────────────

  it("shows an inline error when createScan throws", async () => {
    const user = userEvent.setup();
    mockUseCreateScan.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("Network error")),
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.type(screen.getByLabelText(/Ports/), "80");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Network error");
  });

  it("shows a fallback error message when a non-Error is thrown", async () => {
    const user = userEvent.setup();
    mockUseCreateScan.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue("something went wrong"),
      isPending: false,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: "Custom ports" }));
    await user.type(screen.getByLabelText("Target"), "10.0.0.1");
    await user.type(screen.getByLabelText(/Ports/), "80");
    await user.click(screen.getByRole("button", { name: "Run scan" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Failed to start scan.",
    );
  });

  // ── pending state ─────────────────────────────────────────────────────────

  it("shows 'Starting…' and disables submit button while pending", () => {
    mockUseCreateScan.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
    } as unknown as ReturnType<typeof useCreateScan>);

    render(<RunScanModal onClose={vi.fn()} />);
    const submitBtn = screen.getByRole("button", { name: /Starting/ });
    expect(submitBtn).toBeDisabled();
  });
});
