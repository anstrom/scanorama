import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { SchedulesPage } from "./schedules";

// ── Mock hooks ────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-schedules", () => ({
  useSchedules: vi.fn(),
  useEnableSchedule: vi.fn(),
  useDisableSchedule: vi.fn(),
  useDeleteSchedule: vi.fn(),
  useUpdateSchedule: vi.fn(),
}));

vi.mock("../components/create-schedule-modal", () => ({
  CreateScheduleModal: ({
    onClose,
  }: {
    onClose: () => void;
    onCreated?: () => void;
  }) => (
    <div role="dialog" aria-label="Create Schedule">
      <button type="button" onClick={onClose}>
        Close modal
      </button>
    </div>
  ),
  ScheduleFormModal: ({ onClose }: { onClose: () => void; mode?: string }) => (
    <div role="dialog" aria-label="Schedule Form">
      <button type="button" onClick={onClose}>
        Close modal
      </button>
    </div>
  ),
}));

vi.mock("../components/column-toggle", () => ({
  ColumnToggle: ({
    columns,
    onToggle,
  }: {
    columns: Array<{ key: string; label: string; alwaysVisible?: boolean }>;
    onToggle: (key: string) => void;
  }) => (
    <div aria-label="column-toggle">
      {columns
        .filter((c) => !c.alwaysVisible)
        .map((c) => (
          <button key={c.key} type="button" onClick={() => onToggle(c.key)}>
            Toggle {c.label}
          </button>
        ))}
    </div>
  ),
}));

import {
  useSchedules,
  useEnableSchedule,
  useDisableSchedule,
  useDeleteSchedule,
  useUpdateSchedule,
} from "../api/hooks/use-schedules";

const mockUseSchedules = vi.mocked(useSchedules);
const mockUseEnableSchedule = vi.mocked(useEnableSchedule);
const mockUseDisableSchedule = vi.mocked(useDisableSchedule);
const mockUseDeleteSchedule = vi.mocked(useDeleteSchedule);
const mockUseUpdateSchedule = vi.mocked(useUpdateSchedule);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockSchedules = [
  {
    id: "sched-1",
    name: "Daily Scan",
    cron_expr: "0 2 * * *",
    enabled: true,
    network_name: "Office LAN",
    next_run: new Date(Date.now() + 3_600_000).toISOString(),
    last_run: new Date(Date.now() - 3_600_000).toISOString(),
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-06-01T00:00:00Z",
    profile_id: "profile-1",
  },
  {
    id: "sched-2",
    name: "Weekly Recon",
    cron_expr: "0 0 * * 1",
    enabled: false,
    network_name: "DMZ Network",
    next_run: undefined,
    last_run: undefined,
    created_at: "2024-02-01T00:00:00Z",
    updated_at: "2024-06-02T00:00:00Z",
    profile_id: undefined,
  },
  {
    id: "sched-3",
    name: "Hourly Check",
    cron_expr: "0 * * * *",
    enabled: true,
    network_name: undefined,
    next_run: undefined,
    last_run: undefined,
    created_at: "2024-03-01T00:00:00Z",
    updated_at: undefined,
    profile_id: undefined,
  },
];

const mockPagination = {
  page: 1,
  page_size: 25,
  total_items: 3,
  total_pages: 1,
};

const idleMutation = {
  mutateAsync: vi.fn(),
  mutate: vi.fn(),
  isPending: false,
  isSuccess: false,
  isError: false,
};

function makeUseSchedulesResult(overrides = {}) {
  return {
    data: {
      data: mockSchedules,
      pagination: mockPagination,
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useSchedules>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseSchedules.mockReturnValue(makeUseSchedulesResult());
  mockUseEnableSchedule.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useEnableSchedule>,
  );
  mockUseDisableSchedule.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDisableSchedule>,
  );
  mockUseDeleteSchedule.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDeleteSchedule>,
  );
  mockUseUpdateSchedule.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useUpdateSchedule>,
  );
});

// ── Toolbar ───────────────────────────────────────────────────────────────────

describe("SchedulesPage — toolbar", () => {
  it("renders status filter select", () => {
    render(<SchedulesPage />);
    expect(
      screen.getByRole("combobox", { name: /filter by status/i }),
    ).toBeInTheDocument();
  });

  it("status filter has All, Enabled, Disabled options", () => {
    render(<SchedulesPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    expect(within(select).getByText("All")).toBeInTheDocument();
    expect(within(select).getByText("Enabled")).toBeInTheDocument();
    expect(within(select).getByText("Disabled")).toBeInTheDocument();
  });

  it("renders Create schedule button", () => {
    render(<SchedulesPage />);
    expect(
      screen.getByRole("button", { name: /create schedule/i }),
    ).toBeInTheDocument();
  });
});

// ── Loading ───────────────────────────────────────────────────────────────────

describe("SchedulesPage — loading", () => {
  it("renders skeleton rows when loading", () => {
    mockUseSchedules.mockReturnValue(
      makeUseSchedulesResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<SchedulesPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("does not show 'No schedules found.' while loading", () => {
    mockUseSchedules.mockReturnValue(
      makeUseSchedulesResult({ isLoading: true, data: undefined }),
    );
    render(<SchedulesPage />);
    expect(screen.queryByText("No schedules found.")).not.toBeInTheDocument();
  });
});

// ── Empty state ───────────────────────────────────────────────────────────────

describe("SchedulesPage — empty state", () => {
  it("shows 'No schedules found.' when the list is empty", () => {
    mockUseSchedules.mockReturnValue(
      makeUseSchedulesResult({
        data: {
          data: [],
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 0,
            total_pages: 0,
          },
        },
      }),
    );
    render(<SchedulesPage />);
    expect(screen.getByText("No schedules found.")).toBeInTheDocument();
  });
});

// ── Table structure ───────────────────────────────────────────────────────────

describe("SchedulesPage — table structure", () => {
  it("renders the schedules table", () => {
    render(<SchedulesPage />);
    expect(screen.getByRole("table")).toBeInTheDocument();
  });

  it("renders all column headers", () => {
    render(<SchedulesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Cron" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Next Run" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Last Run" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Network" }),
    ).toBeInTheDocument();
  });

  it("renders one data row per schedule", () => {
    render(<SchedulesPage />);
    const rows = screen.getAllByRole("row");
    // 1 header + 3 data rows
    expect(rows.length).toBe(4);
  });
});

// ── Row data ──────────────────────────────────────────────────────────────────

describe("SchedulesPage — row data", () => {
  it("renders schedule names", () => {
    render(<SchedulesPage />);
    expect(screen.getByText("Daily Scan")).toBeInTheDocument();
    expect(screen.getByText("Weekly Recon")).toBeInTheDocument();
    expect(screen.getByText("Hourly Check")).toBeInTheDocument();
  });

  it("renders describeCron output for cron expressions", () => {
    render(<SchedulesPage />);
    // "0 2 * * *" => "Every day at 02:00"
    expect(screen.getByText("Every day at 02:00")).toBeInTheDocument();
    // "0 0 * * 1" => "Every Monday at 00:00"
    expect(screen.getByText("Every Monday at 00:00")).toBeInTheDocument();
  });

  it("renders 'enabled' badge for enabled schedules", () => {
    render(<SchedulesPage />);
    const badges = screen.getAllByText("enabled");
    expect(badges.length).toBeGreaterThan(0);
  });

  it("renders 'disabled' badge for disabled schedules", () => {
    render(<SchedulesPage />);
    expect(screen.getByText("disabled")).toBeInTheDocument();
  });

  it("shows network_name for schedules with a linked network", () => {
    render(<SchedulesPage />);
    expect(screen.getByText("Office LAN")).toBeInTheDocument();
    expect(screen.getByText("DMZ Network")).toBeInTheDocument();
  });

  it("shows em-dash for schedules with no network_name", () => {
    render(<SchedulesPage />);
    // "Hourly Check" has no network_name — should show em-dash in Network column
    const rows = screen.getAllByRole("row");
    const hourlyRow = rows.find((r) => within(r).queryByText("Hourly Check"));
    expect(hourlyRow).toBeTruthy();
    // The network cell should contain an em-dash
    const cells = within(hourlyRow!).getAllByRole("cell");
    // Network is the last column (index 5)
    expect(cells[5].textContent).toBe("—");
  });

  it("shows em-dash for missing next_run and last_run", () => {
    render(<SchedulesPage />);
    const rows = screen.getAllByRole("row");
    const weeklyRow = rows.find((r) => within(r).queryByText("Weekly Recon"));
    expect(weeklyRow).toBeTruthy();
    const cells = within(weeklyRow!).getAllByRole("cell");
    // Next Run is index 2, Last Run is index 3
    expect(cells[2].textContent).toBe("—");
    expect(cells[3].textContent).toBe("—");
  });
});

// ── Filter ────────────────────────────────────────────────────────────────────

describe("SchedulesPage — filter", () => {
  it("selecting 'enabled' passes enabled: true to useSchedules", async () => {
    const user = userEvent.setup();
    render(<SchedulesPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await user.selectOptions(select, "enabled");
    const lastCall = mockUseSchedules.mock.lastCall?.[0];
    expect(lastCall).toMatchObject({ enabled: true });
  });

  it("selecting 'disabled' passes enabled: false to useSchedules", async () => {
    const user = userEvent.setup();
    render(<SchedulesPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    await user.selectOptions(select, "disabled");
    const lastCall = mockUseSchedules.mock.lastCall?.[0];
    expect(lastCall).toMatchObject({ enabled: false });
  });

  it("selecting 'all' does not include enabled in params", async () => {
    const user = userEvent.setup();
    render(<SchedulesPage />);
    const select = screen.getByRole("combobox", { name: /filter by status/i });
    // Switch to enabled then back to all
    await user.selectOptions(select, "enabled");
    await user.selectOptions(select, "all");
    const lastCall = mockUseSchedules.mock.lastCall?.[0];
    expect(lastCall).not.toHaveProperty("enabled");
  });
});

// ── Detail panel ──────────────────────────────────────────────────────────────

describe("SchedulesPage — detail panel", () => {
  it("opens the detail panel when a row is clicked", async () => {
    render(<SchedulesPage />);
    const row = screen.getByText("Daily Scan").closest("tr")!;
    await userEvent.click(row);
    expect(
      screen.getByRole("dialog", { name: /schedule details/i }),
    ).toBeInTheDocument();
  });

  it("shows the schedule name in the detail panel", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    expect(within(panel).getByText("Daily Scan")).toBeInTheDocument();
  });

  it("shows the close button in the detail panel", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    expect(
      within(panel).getByRole("button", { name: /close panel/i }),
    ).toBeInTheDocument();
  });

  it("closes the detail panel when the close button is clicked", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    await userEvent.click(
      within(panel).getByRole("button", { name: /close panel/i }),
    );
    expect(
      screen.queryByRole("dialog", { name: /schedule details/i }),
    ).not.toBeInTheDocument();
  });

  it("shows Disable button for enabled schedule", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    expect(
      within(panel).getByRole("button", { name: /^disable$/i }),
    ).toBeInTheDocument();
  });

  it("shows Enable button for disabled schedule", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Weekly Recon").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    expect(
      within(panel).getByRole("button", { name: /^enable$/i }),
    ).toBeInTheDocument();
  });

  it("shows Delete button in the detail panel", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    expect(
      within(panel).getByRole("button", { name: /^delete$/i }),
    ).toBeInTheDocument();
  });

  it("shows delete confirm prompt after clicking Delete", async () => {
    render(<SchedulesPage />);
    await userEvent.click(screen.getByText("Daily Scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /schedule details/i });
    const deleteBtn = within(panel).getByRole("button", { name: /^delete$/i });
    await userEvent.click(deleteBtn);
    expect(within(panel).getByText(/confirm delete/i)).toBeInTheDocument();
  });
});

// ── Add modal ─────────────────────────────────────────────────────────────────

describe("SchedulesPage — add modal", () => {
  it("opens ScheduleFormModal when Create schedule is clicked", async () => {
    render(<SchedulesPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /create schedule/i }),
    );
    expect(
      screen.getByRole("dialog", { name: /schedule form/i }),
    ).toBeInTheDocument();
  });

  it("closes ScheduleFormModal when its close button is clicked", async () => {
    render(<SchedulesPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /create schedule/i }),
    );
    const modal = screen.getByRole("dialog", { name: /schedule form/i });
    await userEvent.click(
      within(modal).getByRole("button", { name: /close modal/i }),
    );
    expect(
      screen.queryByRole("dialog", { name: /schedule form/i }),
    ).not.toBeInTheDocument();
  });
});

// ── Column visibility ─────────────────────────────────────────────────────────

describe("SchedulesPage — column visibility", () => {
  it("renders optional columns visible by default", () => {
    render(<SchedulesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Next Run" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Last Run" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Network" }),
    ).toBeInTheDocument();
  });

  it("hides the Next Run column when its toggle is clicked", async () => {
    render(<SchedulesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Next Run" }),
    ).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Toggle Next Run" }),
    );
    expect(
      screen.queryByRole("columnheader", { name: "Next Run" }),
    ).not.toBeInTheDocument();
  });
});
