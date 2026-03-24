import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ScheduleFormModal } from "./create-schedule-modal";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-schedules", () => ({
  useCreateSchedule: vi.fn(),
  useUpdateSchedule: vi.fn(),
}));

vi.mock("../api/hooks/use-networks", () => ({
  useNetworks: vi.fn(),
}));

import { useCreateSchedule, useUpdateSchedule } from "../api/hooks/use-schedules";
import { useNetworks } from "../api/hooks/use-networks";

const mockUseCreateSchedule = vi.mocked(useCreateSchedule);
const mockUseUpdateSchedule = vi.mocked(useUpdateSchedule);
const mockUseNetworks = vi.mocked(useNetworks);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockNetworksList = [
  { id: "net-1", name: "Office LAN", cidr: "192.168.1.0/24" },
];

function setupDefaultMocks() {
  mockUseCreateSchedule.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  } as unknown as ReturnType<typeof useCreateSchedule>);

  mockUseUpdateSchedule.mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  } as unknown as ReturnType<typeof useUpdateSchedule>);

  mockUseNetworks.mockReturnValue({
    data: {
      data: mockNetworksList,
      pagination: { page: 1, page_size: 100, total_items: 1, total_pages: 1 },
    },
    isLoading: false,
  } as unknown as ReturnType<typeof useNetworks>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setupDefaultMocks();
});

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("ScheduleFormModal — create mode", () => {
  it("shows 'Create Schedule' title and 'Create schedule' submit button", () => {
    render(
      <ScheduleFormModal mode="create" onClose={vi.fn()} />,
    );
    expect(
      screen.getByRole("heading", { name: "Create Schedule" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create schedule" }),
    ).toBeInTheDocument();
  });
});

describe("ScheduleFormModal — edit mode", () => {
  const editInitial = {
    id: "sched-1",
    name: "My Schedule",
    cron_expr: "0 * * * *",
    network_id: "net-1",
    type: "scan" as const,
    enabled: true,
  };

  it("shows 'Edit Schedule' title, 'Save changes' button, and pre-populated name", () => {
    render(
      <ScheduleFormModal
        mode="edit"
        initial={editInitial}
        onClose={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Edit Schedule" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save changes" }),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("My Schedule")).toBeInTheDocument();
  });

  it("calls useUpdateSchedule (not useCreateSchedule) on submit", async () => {
    const user = userEvent.setup();
    const mockUpdateAsync = vi.fn().mockResolvedValue({});
    mockUseUpdateSchedule.mockReturnValue({
      mutateAsync: mockUpdateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateSchedule>);

    render(
      <ScheduleFormModal
        mode="edit"
        initial={editInitial}
        onClose={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mockUpdateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "sched-1",
          body: expect.objectContaining({
            name: "My Schedule",
            cron_expr: "0 * * * *",
            network_id: "net-1",
          }),
        }),
      );
    });

    // Ensure create was not called
    const createMock = vi.mocked(useCreateSchedule)().mutateAsync;
    expect(createMock).not.toHaveBeenCalled();
  });
});
