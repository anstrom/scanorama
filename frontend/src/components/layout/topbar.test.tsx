import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Topbar } from "./topbar";

// ── Mock hooks used by Topbar ─────────────────────────────────────────────────

vi.mock("../../api/hooks/use-system", () => ({
  useHealth: vi.fn(),
}));

vi.mock("../../lib/use-ws", () => ({
  useWsStatus: vi.fn(),
}));

vi.mock("../../hooks/use-theme", () => ({
  useTheme: vi.fn(),
}));

import { useHealth } from "../../api/hooks/use-system";
import { useWsStatus } from "../../lib/use-ws";
import { useTheme } from "../../hooks/use-theme";

const mockUseHealth = vi.mocked(useHealth);
const mockUseWsStatus = vi.mocked(useWsStatus);
const mockUseTheme = vi.mocked(useTheme);

function setupDefaultMocks() {
  mockUseHealth.mockReturnValue({
    data: { status: "healthy" },
    isLoading: false,
    error: null,
  } as ReturnType<typeof useHealth>);
  mockUseWsStatus.mockReturnValue("connected");
  mockUseTheme.mockReturnValue({
    theme: "dark",
    toggleTheme: vi.fn(),
  });
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("Topbar", () => {
  beforeEach(() => {
    setupDefaultMocks();
  });

  it("renders the title", () => {
    render(<Topbar title="Dashboard" />);
    expect(screen.getByText("Dashboard")).toBeInTheDocument();
  });

  it("shows 'Live' when WS is connected", () => {
    mockUseWsStatus.mockReturnValue("connected");
    render(<Topbar title="Test" />);
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("shows 'Healthy' when API is healthy", () => {
    render(<Topbar title="Test" />);
    expect(screen.getByText("Healthy")).toBeInTheDocument();
  });

  it("shows 'Unhealthy' when API is not healthy", () => {
    mockUseHealth.mockReturnValue({
      data: { status: "degraded" },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useHealth>);
    render(<Topbar title="Test" />);
    expect(screen.getByText("Unhealthy")).toBeInTheDocument();
  });

  // ── ThemeToggle integration ────────────────────────────────────────────────

  it("renders Sun icon and correct aria-label when theme is dark", () => {
    mockUseTheme.mockReturnValue({ theme: "dark", toggleTheme: vi.fn() });
    render(<Topbar title="Test" />);
    const btn = screen.getByRole("button", { name: "Switch to light mode" });
    expect(btn).toBeInTheDocument();
  });

  it("renders Moon icon and correct aria-label when theme is light", () => {
    mockUseTheme.mockReturnValue({ theme: "light", toggleTheme: vi.fn() });
    render(<Topbar title="Test" />);
    const btn = screen.getByRole("button", { name: "Switch to dark mode" });
    expect(btn).toBeInTheDocument();
  });

  it("calls toggleTheme when the theme toggle button is clicked", async () => {
    const user = userEvent.setup();
    const toggleTheme = vi.fn();
    mockUseTheme.mockReturnValue({ theme: "dark", toggleTheme });
    render(<Topbar title="Test" />);
    const btn = screen.getByRole("button", { name: "Switch to light mode" });
    await user.click(btn);
    expect(toggleTheme).toHaveBeenCalledTimes(1);
  });
});
