import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AdminPage } from "./admin";

vi.mock("../api/hooks/use-system", () => ({
  useAdminStatus: vi.fn(),
  useWorkers: vi.fn(),
  useVersion: vi.fn(),
}));

import {
  useAdminStatus,
  useWorkers,
  useVersion,
} from "../api/hooks/use-system";
const mockUseAdminStatus = vi.mocked(useAdminStatus);
const mockUseWorkers = vi.mocked(useWorkers);
const mockUseVersion = vi.mocked(useVersion);

beforeEach(() => {
  // Default: not loading, no data — SystemStatusCard renders real UI (including "System Status" heading)
  mockUseAdminStatus.mockReturnValue({
    data: null,
    isLoading: false,
    isError: false,
  } as any);
  mockUseWorkers.mockReturnValue({
    data: { workers: [] },
    isLoading: false,
    isError: false,
  } as any);
  mockUseVersion.mockReturnValue({ data: null, isLoading: false } as any);
});

describe("AdminPage", () => {
  it("renders the System Status section heading", () => {
    render(<AdminPage />);
    expect(screen.getByText("System Status")).toBeInTheDocument();
  });

  it("renders the Workers section heading", () => {
    render(<AdminPage />);
    expect(screen.getByText("Workers")).toBeInTheDocument();
  });

  it("shows loading state while data is fetching", () => {
    mockUseAdminStatus.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
    } as any);
    mockUseWorkers.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
    } as any);
    mockUseVersion.mockReturnValue({ data: null, isLoading: true } as any);
    render(<AdminPage />);
    // Loading skeletons should be present (no error text)
    expect(screen.queryByText(/failed to load/i)).not.toBeInTheDocument();
  });

  it("shows worker data when loaded", () => {
    mockUseWorkers.mockReturnValue({
      data: {
        workers: [
          {
            id: "w1",
            status: "idle",
            current_job: null,
            start_time: new Date().toISOString(),
          },
        ],
      },
      isLoading: false,
      isError: false,
    } as any);
    render(<AdminPage />);
    expect(screen.getByText("w1")).toBeInTheDocument();
  });

  it("shows empty state when no workers are running", () => {
    mockUseWorkers.mockReturnValue({
      data: { workers: [] },
      isLoading: false,
      isError: false,
    } as any);
    render(<AdminPage />);
    expect(screen.getByText(/no workers running/i)).toBeInTheDocument();
  });
});
