/* eslint-disable @typescript-eslint/no-explicit-any */
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AdminPage } from "./admin";

// Mock WsManager so LogViewer doesn't try to open a real WebSocket.
// The class is defined inline inside the factory to avoid hoisting issues
// (vi.mock factories are hoisted to the top of the file before any declarations).
vi.mock("../lib/ws", () => ({
  WsManager: class {
    connect() {}
    disconnect() {}
    on() {
      return () => {};
    }
    onStatusChange() {
      return () => {};
    }
    getStatus() {
      return "disconnected";
    }
  },
}));

vi.mock("../api/hooks/use-system", () => ({
  useAdminStatus: vi.fn(),
  useWorkers: vi.fn(),
  useVersion: vi.fn(),
  useLogs: vi.fn(),
}));

import {
  useAdminStatus,
  useWorkers,
  useVersion,
  useLogs,
} from "../api/hooks/use-system";
const mockUseAdminStatus = vi.mocked(useAdminStatus);
const mockUseWorkers = vi.mocked(useWorkers);
const mockUseVersion = vi.mocked(useVersion);
const mockUseLogs = vi.mocked(useLogs);

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
  mockUseLogs.mockReturnValue({
    data: {
      data: [],
      pagination: { page: 1, page_size: 50, total_items: 0, total_pages: 0 },
    },
    isLoading: false,
    isError: false,
  } as any);
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
