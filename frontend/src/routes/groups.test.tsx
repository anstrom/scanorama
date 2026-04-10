import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { GroupsPage } from "./groups";

// ── Mock hooks ────────────────────────────────────────────────────────────────

vi.mock("../api/hooks/use-groups", () => ({
  useGroups: vi.fn(),
  useGroupMembers: vi.fn(),
  useCreateGroup: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useUpdateGroup: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useDeleteGroup: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useRemoveHostsFromGroup: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock("../components/toast-provider", () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn() },
  }),
}));

vi.mock("../components", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../components")>();
  return {
    ...actual,
    PaginationBar: ({ page, totalPages }: { page: number; totalPages: number }) => (
      <div>{`Page ${page} of ${totalPages}`}</div>
    ),
  };
});

import {
  useGroups,
  useGroupMembers,
} from "../api/hooks/use-groups";

const mockUseGroups = vi.mocked(useGroups);
const mockUseGroupMembers = vi.mocked(useGroupMembers);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockGroups = [
  {
    id: "g1",
    name: "Production",
    description: "Production hosts",
    color: "#ef4444",
    member_count: 5,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: new Date(Date.now() - 3_600_000).toISOString(),
  },
  {
    id: "g2",
    name: "Development",
    description: undefined,
    color: "#22c55e",
    member_count: 3,
    created_at: "2024-01-02T00:00:00Z",
    updated_at: "2024-06-01T00:00:00Z",
  },
];

const mockMembers = [
  {
    id: "h1",
    ip_address: "192.168.1.1",
    hostname: "server-1",
    status: "up",
    tags: ["web"],
    last_seen: new Date(Date.now() - 60_000).toISOString(),
  },
  {
    id: "h2",
    ip_address: "192.168.1.2",
    hostname: undefined,
    status: "down",
    tags: [],
    last_seen: "2024-06-01T00:00:00Z",
  },
];

function makeUseGroupsResult(overrides = {}) {
  return {
    data: mockGroups,
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useGroups>;
}

function makeUseGroupMembersResult(overrides = {}) {
  return {
    data: {
      data: mockMembers,
      pagination: { total_pages: 1, total: 2 },
    },
    isLoading: false,
    ...overrides,
  } as unknown as ReturnType<typeof useGroupMembers>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseGroups.mockReturnValue(makeUseGroupsResult());
  mockUseGroupMembers.mockReturnValue(makeUseGroupMembersResult());
});

// ── Toolbar ───────────────────────────────────────────────────────────────────

describe("GroupsPage — toolbar", () => {
  it("renders search input", () => {
    render(<GroupsPage />);
    expect(screen.getByRole("textbox", { name: /search groups/i })).toBeInTheDocument();
  });

  it("renders the Create group button", () => {
    render(<GroupsPage />);
    expect(screen.getByRole("button", { name: /create group/i })).toBeInTheDocument();
  });
});

// ── Loading state ─────────────────────────────────────────────────────────────

describe("GroupsPage — loading", () => {
  it("renders skeleton rows when loading", () => {
    mockUseGroups.mockReturnValue(
      makeUseGroupsResult({ isLoading: true, data: undefined }),
    );
    const { container } = render(<GroupsPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(4);
  });
});

// ── Empty state ───────────────────────────────────────────────────────────────

describe("GroupsPage — empty state", () => {
  it("shows 'No groups found.' when list is empty", () => {
    mockUseGroups.mockReturnValue(makeUseGroupsResult({ data: [] }));
    render(<GroupsPage />);
    expect(screen.getByText("No groups found.")).toBeInTheDocument();
  });
});

// ── Table ─────────────────────────────────────────────────────────────────────

describe("GroupsPage — table", () => {
  it("renders all column headers", () => {
    render(<GroupsPage />);
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Description" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Members" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Updated" })).toBeInTheDocument();
  });

  it("renders group names", () => {
    render(<GroupsPage />);
    expect(screen.getByText("Production")).toBeInTheDocument();
    expect(screen.getByText("Development")).toBeInTheDocument();
  });

  it("renders member counts", () => {
    render(<GroupsPage />);
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("renders one row per group plus header", () => {
    render(<GroupsPage />);
    expect(screen.getAllByRole("row")).toHaveLength(3);
  });
});

// ── Search ────────────────────────────────────────────────────────────────────

describe("GroupsPage — search", () => {
  it("filters groups by name", async () => {
    render(<GroupsPage />);
    await userEvent.type(screen.getByRole("textbox", { name: /search groups/i }), "Prod");
    expect(screen.getByText("Production")).toBeInTheDocument();
    expect(screen.queryByText("Development")).not.toBeInTheDocument();
  });

  it("shows 'No groups match your search.' when no results", async () => {
    render(<GroupsPage />);
    await userEvent.type(
      screen.getByRole("textbox", { name: /search groups/i }),
      "xyznonexistent",
    );
    expect(screen.getByText("No groups match your search.")).toBeInTheDocument();
  });
});

// ── Detail panel ──────────────────────────────────────────────────────────────

describe("GroupsPage — detail panel", () => {
  it("opens the detail panel when a row is clicked", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    expect(screen.getByRole("dialog", { name: /group details/i })).toBeInTheDocument();
  });

  it("shows group name in detail panel", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    expect(within(panel).getAllByText("Production").length).toBeGreaterThanOrEqual(1);
  });

  it("closes the detail panel when close button is clicked", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    await userEvent.click(within(panel).getByRole("button", { name: /close panel/i }));
    expect(screen.queryByRole("dialog", { name: /group details/i })).not.toBeInTheDocument();
  });

  it("shows member IP addresses in panel", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    expect(within(panel).getByText("192.168.1.1")).toBeInTheDocument();
    expect(within(panel).getByText("192.168.1.2")).toBeInTheDocument();
  });

  it("shows 'No members' message when group is empty", async () => {
    mockUseGroupMembers.mockReturnValue(
      makeUseGroupMembersResult({
        data: { data: [], pagination: { total_pages: 0, total: 0 } },
      }),
    );
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    expect(within(panel).getByText("No members in this group.")).toBeInTheDocument();
  });

  it("shows 'Delete group' in panel footer", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    expect(within(panel).getByText("Delete group")).toBeInTheDocument();
  });

  it("shows delete confirmation when delete is clicked", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    await userEvent.click(within(panel).getByText("Delete group"));
    expect(within(panel).getByText("Delete this group?")).toBeInTheDocument();
  });

  it("shows 'Edit group' button", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    expect(within(panel).getByRole("button", { name: /edit group/i })).toBeInTheDocument();
  });
});

// ── Create modal ──────────────────────────────────────────────────────────────

describe("GroupsPage — create modal", () => {
  it("opens create modal when 'Create group' is clicked", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByRole("button", { name: /create group/i }));
    expect(screen.getByRole("dialog", { name: /create group/i })).toBeInTheDocument();
  });

  it("renders name input in create modal", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByRole("button", { name: /create group/i }));
    expect(screen.getByPlaceholderText(/e.g. Production servers/i)).toBeInTheDocument();
  });

  it("renders color swatches in create modal", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByRole("button", { name: /create group/i }));
    const modal = screen.getByRole("dialog", { name: /create group/i });
    const swatches = within(modal).getAllByRole("button", { name: /^Color #/i });
    expect(swatches.length).toBeGreaterThanOrEqual(6);
  });

  it("closes create modal when Cancel is clicked", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByRole("button", { name: /create group/i }));
    const modal = screen.getByRole("dialog", { name: /create group/i });
    await userEvent.click(within(modal).getByRole("button", { name: /cancel/i }));
    expect(screen.queryByRole("dialog", { name: /create group/i })).not.toBeInTheDocument();
  });
});

// ── Edit modal ────────────────────────────────────────────────────────────────

describe("GroupsPage — edit modal", () => {
  it("opens edit modal from detail panel", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    await userEvent.click(within(panel).getByRole("button", { name: /edit group/i }));
    expect(screen.getByRole("dialog", { name: /edit group/i })).toBeInTheDocument();
  });

  it("pre-fills name in edit modal", async () => {
    render(<GroupsPage />);
    await userEvent.click(screen.getByText("Production").closest("tr")!);
    const panel = screen.getByRole("dialog", { name: /group details/i });
    await userEvent.click(within(panel).getByRole("button", { name: /edit group/i }));
    const modal = screen.getByRole("dialog", { name: /edit group/i });
    expect(within(modal).getByDisplayValue("Production")).toBeInTheDocument();
  });
});
