import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ActivityFeed } from "./activity-feed";
import type { ActivityEvent } from "../hooks/use-activity-feed";
import type { WsStatus } from "../lib/ws";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("../hooks/use-activity-feed", () => ({
  useActivityFeed: vi.fn(() => []),
}));

vi.mock("../lib/use-ws", () => ({
  useWsStatus: vi.fn(() => "disconnected" as WsStatus),
}));

import { useActivityFeed } from "../hooks/use-activity-feed";
import { useWsStatus } from "../lib/use-ws";

const mockUseActivityFeed = vi.mocked(useActivityFeed);
const mockUseWsStatus = vi.mocked(useWsStatus);

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeEvent(overrides: Partial<ActivityEvent>): ActivityEvent {
  return {
    id: "ev-1",
    kind: "scan_started",
    title: "Scan started",
    detail: "Scan #1",
    timestamp: new Date().toISOString(),
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  vi.clearAllMocks();
  mockUseActivityFeed.mockReturnValue([]);
  mockUseWsStatus.mockReturnValue("disconnected");
});

describe("ActivityFeed", () => {
  // ── Empty state ─────────────────────────────────────────────────────────────

  it("shows empty state when no events", () => {
    render(<ActivityFeed />);
    expect(screen.getByText("No recent activity")).toBeInTheDocument();
  });

  it("shows Activity header label", () => {
    render(<ActivityFeed />);
    expect(screen.getByText("Activity")).toBeInTheDocument();
  });

  // ── WS status badge ─────────────────────────────────────────────────────────

  it("shows Live badge when connected", () => {
    mockUseWsStatus.mockReturnValue("connected");
    render(<ActivityFeed />);
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("shows Connecting badge when connecting", () => {
    mockUseWsStatus.mockReturnValue("connecting");
    render(<ActivityFeed />);
    expect(screen.getByText("Connecting…")).toBeInTheDocument();
  });

  it("shows Disconnected badge when disconnected", () => {
    mockUseWsStatus.mockReturnValue("disconnected");
    render(<ActivityFeed />);
    expect(screen.getByText("Disconnected")).toBeInTheDocument();
  });

  // ── Event rendering ─────────────────────────────────────────────────────────

  it("renders event title and detail", () => {
    mockUseActivityFeed.mockReturnValue([
      makeEvent({ title: "Scan started", detail: "Scan #42" }),
    ]);
    render(<ActivityFeed />);
    expect(screen.getByText("Scan started")).toBeInTheDocument();
    expect(screen.getByText("Scan #42")).toBeInTheDocument();
  });

  it("renders multiple events", () => {
    mockUseActivityFeed.mockReturnValue([
      makeEvent({ id: "1", title: "Scan started", detail: "Scan #1" }),
      makeEvent({ id: "2", title: "Discovery completed", detail: "No changes", kind: "discovery_completed" }),
    ]);
    render(<ActivityFeed />);
    expect(screen.getByText("Scan started")).toBeInTheDocument();
    expect(screen.getByText("Discovery completed")).toBeInTheDocument();
  });

  it("renders event with href as a link", () => {
    mockUseActivityFeed.mockReturnValue([
      makeEvent({ href: "/scans" }),
    ]);
    render(<ActivityFeed />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "#/scans");
  });

  it("renders event without href as a div (no link)", () => {
    mockUseActivityFeed.mockReturnValue([
      makeEvent({ href: undefined }),
    ]);
    render(<ActivityFeed />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("limits display to 20 events even if more are provided", () => {
    const events: ActivityEvent[] = Array.from({ length: 25 }, (_, i) =>
      makeEvent({ id: String(i), title: `Event ${i}` }),
    );
    mockUseActivityFeed.mockReturnValue(events);
    render(<ActivityFeed />);

    // Events 0-19 visible, 20-24 not
    expect(screen.getByText("Event 0")).toBeInTheDocument();
    expect(screen.getByText("Event 19")).toBeInTheDocument();
    expect(screen.queryByText("Event 20")).not.toBeInTheDocument();
  });

  // ── Event kinds ─────────────────────────────────────────────────────────────

  it.each([
    "scan_started",
    "scan_completed",
    "scan_failed",
    "discovery_started",
    "discovery_completed",
    "host_status_change",
  ] as const)("renders %s event without error", (kind) => {
    mockUseActivityFeed.mockReturnValue([makeEvent({ kind, title: kind })]);
    render(<ActivityFeed />);
    expect(screen.getByText(kind)).toBeInTheDocument();
  });

  it("does not show 'No recent activity' when events exist", () => {
    mockUseActivityFeed.mockReturnValue([makeEvent({})]);
    render(<ActivityFeed />);
    expect(screen.queryByText("No recent activity")).not.toBeInTheDocument();
  });

  it("omits detail paragraph when detail is empty string", () => {
    mockUseActivityFeed.mockReturnValue([
      makeEvent({ title: "Scan started", detail: "" }),
    ]);
    render(<ActivityFeed />);
    // The title is rendered but detail paragraph should not appear
    expect(screen.getByText("Scan started")).toBeInTheDocument();
  });
});
