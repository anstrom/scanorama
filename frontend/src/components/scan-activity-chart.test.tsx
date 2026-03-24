import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ScanActivityChart } from "./scan-activity-chart";

// ── Mock recharts to avoid SVG / ResizeObserver issues in jsdom ───────────────

vi.mock("recharts", () => ({
  AreaChart: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="area-chart">{children}</div>
  ),
  Area: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="responsive-container">{children}</div>
  ),
  CartesianGrid: () => null,
}));

// ── Mock the scan activity hook ───────────────────────────────────────────────

vi.mock("../api/hooks/use-scans", () => ({
  useScanActivity: vi.fn(),
}));

import { useScanActivity } from "../api/hooks/use-scans";

const mockUseScanActivity = vi.mocked(useScanActivity);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const emptyActivityData = [
  { date: "Mon", completed: 0, failed: 0, running: 0 },
  { date: "Tue", completed: 0, failed: 0, running: 0 },
  { date: "Wed", completed: 0, failed: 0, running: 0 },
  { date: "Thu", completed: 0, failed: 0, running: 0 },
  { date: "Fri", completed: 0, failed: 0, running: 0 },
  { date: "Sat", completed: 0, failed: 0, running: 0 },
  { date: "Today", completed: 0, failed: 0, running: 0 },
];

const realActivityData = [
  { date: "Mon", completed: 3, failed: 1, running: 0 },
  { date: "Tue", completed: 0, failed: 0, running: 0 },
  { date: "Wed", completed: 2, failed: 0, running: 1 },
  { date: "Thu", completed: 1, failed: 0, running: 0 },
  { date: "Fri", completed: 0, failed: 2, running: 0 },
  { date: "Sat", completed: 4, failed: 0, running: 0 },
  { date: "Today", completed: 1, failed: 0, running: 2 },
];

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("ScanActivityChart", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows a loading skeleton when isLoading is true", () => {
    mockUseScanActivity.mockReturnValue({
      data: [],
      isLoading: true,
    });

    const { container } = render(<ScanActivityChart />);

    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
    expect(
      screen.queryByText("No scan data for the past 7 days."),
    ).not.toBeInTheDocument();
  });

  it("shows the empty-state message when all counts are zero", () => {
    mockUseScanActivity.mockReturnValue({
      data: emptyActivityData,
      isLoading: false,
    });

    const { container } = render(<ScanActivityChart />);

    expect(
      screen.getByText("No scan data for the past 7 days."),
    ).toBeInTheDocument();
    expect(container.querySelector(".animate-pulse")).not.toBeInTheDocument();
  });

  it("renders the chart heading and chart when data is present", () => {
    mockUseScanActivity.mockReturnValue({
      data: realActivityData,
      isLoading: false,
    });

    const { container } = render(<ScanActivityChart />);

    expect(screen.getByText("Scan Activity (7 days)")).toBeInTheDocument();
    expect(
      screen.queryByText("No scan data for the past 7 days."),
    ).not.toBeInTheDocument();
    expect(container.querySelector(".animate-pulse")).not.toBeInTheDocument();
    expect(screen.getByTestId("responsive-container")).toBeInTheDocument();
  });
});
