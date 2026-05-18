import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderWithRouter } from "../test/utils";
import { OnboardingWizard } from "./onboarding-wizard";

// ── localStorage mock ──────────────────────────────────────────────────────────
// Node 26 has an experimental (broken) global localStorage that conflicts with
// jsdom's. We stub it with a working in-memory implementation.

function makeStorageMock(): Storage {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      Object.keys(store).forEach((k) => delete store[k]);
    },
    get length() {
      return Object.keys(store).length;
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
  };
}

// ── Mock hooks ─────────────────────────────────────────────────────────────────

const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return { ...actual, useNavigate: () => mockNavigate };
});

vi.mock("../api/hooks/use-networks", () => ({
  useNetworks: vi.fn(),
  useCreateNetwork: vi.fn(),
}));

vi.mock("../api/hooks/use-discovery", () => ({
  useRerunDiscovery: vi.fn(),
  useDiscoveryJobs: vi.fn(),
}));

import { useNetworks, useCreateNetwork } from "../api/hooks/use-networks";
import { useRerunDiscovery, useDiscoveryJobs } from "../api/hooks/use-discovery";

const mockUseNetworks = vi.mocked(useNetworks);
const mockUseCreateNetwork = vi.mocked(useCreateNetwork);
const mockUseRerunDiscovery = vi.mocked(useRerunDiscovery);
const mockUseDiscoveryJobs = vi.mocked(useDiscoveryJobs);

const SKIP_KEY = "scanorama_onboarding_skipped";

// ── Default mock setup ─────────────────────────────────────────────────────────

function setupDefaultMocks() {
  mockUseNetworks.mockReturnValue({
    data: { data: [], total: 0 },
    isLoading: false,
  } as unknown as ReturnType<typeof useNetworks>);

  mockUseCreateNetwork.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ id: "net-1" }),
    isPending: false,
  } as unknown as ReturnType<typeof useCreateNetwork>);

  mockUseRerunDiscovery.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({ id: "job-1" }),
    isPending: false,
  } as unknown as ReturnType<typeof useRerunDiscovery>);

  mockUseDiscoveryJobs.mockReturnValue({
    data: { data: [], total: 0 },
    isLoading: false,
  } as unknown as ReturnType<typeof useDiscoveryJobs>);
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("OnboardingWizard", () => {
  let storageMock: Storage;

  beforeEach(() => {
    storageMock = makeStorageMock();
    vi.stubGlobal("localStorage", storageMock);
    vi.clearAllMocks();
    mockNavigate.mockClear();
    setupDefaultMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // 1. Does not render wizard when networks.total > 0
  it("does not render wizard when networks already exist", async () => {
    mockUseNetworks.mockReturnValue({
      data: { data: [{ id: "net-existing" }], total: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useNetworks>);

    await renderWithRouter(<OnboardingWizard />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  // 2. Does not render wizard when localStorage skip flag is set
  it("does not render wizard when the skip flag is set in localStorage", async () => {
    localStorage.setItem(SKIP_KEY, "true");

    await renderWithRouter(<OnboardingWizard />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  // 3. Renders wizard when networks total is 0
  it("renders the wizard when no networks exist", async () => {
    await renderWithRouter(<OnboardingWizard />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  // 4. Shows "Step 1 of 3" progress text
  it("shows 'Step 1 of 3' progress indicator on first render", async () => {
    await renderWithRouter(<OnboardingWizard />);

    expect(screen.getByText("Step 1 of 3")).toBeInTheDocument();
  });

  // 5. "Skip setup" button dismisses wizard and sets localStorage
  it("dismisses the wizard and sets localStorage when Skip setup is clicked", async () => {
    const user = userEvent.setup();

    await renderWithRouter(<OnboardingWizard />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Skip setup" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(localStorage.getItem(SKIP_KEY)).toBe("true");
  });

  // 6. Submitting Step 1 form advances to Step 2
  it("advances to Step 2 after successfully creating a network", async () => {
    const user = userEvent.setup();

    await renderWithRouter(<OnboardingWizard />);

    // Fill in required fields
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(
      screen.getByPlaceholderText("192.168.1.0/24"),
      "10.0.0.0/8",
    );

    await user.click(screen.getByRole("button", { name: "Add network" }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });
  });

  // 6b. Submitting with empty name shows validation error
  it("shows a validation error when name is empty", async () => {
    const user = userEvent.setup();

    await renderWithRouter(<OnboardingWizard />);

    await user.click(screen.getByRole("button", { name: "Add network" }));

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Network name is required.",
    );
  });

  // 7. Step 2 shows discovery running state
  it("shows discovery running state after starting a scan", async () => {
    const user = userEvent.setup();

    // Job is still in the running list → scan not yet complete
    mockUseDiscoveryJobs.mockReturnValue({
      data: { data: [{ id: "job-1", status: "running" }], total: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);

    await renderWithRouter(<OnboardingWizard />);

    // Get to step 2
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(
      screen.getByPlaceholderText("192.168.1.0/24"),
      "10.0.0.0/8",
    );
    await user.click(screen.getByRole("button", { name: "Add network" }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    await user.click(
      screen.getByRole("button", { name: "Start discovery scan" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Discovery scan running…")).toBeInTheDocument();
    });
  });

  // 8. Step 2 → Step 3 transition when job completes
  //
  // Tests the completed state directly: when the discovery job has status
  // "completed", the wizard shows "Discovery scan complete." and a "View results"
  // button.
  it("shows 'Discovery scan complete' and View results button when job finishes", async () => {
    const user = userEvent.setup();

    // Job is in the list with status "completed" → scan complete immediately.
    mockUseDiscoveryJobs.mockReturnValue({
      data: { data: [{ id: "job-1", status: "completed", hosts_found: 5 }], total: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);

    await renderWithRouter(<OnboardingWizard />);

    // Navigate to step 2
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(
      screen.getByPlaceholderText("192.168.1.0/24"),
      "10.0.0.0/8",
    );
    await user.click(screen.getByRole("button", { name: "Add network" }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    await user.click(
      screen.getByRole("button", { name: "Start discovery scan" }),
    );

    // Once jobId is set and our job reports "completed", the wizard shows
    // "Discovery scan complete." and a "View results" button.
    await waitFor(() => {
      expect(screen.getByText("Discovery scan complete.")).toBeInTheDocument();
    });

    expect(
      screen.getByRole("button", { name: "View results" }),
    ).toBeInTheDocument();
  });

  // 9. Step 3 shows hosts count and navigates to /hosts
  it("shows new hosts count and navigates to /hosts when Go to Hosts is clicked", async () => {
    const user = userEvent.setup();

    // Job reports "completed" with 7 hosts found.
    mockUseDiscoveryJobs.mockReturnValue({
      data: { data: [{ id: "job-1", status: "completed", hosts_found: 7 }], total: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);

    await renderWithRouter(<OnboardingWizard />);

    // Step 1
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(
      screen.getByPlaceholderText("192.168.1.0/24"),
      "10.0.0.0/8",
    );
    await user.click(screen.getByRole("button", { name: "Add network" }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    // Step 2: trigger scan, job immediately complete
    await user.click(
      screen.getByRole("button", { name: "Start discovery scan" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Discovery scan complete.")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "View results" }));

    await waitFor(() => {
      expect(screen.getByText("Step 3 of 3")).toBeInTheDocument();
    });

    // Step 3: verify hosts found count is displayed.
    expect(screen.getByText("7")).toBeInTheDocument();

    // Clicking "Go to Hosts" navigates via TanStack Router, not a bare href.
    await user.click(screen.getByRole("button", { name: "Go to Hosts" }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/hosts" });
  });

  // 10a. Jobs query failing while scanning shows API error and Retry button
  it("shows an error when the discovery jobs query fails while scanning", async () => {
    const user = userEvent.setup();

    // First render: jobs query succeeds (no error)
    setupDefaultMocks();

    await renderWithRouter(<OnboardingWizard />);

    // Navigate to step 2
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(screen.getByPlaceholderText("192.168.1.0/24"), "10.0.0.0/8");
    await user.click(screen.getByRole("button", { name: "Add network" }));
    await waitFor(() => expect(screen.getByText("Step 2 of 3")).toBeInTheDocument());

    // Now simulate the jobs query failing after the job is started
    mockUseDiscoveryJobs.mockReturnValue({
      data: undefined,
      error: new Error("Network error"),
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);

    await user.click(screen.getByRole("button", { name: "Start discovery scan" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Failed to check scan status");
    });
  });

  // 10. Failed discovery scan shows error and retry button
  it("shows error message and Retry button when discovery scan fails", async () => {
    const user = userEvent.setup();

    // Job reports "failed".
    mockUseDiscoveryJobs.mockReturnValue({
      data: { data: [{ id: "job-1", status: "failed", hosts_found: 0 }], total: 1 },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);

    await renderWithRouter(<OnboardingWizard />);

    // Navigate to step 2
    await user.type(screen.getByPlaceholderText("Office LAN"), "My Network");
    await user.type(
      screen.getByPlaceholderText("192.168.1.0/24"),
      "10.0.0.0/8",
    );
    await user.click(screen.getByRole("button", { name: "Add network" }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    await user.click(
      screen.getByRole("button", { name: "Start discovery scan" }),
    );

    // When the job status is "failed", show an error alert and a Retry button.
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Discovery scan failed.",
      );
    });

    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();

    // Clicking Retry resets back to the "Start discovery scan" state.
    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Start discovery scan" }),
      ).toBeInTheDocument();
    });
  });
});
