import { screen } from "@testing-library/react";
import { renderWithRouter } from "../test/utils";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DashboardPage } from "./dashboard";

// Mock all the API hooks so we don't need MSW or network requests
vi.mock("../api/hooks/use-system", () => ({
  useVersion: vi.fn(),
}));

vi.mock("../api/hooks/use-networks", () => ({
  useNetworkStats: vi.fn(),
}));

vi.mock("../api/hooks/use-scans", () => ({
  useRecentScans: vi.fn(),
  useScanActivity: vi.fn(),
}));

vi.mock("../api/hooks/use-hosts", () => ({
  useActiveHostCount: vi.fn(),
}));

vi.mock("../api/hooks/use-discovery", () => ({
  useDiscoveryJobs: vi.fn(),
  useDiscoveryDiff: vi.fn(),
}));

vi.mock("../api/hooks/use-dashboard", () => ({
  useStatsSummary: vi.fn(),
}));

vi.mock("../components/activity-feed", () => ({
  ActivityFeed: () => null,
}));

import { useVersion } from "../api/hooks/use-system";
import { useNetworkStats } from "../api/hooks/use-networks";
import { useRecentScans, useScanActivity } from "../api/hooks/use-scans";
import { useActiveHostCount } from "../api/hooks/use-hosts";
import { useDiscoveryJobs, useDiscoveryDiff } from "../api/hooks/use-discovery";
import { useStatsSummary } from "../api/hooks/use-dashboard";

const mockUseVersion = vi.mocked(useVersion);
const mockUseNetworkStats = vi.mocked(useNetworkStats);
const mockUseRecentScans = vi.mocked(useRecentScans);
const mockUseScanActivity = vi.mocked(useScanActivity);
const mockUseActiveHostCount = vi.mocked(useActiveHostCount);
const mockUseDiscoveryJobs = vi.mocked(useDiscoveryJobs);
const mockUseDiscoveryDiff = vi.mocked(useDiscoveryDiff);
const mockUseStatsSummary = vi.mocked(useStatsSummary);

function setupDefaultMocks() {
  mockUseVersion.mockReturnValue({
    data: {
      version: "v0.15.0",
      service: "scanorama",
      commit: "abc1234",
      build_time: "2025-01-01T12:00:00Z",
    },
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

  mockUseScanActivity.mockReturnValue({
    data: [],
    isLoading: false,
  } as unknown as ReturnType<typeof useScanActivity>);

  mockUseDiscoveryJobs.mockReturnValue({
    data: undefined,
    isLoading: false,
  } as unknown as ReturnType<typeof useDiscoveryJobs>);

  mockUseDiscoveryDiff.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
  } as unknown as ReturnType<typeof useDiscoveryDiff>);

  mockUseStatsSummary.mockReturnValue({
    data: undefined,
    isLoading: false,
  } as unknown as ReturnType<typeof useStatsSummary>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

describe("DashboardPage", () => {
  // ── Build info card ────────────────────────────────────────────────────────

  it("shows the version number", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("v0.15.0")).toBeInTheDocument();
  });

  it("shows the commit hash", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("abc1234")).toBeInTheDocument();
  });

  it("shows the Build info label", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Build info")).toBeInTheDocument();
  });

  it("shows the Version label", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Version")).toBeInTheDocument();
  });

  it("shows the Commit label", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Commit")).toBeInTheDocument();
  });

  it("shows the Built label", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Built")).toBeInTheDocument();
  });

  it("shows dev build badge when version is dev", async () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: "dev",
        service: "scanorama",
        commit: "none",
        build_time: "unknown",
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useVersion>);
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("dev build")).toBeInTheDocument();
  });

  it("does not show dev build badge for a release version", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.queryByText("dev build")).not.toBeInTheDocument();
  });

  it("shows em-dash for commit when commit is none", async () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: "dev",
        service: "scanorama",
        commit: "none",
        build_time: "unknown",
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useVersion>);
    await renderWithRouter(<DashboardPage />);
    // em-dashes appear for both commit and build_time fields
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(2);
  });

  it("shows loading skeleton for build info while version is loading", async () => {
    mockUseVersion.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useVersion>);
    const { container } = await renderWithRouter(<DashboardPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  // ── Stat cards ─────────────────────────────────────────────────────────────

  it("renders all four stat card labels", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Networks")).toBeInTheDocument();
    expect(screen.getAllByText("Hosts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Active Hosts")).toBeInTheDocument();
    expect(screen.getByText("Exclusions")).toBeInTheDocument();
  });

  it("shows network count in stat card", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Networks")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("shows host count in stat card", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getAllByText("Hosts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("shows active host count from dedicated hook", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Active Hosts")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
  });

  it("shows exclusion count in stat card", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Exclusions")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows em-dash when stat data is unavailable", async () => {
    mockUseNetworkStats.mockReturnValue({
      data: undefined,
      isLoading: false,
    } as unknown as ReturnType<typeof useNetworkStats>);
    await renderWithRouter(<DashboardPage />);
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(3);
  });

  it("shows loading skeletons for stat cards while stats are loading", async () => {
    mockUseNetworkStats.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useNetworkStats>);
    mockUseActiveHostCount.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useActiveHostCount>);
    const { container } = await renderWithRouter(<DashboardPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  // ── Recent scans ───────────────────────────────────────────────────────────

  it("shows recent scans table heading", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Recent Scans")).toBeInTheDocument();
  });

  it("shows scan status and target in recent scans table", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  it("shows No scans found when scan list is empty", async () => {
    mockUseRecentScans.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 5, total_items: 0, total_pages: 0 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useRecentScans>);
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  it("shows loading skeleton for recent scans while scans are loading", async () => {
    mockUseRecentScans.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useRecentScans>);
    const { container } = await renderWithRouter(<DashboardPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  // ── Recent Discovery Changes widget ───────────────────────────────────────

  it("shows Recent Discovery Changes heading", async () => {
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("Recent Discovery Changes")).toBeInTheDocument();
  });

  it("shows no discovery runs yet when there are no completed jobs", async () => {
    mockUseDiscoveryJobs.mockReturnValue({
      data: {
        data: [],
        pagination: { page: 1, page_size: 1, total_items: 0, total_pages: 0 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText("No discovery runs yet.")).toBeInTheDocument();
  });

  it("shows diff counts when a completed job and diff are available", async () => {
    mockUseDiscoveryJobs.mockReturnValue({
      data: {
        data: [
          {
            id: "job-1",
            name: "Office LAN",
            networks: ["10.0.1.0/24"],
            status: "completed",
            started_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
          },
        ],
        pagination: { page: 1, page_size: 1, total_items: 1, total_pages: 1 },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);
    mockUseDiscoveryDiff.mockReturnValue({
      data: {
        job_id: "job-1",
        new_hosts: [
          {
            id: "h1",
            ip_address: "10.0.1.50",
            status: "up",
            last_seen: new Date().toISOString(),
            first_seen: new Date().toISOString(),
          },
          {
            id: "h2",
            ip_address: "10.0.1.51",
            status: "up",
            last_seen: new Date().toISOString(),
            first_seen: new Date().toISOString(),
          },
        ],
        gone_hosts: [
          {
            id: "h3",
            ip_address: "10.0.1.10",
            status: "down",
            last_seen: new Date().toISOString(),
            first_seen: new Date().toISOString(),
          },
        ],
        changed_hosts: [],
        unchanged_count: 47,
      },
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useDiscoveryDiff>);
    await renderWithRouter(<DashboardPage />);
    expect(screen.getByText(/2 new/)).toBeInTheDocument();
    expect(screen.getByText(/1 gone/)).toBeInTheDocument();
    expect(screen.getByText(/47 unchanged/)).toBeInTheDocument();
  });

  it("shows loading skeleton for the discovery widget while jobs are loading", async () => {
    mockUseDiscoveryJobs.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useDiscoveryJobs>);
    const { container } = await renderWithRouter(<DashboardPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });
});
