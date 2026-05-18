import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderWithRouter } from "../test/utils";
import { ScanDiffPage } from "./scan-diff";
import type { ScanDiff } from "../api/hooks/use-scan-diff";

vi.mock("../api/hooks/use-scan-diff", () => ({
  useScanDiff: vi.fn(),
}));

// TanStack Router's useSearch returns an empty object by default in test.
// The page reads optional `a` and `b` params, so missing params renders the
// missing-params notice without crashing.
vi.mock("@tanstack/react-router", async (importOriginal) => {
  const original = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...original,
    useSearch: vi.fn(() => ({})),
    Link: ({ children, ...rest }: { children: React.ReactNode; [key: string]: unknown }) => (
      <a {...rest}>{children}</a>
    ),
  };
});

import { useScanDiff } from "../api/hooks/use-scan-diff";
import { useSearch } from "@tanstack/react-router";

const mockUseScanDiff = vi.mocked(useScanDiff);
const mockUseSearch = vi.mocked(useSearch);

const idA = "aaaaaaaa-0000-0000-0000-000000000001";
const idB = "bbbbbbbb-0000-0000-0000-000000000002";

const mockDiff: ScanDiff = {
  scan_a_id: idA,
  scan_b_id: idB,
  host_id: "cccccccc-0000-0000-0000-000000000003",
  ports: [
    {
      port: 443,
      protocol: "tcp",
      state: "open",
      service_name: "https",
      status: "new",
    },
    {
      port: 80,
      protocol: "tcp",
      state: "open",
      service_name: "http",
      status: "unchanged",
    },
    {
      port: 22,
      protocol: "tcp",
      state: "open",
      service_name: "ssh",
      status: "changed",
      prev_state: "filtered",
      prev_service_name: "ssh-old",
    },
  ],
  os_changed: false,
  new_count: 1,
  closed_count: 0,
  changed_count: 1,
  unchanged_count: 1,
};

function setupDefaultMocks() {
  mockUseScanDiff.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } as any);
}

describe("ScanDiffPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupDefaultMocks();
    mockUseSearch.mockReturnValue({ a: idA, b: idB });
  });

  it("shows missing-params notice when no IDs in URL", async () => {
    mockUseSearch.mockReturnValue({});
    const { getByText } = await renderWithRouter(<ScanDiffPage />);
    expect(getByText(/Two scan IDs are required/i)).toBeTruthy();
  });

  it("shows loading skeleton when isLoading is true", async () => {
    mockUseScanDiff.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const { container } = await renderWithRouter(<ScanDiffPage />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows error message when isError is true", async () => {
    mockUseScanDiff.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("scan not found"),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const { getByText } = await renderWithRouter(<ScanDiffPage />);
    expect(getByText(/Failed to load diff/i)).toBeTruthy();
    expect(getByText(/scan not found/i)).toBeTruthy();
  });

  it("renders port table with correct status badges on success", async () => {
    mockUseScanDiff.mockReturnValue({
      data: mockDiff,
      isLoading: false,
      isError: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const { getAllByText, getByText } = await renderWithRouter(<ScanDiffPage />);

    // Summary counts (check badge labels are rendered)
    expect(getByText("new")).toBeTruthy();

    // Status badges
    expect(getAllByText("NEW").length).toBeGreaterThan(0);
    expect(getAllByText("CHANGED").length).toBeGreaterThan(0);

    // Port numbers appear
    expect(getByText("443/tcp")).toBeTruthy();
    expect(getByText("80/tcp")).toBeTruthy();
    expect(getByText("22/tcp")).toBeTruthy();
  });

  it("shows OS changed banner when os_changed is true", async () => {
    mockUseScanDiff.mockReturnValue({
      data: {
        ...mockDiff,
        os_changed: true,
        prev_os_name: "Ubuntu 20.04",
        curr_os_name: "Ubuntu 22.04",
      },
      isLoading: false,
      isError: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const { getByText } = await renderWithRouter(<ScanDiffPage />);
    expect(getByText(/OS changed/i)).toBeTruthy();
    expect(getByText("Ubuntu 22.04")).toBeTruthy();
  });

  it("shows no-changes notice when all counts are zero", async () => {
    mockUseScanDiff.mockReturnValue({
      data: {
        ...mockDiff,
        ports: [],
        new_count: 0,
        closed_count: 0,
        changed_count: 0,
        unchanged_count: 0,
      },
      isLoading: false,
      isError: false,
      error: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const { getByText } = await renderWithRouter(<ScanDiffPage />);
    expect(getByText(/No changes detected/i)).toBeTruthy();
  });
});
