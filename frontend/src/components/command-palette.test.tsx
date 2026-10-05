import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
  afterEach,
} from "vitest";
import { screen, fireEvent, waitFor, act } from "@testing-library/react";
import { renderWithRouter } from "../test/utils";
import { CommandPalette } from "./command-palette";


// ── Mock hooks ─────────────────────────────────────────────────────────────────

const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock("../api/hooks/use-search", () => ({
  useSearch: vi.fn(),
}));

vi.mock("../hooks/use-recent-pages", () => ({
  useRecentPages: vi.fn(),
}));

import { useSearch, type SearchResults } from "../api/hooks/use-search";
import { useRecentPages } from "../hooks/use-recent-pages";

const mockUseSearch = vi.mocked(useSearch);
const mockUseRecentPages = vi.mocked(useRecentPages);

const mockAddRecentPage = vi.fn();

// The palette only reads data/isLoading/isError from the query result, so the
// mock supplies that subset; the payload itself is checked against SearchResults.
function searchReturning(data: SearchResults | undefined): ReturnType<typeof useSearch> {
  return { data, isLoading: false, isError: false } as unknown as ReturnType<typeof useSearch>;
}

function setupDefaultMocks() {
  mockUseSearch.mockReturnValue(searchReturning(undefined));

  mockUseRecentPages.mockReturnValue({
    recentPages: [],
    addRecentPage: mockAddRecentPage,
  });
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("CommandPalette", () => {
  const onClose = vi.fn();

  beforeEach(() => {
    setupDefaultMocks();
    mockNavigate.mockClear();
    onClose.mockClear();
    mockAddRecentPage.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("renders the search input", async () => {
    await renderWithRouter(<CommandPalette onClose={onClose} />);
    expect(screen.getByRole("searchbox")).toBeInTheDocument();
  });

  it("shows the backdrop overlay", async () => {
    await renderWithRouter(<CommandPalette onClose={onClose} />);
    expect(screen.getByTestId("palette-backdrop")).toBeInTheDocument();
  });

  it("calls onClose when Escape is pressed", async () => {
    await renderWithRouter(<CommandPalette onClose={onClose} />);
    const input = screen.getByRole("searchbox");
    fireEvent.keyDown(input, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the backdrop is clicked", async () => {
    await renderWithRouter(<CommandPalette onClose={onClose} />);
    fireEvent.click(screen.getByTestId("palette-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("shows recent pages when query is empty and there are recent pages", async () => {
    mockUseRecentPages.mockReturnValue({
      recentPages: [
        { label: "Hosts", url: "/hosts", type: "recent" },
        { label: "Scans", url: "/scans", type: "recent" },
      ],
      addRecentPage: mockAddRecentPage,
    });

    await renderWithRouter(<CommandPalette onClose={onClose} />);

    expect(screen.getByText("Recent")).toBeInTheDocument();
    expect(screen.getByText("Hosts")).toBeInTheDocument();
    expect(screen.getByText("Scans")).toBeInTheDocument();
  });

  it("shows search results grouped by type when query has 2+ chars", async () => {
    // Return results for any query (debounce is bypassed since hook is mocked).
    mockUseSearch.mockReturnValue(searchReturning({
      results: {
        hosts: [
          { id: "h1", label: "192.168.1.1 (myhost)", url: "/hosts/h1", type: "host" },
        ],
        networks: [],
        scans: [],
        profiles: [],
      },
      total: 1,
    }));

    await renderWithRouter(<CommandPalette onClose={onClose} />);

    const input = screen.getByRole("searchbox");
    // Type 2+ chars so inputValue >= 2 chars — buildSections shows search mode.
    fireEvent.change(input, { target: { value: "my" } });

    await waitFor(() => {
      expect(screen.getByText("Hosts")).toBeInTheDocument();
      expect(screen.getByText("192.168.1.1 (myhost)")).toBeInTheDocument();
    });
  });

  it("shows 'no results' message when search returns empty", async () => {
    mockUseSearch.mockReturnValue(
      searchReturning({ results: { hosts: [], networks: [], scans: [], profiles: [] }, total: 0 }),
    );

    await renderWithRouter(<CommandPalette onClose={onClose} />);

    const input = screen.getByRole("searchbox");
    fireEvent.change(input, { target: { value: "xyznotfound" } });

    await waitFor(() => {
      expect(screen.getByText(/No results for/)).toBeInTheDocument();
    });
  });

  it("navigates and calls addRecentPage when a result is clicked", async () => {
    mockUseSearch.mockReturnValue(searchReturning({
      results: {
        hosts: [{ id: "h1", label: "192.168.1.1", url: "/hosts/h1", type: "host" }],
        networks: [],
        scans: [],
        profiles: [],
      },
      total: 1,
    }));

    await renderWithRouter(<CommandPalette onClose={onClose} />);

    const input = screen.getByRole("searchbox");
    fireEvent.change(input, { target: { value: "ip" } });

    await waitFor(() => {
      expect(screen.getByText("192.168.1.1")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("192.168.1.1"));

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/hosts/h1" });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mockAddRecentPage).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/hosts/h1", type: "recent" }),
    );
  });

  it("navigates with arrow keys and Enter", async () => {
    mockUseSearch.mockReturnValue(searchReturning({
      results: {
        hosts: [
          { id: "h1", label: "192.168.1.1", url: "/hosts/h1", type: "host" },
          { id: "h2", label: "192.168.1.2", url: "/hosts/h2", type: "host" },
        ],
        networks: [],
        scans: [],
        profiles: [],
      },
      total: 2,
    }));

    await renderWithRouter(<CommandPalette onClose={onClose} />);

    const input = screen.getByRole("searchbox");
    fireEvent.change(input, { target: { value: "ip" } });

    await waitFor(() => {
      expect(screen.getByText("192.168.1.1")).toBeInTheDocument();
    });

    // Arrow down to first item, then Enter.
    act(() => {
      fireEvent.keyDown(input, { key: "ArrowDown" });
    });
    act(() => {
      fireEvent.keyDown(input, { key: "Enter" });
    });

    expect(mockNavigate).toHaveBeenCalledWith({ to: "/hosts/h1" });
  });
});
