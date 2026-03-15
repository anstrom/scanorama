import { screen, waitFor } from "@testing-library/react";
import { renderWithRouter } from "../test/utils";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DashboardPage } from "./dashboard";

// Mock all the API hooks so we don't need MSW or network requests
vi.mock("../api/hooks/use-system", () => ({
  useHealth: vi.fn(),
  useVersion: vi.fn(),
}));

vi.mock("../api/hooks/use-networks", () => ({
  useNetworkStats: vi.fn(),
}));

vi.mock("../api/hooks/use-scans", () => ({
  useRecentScans: vi.fn(),
}));

vi.mock("../api/hooks/use-hosts", () => ({
  useActiveHostCount: vi.fn(),
}));

import { useHealth, useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { useRecentScans } from "../api/hooks/use-scans";
import { useActiveHostCount } from "../api/hooks/use-hosts";

const mockUseHealth = vi.mocked(useHealth);
const mockUseVersion = vi.mocked(useVersion);
const mockUseNetworkStats = vi.mocked(useNetworkStats);
const mockUseRecentScans = vi.mocked(useRecentScans);
const mockUseActiveHostCount = vi.mocked(useActiveHostCount);

function setupDefaultMocks() {
  mockUseHealth.mockReturnValue({
    data: { status: "healthy", uptime: "2h30m" },
    isLoading: false,
  } as unknown as ReturnType<typeof useHealth>);

  mockUseVersion.mockReturnValue({
    data: { version: "0.7.0", service: "scanorama" },
    isLoading: false,
  } as unknown as ReturnType<typeof useVersion>);

  mockUseNetworkStats.mockReturnValue({
    data: {
      networks: { total: 5 },
      hosts: { total: 42, active: 30 },
      exclusions: { total: 3 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useNetworkStats>);

  mockUseRecentScans.mockReturnValue({
    data: {
      data: [
        {
          id: "s1",
          status: "completed",
          targets: ["192.168.1.0/24"],
          hosts_discovered: 10,
          ports_scanned: 100,
          created_at: "2024-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, page_size: 5, total_items: 1, total_pages: 1 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useRecentScans>);

  mockUseActiveHostCount.mockReturnValue({
    data: 30,
    isLoading: false,
  } as unknown as ReturnType<typeof useActiveHostCount>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

describe("DashboardPage", () => {
  it("renders the System heading", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("System")).toBeInTheDocument();
  });

  it("shows healthy status badge", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("healthy")).toBeInTheDocument();
  });

  it("shows version number", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("0.7.0")).toBeInTheDocument();
  });

  it("shows network count in stat card", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Networks")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("shows host count in stat card", () => {
    renderWithRouter(<DashboardPage />);
    // "Hosts" appears in both the stat card label and the scans table header
    expect(screen.getAllByText("Hosts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("shows active host count from dedicated hook", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Active Hosts")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
  });

  it("shows exclusion count in stat card", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Exclusions")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows recent scans table heading", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Recent Scans")).toBeInTheDocument();
  });

  it("shows scan status and target in recent scans table", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  it("shows error badge when health check fails", () => {
    mockUseHealth.mockReturnValue({
      data: null,
      isLoading: false,
    } as unknown as ReturnType<typeof useHealth>);
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("shows Checking... while health is loading", () => {
    mockUseHealth.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useHealth>);
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Checking...")).toBeInTheDocument();
  });

  it("shows loading skeletons for stat cards while stats are loading", () => {
    mockUseNetworkStats.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useNetworkStats>);
    mockUseActiveHostCount.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useActiveHostCount>);
    const { container } = renderWithRouter(<DashboardPage />);
    // Loading skeleton uses animate-pulse; there should be at least one
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows loading skeleton for recent scans while scans are loading", () => {
    mockUseRecentScans.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useRecentScans>);
    renderWithRouter(<DashboardPage />);
    const skeletons = document.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("does not show version when version data is absent", () => {
    mockUseVersion.mockReturnValue({
      data: undefined,
      isLoading: false,
    } as unknown as ReturnType<typeof useVersion>);
    renderWithRouter(<DashboardPage />);
    expect(screen.queryByText("0.7.0")).not.toBeInTheDocument();
  });

  it("shows em-dash when stat data is unavailable", () => {
    mockUseNetworkStats.mockReturnValue({
      data: undefined,
      isLoading: false,
    } as unknown as ReturnType<typeof useNetworkStats>);
    renderWithRouter(<DashboardPage />);
    // All four stat cards should show the em-dash fallback
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(3);
  });

  it("shows No scans found when scan list is empty", () => {
    mockUseRecentScans.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 5, total_items: 0, total_pages: 0 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useRecentScans>);
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  it("renders all four stat card labels", () => {
    renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Networks")).toBeInTheDocument();
    // "Hosts" appears in both the stat card label and the scans table header
    expect(screen.getAllByText("Hosts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Active Hosts")).toBeInTheDocument();
    expect(screen.getByText("Exclusions")).toBeInTheDocument();
  });

  it("waitFor still works with async state transitions", async () => {
    // Start in loading state then update to resolved
    mockUseHealth.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useHealth>);

    renderWithRouter(<DashboardPage />);

    expect(screen.getByText("Checking...")).toBeInTheDocument();

    // Update the mock — on the next render cycle the component will re-read it
    mockUseHealth.mockReturnValue({
      data: { status: "healthy" },
      isLoading: false,
    } as unknown as ReturnType<typeof useHealth>);

    // Re-render by triggering a state change via a sibling mock update
    mockUseNetworkStats.mockReturnValue({
      data: {
        networks: { total: 5 },
        hosts: { total: 42, active: 30 },
        exclusions: { total: 3 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useNetworkStats>);

    renderWithRouter(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("healthy")).toBeInTheDocument();
    });
  });
});
