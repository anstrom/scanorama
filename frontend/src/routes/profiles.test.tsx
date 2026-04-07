import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ProfilesPage } from "./profiles";

// ── Mock hooks ────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-profiles", () => ({
  useProfiles: vi.fn(),
  useDeleteProfile: vi.fn(),
}));

vi.mock("../components/profile-form-modal", () => ({
  ProfileFormModal: ({ onClose }: { onClose: () => void }) => (
    <div role="dialog" aria-label="Profile Form">
      <button type="button" onClick={onClose}>
        Close profile form
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

import { useProfiles, useDeleteProfile } from "../api/hooks/use-profiles";

const mockUseProfiles = vi.mocked(useProfiles);
const mockUseDeleteProfile = vi.mocked(useDeleteProfile);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockProfiles = [
  {
    id: "p1",
    name: "Quick scan",
    scan_type: "connect",
    ports: "22,80,443",
    description: "Fast TCP connect scan",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: new Date(Date.now() - 3_600_000).toISOString(),
  },
  {
    id: "p2",
    name: "Full scan",
    scan_type: "syn",
    ports: "1-65535",
    description: "Comprehensive SYN scan",
    created_at: "2024-01-02T00:00:00Z",
    updated_at: "2024-06-01T00:00:00Z",
  },
  {
    id: "p3",
    name: "UDP check",
    scan_type: "udp",
    ports: undefined,
    description: undefined,
    created_at: "2024-01-03T00:00:00Z",
    updated_at: undefined,
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

function makeUseProfilesResult(overrides = {}) {
  return {
    data: {
      data: mockProfiles,
      pagination: mockPagination,
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useProfiles>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseProfiles.mockReturnValue(makeUseProfilesResult());
  mockUseDeleteProfile.mockReturnValue(
    idleMutation as unknown as ReturnType<typeof useDeleteProfile>,
  );
});

// ── Toolbar ───────────────────────────────────────────────────────────────────

describe("ProfilesPage — toolbar", () => {
  it("renders search input with label 'Search by name'", () => {
    render(<ProfilesPage />);
    expect(
      screen.getByRole("textbox", { name: /search by name/i }),
    ).toBeInTheDocument();
  });

  it("renders the Create Profile button", () => {
    render(<ProfilesPage />);
    expect(
      screen.getByRole("button", { name: /create profile/i }),
    ).toBeInTheDocument();
  });
});

// ── Loading state ─────────────────────────────────────────────────────────────

describe("ProfilesPage — loading", () => {
  it("renders >=6 skeleton rows when loading", () => {
    mockUseProfiles.mockReturnValue(
      makeUseProfilesResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<ProfilesPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(6);
  });

  it("does not show 'No profiles found.' while loading", () => {
    mockUseProfiles.mockReturnValue(
      makeUseProfilesResult({ isLoading: true, data: undefined }),
    );
    render(<ProfilesPage />);
    expect(screen.queryByText("No profiles found.")).not.toBeInTheDocument();
  });
});

// ── Empty state ───────────────────────────────────────────────────────────────

describe("ProfilesPage — empty state", () => {
  it("shows 'No profiles found.' when the list is empty", () => {
    mockUseProfiles.mockReturnValue(
      makeUseProfilesResult({
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
    render(<ProfilesPage />);
    expect(screen.getByText("No profiles found.")).toBeInTheDocument();
  });
});

// ── Table structure ───────────────────────────────────────────────────────────

describe("ProfilesPage — table structure", () => {
  it("renders all column headers", () => {
    render(<ProfilesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Scan Type" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Ports" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Description" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Updated" }),
    ).toBeInTheDocument();
  });

  it("renders one data row per profile", () => {
    render(<ProfilesPage />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
  });
});

// ── Row data ──────────────────────────────────────────────────────────────────

describe("ProfilesPage — row data", () => {
  it("renders profile names", () => {
    render(<ProfilesPage />);
    expect(screen.getByText("Quick scan")).toBeInTheDocument();
    expect(screen.getByText("Full scan")).toBeInTheDocument();
    expect(screen.getByText("UDP check")).toBeInTheDocument();
  });

  it("renders short scan type labels", () => {
    render(<ProfilesPage />);
    expect(screen.getAllByText("Connect").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("SYN")).toBeInTheDocument();
    expect(screen.getByText("UDP")).toBeInTheDocument();
  });

  it("renders ports values", () => {
    render(<ProfilesPage />);
    expect(screen.getByText("22,80,443")).toBeInTheDocument();
    expect(screen.getByText("1-65535")).toBeInTheDocument();
  });

  it("shows em-dash for missing ports", () => {
    render(<ProfilesPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    expect(cells[2]).toHaveTextContent("\u2014");
  });

  it("shows relative time for updated_at when present", () => {
    render(<ProfilesPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[1]).getAllByRole("cell");
    expect(cells[4].textContent).toMatch(/ago|just now/i);
  });

  it("shows em-dash for missing updated_at", () => {
    render(<ProfilesPage />);
    const rows = screen.getAllByRole("row");
    const cells = within(rows[3]).getAllByRole("cell");
    expect(cells[4]).toHaveTextContent("\u2014");
  });
});

// ── Pagination ────────────────────────────────────────────────────────────────

describe("ProfilesPage — pagination", () => {
  it("shows pagination when there are multiple pages", () => {
    mockUseProfiles.mockReturnValue(
      makeUseProfilesResult({
        data: {
          data: mockProfiles,
          pagination: {
            page: 1,
            page_size: 25,
            total_items: 60,
            total_pages: 3,
          },
        },
      }),
    );
    render(<ProfilesPage />);
    expect(screen.getByText("Page 1 of 3")).toBeInTheDocument();
  });

  it("does not show pagination when there is only one page", () => {
    render(<ProfilesPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });

  it("does not show pagination when the list is empty", () => {
    mockUseProfiles.mockReturnValue(
      makeUseProfilesResult({
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
    render(<ProfilesPage />);
    expect(screen.queryByText(/Page \d+ of \d+/)).not.toBeInTheDocument();
  });
});

// ── Detail panel ──────────────────────────────────────────────────────────────

describe("ProfilesPage — detail panel", () => {
  it("opens the detail panel when a row is clicked", async () => {
    render(<ProfilesPage />);
    const row = screen.getByText("Quick scan").closest("tr")!;
    await userEvent.click(row);
    expect(
      screen.getByRole("dialog", { name: /profile details/i }),
    ).toBeInTheDocument();
  });

  it("shows the profile name in the panel", async () => {
    render(<ProfilesPage />);
    await userEvent.click(screen.getByText("Quick scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /profile details/i });
    expect(within(panel).getByText("Quick scan")).toBeInTheDocument();
  });

  it("closes the detail panel when the close button is clicked", async () => {
    render(<ProfilesPage />);
    await userEvent.click(screen.getByText("Quick scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /profile details/i });
    const closeBtn = within(panel).getByRole("button", {
      name: /close panel/i,
    });
    await userEvent.click(closeBtn);
    expect(
      screen.queryByRole("dialog", { name: /profile details/i }),
    ).not.toBeInTheDocument();
  });

  it("shows Edit button in the detail panel", async () => {
    render(<ProfilesPage />);
    await userEvent.click(screen.getByText("Quick scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /profile details/i });
    expect(
      within(panel).getByRole("button", { name: /edit/i }),
    ).toBeInTheDocument();
  });

  it("opens ProfileFormModal when Edit is clicked", async () => {
    render(<ProfilesPage />);
    await userEvent.click(screen.getByText("Quick scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /profile details/i });
    await userEvent.click(within(panel).getByRole("button", { name: /edit/i }));
    expect(
      screen.getByRole("dialog", { name: /profile form/i }),
    ).toBeInTheDocument();
  });

  it("shows Delete button then confirm prompt on click", async () => {
    render(<ProfilesPage />);
    await userEvent.click(screen.getByText("Quick scan").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /profile details/i });
    const deleteBtn = within(panel).getByRole("button", { name: /^delete$/i });
    await userEvent.click(deleteBtn);
    expect(within(panel).getByText("Delete this profile?")).toBeInTheDocument();
  });
});

// ── Add modal ─────────────────────────────────────────────────────────────────

describe("ProfilesPage — add modal", () => {
  it("opens ProfileFormModal when Create Profile is clicked", async () => {
    render(<ProfilesPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /create profile/i }),
    );
    expect(
      screen.getByRole("dialog", { name: /profile form/i }),
    ).toBeInTheDocument();
  });

  it("closes ProfileFormModal when its onClose fires", async () => {
    render(<ProfilesPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /create profile/i }),
    );
    const modal = screen.getByRole("dialog", { name: /profile form/i });
    await userEvent.click(
      within(modal).getByRole("button", { name: /close profile form/i }),
    );
    expect(
      screen.queryByRole("dialog", { name: /profile form/i }),
    ).not.toBeInTheDocument();
  });
});

// ── Table render ──────────────────────────────────────────────────────────────

describe("ProfilesPage — table render", () => {
  it("renders the profiles table", () => {
    render(<ProfilesPage />);
    expect(screen.getByRole("table")).toBeInTheDocument();
  });
});

// ── Column visibility ─────────────────────────────────────────────────────────

describe("ProfilesPage — column visibility", () => {
  it("renders optional columns visible by default", () => {
    render(<ProfilesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Ports" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Description" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Updated" }),
    ).toBeInTheDocument();
  });

  it("hides the Ports column when its toggle button is clicked", async () => {
    render(<ProfilesPage />);
    expect(
      screen.getByRole("columnheader", { name: "Ports" }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Toggle Ports" }));
    expect(
      screen.queryByRole("columnheader", { name: "Ports" }),
    ).not.toBeInTheDocument();
  });
});
