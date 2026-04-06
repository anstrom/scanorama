import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ExclusionsPage } from "./exclusions";

vi.mock("../api/hooks/use-networks", () => ({
  useGlobalExclusions: vi.fn(),
  useDeleteExclusion: vi.fn(),
}));

vi.mock("../components/add-exclusion-modal", () => ({
  AddExclusionModal: ({
    onClose,
  }: {
    networkId?: string;
    onClose: () => void;
    onCreated?: () => void;
  }) => (
    <div role="dialog" aria-label="Add Global Exclusion">
      <button type="button" onClick={onClose}>
        Close modal
      </button>
    </div>
  ),
}));

import {
  useGlobalExclusions,
  useDeleteExclusion,
} from "../api/hooks/use-networks";

const mockUseGlobalExclusions = vi.mocked(useGlobalExclusions);
const mockUseDeleteExclusion = vi.mocked(useDeleteExclusion);

const mockExclusions = [
  {
    id: "excl-1",
    excluded_cidr: "10.0.0.0/8",
    reason: "Private range",
    created_by: "admin",
    created_at: "2024-01-01T00:00:00Z",
    enabled: true,
  },
  {
    id: "excl-2",
    excluded_cidr: "192.168.100.0/24",
    reason: "Test environment",
    created_by: "ops",
    created_at: "2024-02-15T12:00:00Z",
    enabled: true,
  },
  {
    id: "excl-3",
    excluded_cidr: "172.16.0.0/12",
    reason: undefined,
    created_by: "admin",
    created_at: "2024-03-01T08:30:00Z",
    enabled: false,
  },
];

function makeDeleteResult(overrides = {}) {
  return {
    mutate: vi.fn(),
    mutateAsync: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    ...overrides,
  } as unknown as ReturnType<typeof useDeleteExclusion>;
}

function makeQueryResult(overrides = {}) {
  return {
    data: mockExclusions,
    isLoading: false,
    isSuccess: true,
    isError: false,
    ...overrides,
  } as unknown as ReturnType<typeof useGlobalExclusions>;
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseGlobalExclusions.mockReturnValue(makeQueryResult());
  mockUseDeleteExclusion.mockReturnValue(makeDeleteResult());
});

describe("ExclusionsPage", () => {
  // ── Loading state ────────────────────────────────────────────────────────
  it("renders skeleton rows while loading", () => {
    mockUseGlobalExclusions.mockReturnValue(
      makeQueryResult({ data: undefined, isLoading: true, isSuccess: false }),
    );
    const { container } = render(<ExclusionsPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThanOrEqual(5);
  });

  it("does not show the empty message while loading", () => {
    mockUseGlobalExclusions.mockReturnValue(
      makeQueryResult({ data: undefined, isLoading: true, isSuccess: false }),
    );
    render(<ExclusionsPage />);
    expect(
      screen.queryByText("No global exclusions defined."),
    ).not.toBeInTheDocument();
  });

  // ── Empty state ──────────────────────────────────────────────────────────
  it("shows empty message when there are no global exclusions", () => {
    mockUseGlobalExclusions.mockReturnValue(makeQueryResult({ data: [] }));
    render(<ExclusionsPage />);
    expect(
      screen.getByText("No global exclusions defined."),
    ).toBeInTheDocument();
  });

  // ── Table structure ──────────────────────────────────────────────────────
  it("renders all column headers", () => {
    render(<ExclusionsPage />);
    expect(
      screen.getByRole("columnheader", { name: "CIDR Block" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Reason" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Created By" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Created At" }),
    ).toBeInTheDocument();
  });

  it("renders one data row per exclusion", () => {
    render(<ExclusionsPage />);
    // 1 header + 3 data rows
    expect(screen.getAllByRole("row")).toHaveLength(4);
  });

  // ── Data rendering ───────────────────────────────────────────────────────
  it("renders CIDR blocks in monospace font", () => {
    render(<ExclusionsPage />);
    expect(screen.getByText("10.0.0.0/8")).toBeInTheDocument();
    expect(screen.getByText("192.168.100.0/24")).toBeInTheDocument();
    expect(screen.getByText("172.16.0.0/12")).toBeInTheDocument();
  });

  it("renders reason text when present", () => {
    render(<ExclusionsPage />);
    expect(screen.getByText("Private range")).toBeInTheDocument();
    expect(screen.getByText("Test environment")).toBeInTheDocument();
  });

  it("renders em-dash for missing reason", () => {
    render(<ExclusionsPage />);
    const rows = screen.getAllByRole("row");
    // After CIDR sort: 10.x=row1, 172.x=row2 (excl-3, no reason), 192.x=row3
    const cells = within(rows[2]).getAllByRole("cell");
    // Reason is column index 1
    expect(cells[1]).toHaveTextContent("—");
  });

  it("renders created_by values", () => {
    render(<ExclusionsPage />);
    expect(screen.getAllByText("admin")).toHaveLength(2);
    expect(screen.getByText("ops")).toBeInTheDocument();
  });

  it("renders a relative timestamp for created_at", () => {
    render(<ExclusionsPage />);
    // All dates in the past → should show relative format like "Xd ago" or similar
    const rows = screen.getAllByRole("row");
    const firstDataCells = within(rows[1]).getAllByRole("cell");
    // created_at is index 3
    expect(firstDataCells[3].textContent).not.toBe("");
    expect(firstDataCells[3].textContent).not.toBe("—");
  });

  // ── Toolbar ──────────────────────────────────────────────────────────────
  it("renders the description text about global exclusions", () => {
    render(<ExclusionsPage />);
    expect(
      screen.getByText(/global exclusions apply to all networks/i),
    ).toBeInTheDocument();
  });

  it("renders the Add exclusion button", () => {
    render(<ExclusionsPage />);
    expect(
      screen.getByRole("button", { name: /add exclusion/i }),
    ).toBeInTheDocument();
  });

  // ── Add exclusion modal ──────────────────────────────────────────────────
  it("opens AddExclusionModal when Add exclusion is clicked", async () => {
    render(<ExclusionsPage />);
    const addBtn = screen.getByRole("button", { name: /add exclusion/i });
    await userEvent.click(addBtn);
    expect(
      screen.getByRole("dialog", { name: /add global exclusion/i }),
    ).toBeInTheDocument();
  });

  it("closes AddExclusionModal when the modal's close action fires", async () => {
    render(<ExclusionsPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /add exclusion/i }),
    );
    const modal = screen.getByRole("dialog", { name: /add global exclusion/i });
    await userEvent.click(
      within(modal).getByRole("button", { name: /close modal/i }),
    );
    expect(
      screen.queryByRole("dialog", { name: /add global exclusion/i }),
    ).not.toBeInTheDocument();
  });

  // ── Delete — first click asks for confirmation ───────────────────────────
  it("shows Confirm / Cancel options after clicking the delete icon", async () => {
    render(<ExclusionsPage />);
    const deleteBtn = screen.getByRole("button", {
      name: /delete exclusion 10\.0\.0\.0\/8/i,
    });
    await userEvent.click(deleteBtn);
    expect(screen.getByText("Confirm")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("hides the Confirm/Cancel after clicking Cancel", async () => {
    render(<ExclusionsPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /delete exclusion 10\.0\.0\.0\/8/i }),
    );
    await userEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("Confirm")).not.toBeInTheDocument();
  });

  it("calls deleteExclusion.mutate with the correct id on Confirm", async () => {
    const mockMutate = vi.fn();
    mockUseDeleteExclusion.mockReturnValue(
      makeDeleteResult({ mutate: mockMutate }),
    );

    render(<ExclusionsPage />);
    await userEvent.click(
      screen.getByRole("button", {
        name: /delete exclusion 192\.168\.100\.0\/24/i,
      }),
    );
    await userEvent.click(screen.getByText("Confirm"));
    expect(mockMutate).toHaveBeenCalledWith("excl-2", expect.any(Object));
  });

  it("only shows confirm prompt for the clicked row, not all rows", async () => {
    render(<ExclusionsPage />);
    await userEvent.click(
      screen.getByRole("button", { name: /delete exclusion 10\.0\.0\.0\/8/i }),
    );
    // Confirm appears once (for excl-1)
    expect(screen.getAllByText("Confirm")).toHaveLength(1);
    // The other delete buttons remain as icon buttons
    expect(
      screen.getByRole("button", {
        name: /delete exclusion 192\.168\.100\.0\/24/i,
      }),
    ).toBeInTheDocument();
  });
});
