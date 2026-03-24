import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { SystemInfoCard } from "./system-info-card";
import type { VersionInfo } from "../api/hooks/use-system";

type VersionResponse = VersionInfo;

const releaseVersion: VersionResponse = {
  version: "v1.2.3",
  service: "scanorama",
  commit: "abc1234",
  build_time: "2024-06-01T12:00:00Z",
};

const devVersion: VersionResponse = {
  version: "dev",
  service: "scanorama",
  commit: "none",
  build_time: "unknown",
};

describe("SystemInfoCard", () => {
  // ── loading skeleton ────────────────────────────────────────────────────────

  it("shows a loading skeleton when loading is true", () => {
    const { container } = render(<SystemInfoCard loading />);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });

  it("does not show version text while loading", () => {
    render(<SystemInfoCard version={releaseVersion} loading />);
    expect(screen.queryByText("v1.2.3")).not.toBeInTheDocument();
  });

  it("does not show the dev build badge while loading", () => {
    render(<SystemInfoCard version={devVersion} loading />);
    expect(screen.queryByText("dev build")).not.toBeInTheDocument();
  });

  // ── build info labels ───────────────────────────────────────────────────────

  it("shows the Build info label", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("Build info")).toBeInTheDocument();
  });

  it("shows the Version label", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("Version")).toBeInTheDocument();
  });

  it("shows the Commit label", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("Commit")).toBeInTheDocument();
  });

  it("shows the Built label", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("Built")).toBeInTheDocument();
  });

  // ── release version ─────────────────────────────────────────────────────────

  it("shows the version number for a release build", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("v1.2.3")).toBeInTheDocument();
  });

  it("shows the commit hash for a release build", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.getByText("abc1234")).toBeInTheDocument();
  });

  it("does not show the dev build badge for a release version", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    expect(screen.queryByText("dev build")).not.toBeInTheDocument();
  });

  it("shows a formatted build time for a release build", () => {
    render(<SystemInfoCard version={releaseVersion} />);
    // The build time should not be an em-dash for a valid timestamp
    const builtValues = screen
      .getAllByText(/./i)
      .filter((el) => el.closest(".grid"));
    expect(builtValues.length).toBeGreaterThan(0);
    // em-dash should not appear in the built field for a known date
    const emDashes = screen.queryAllByText("—");
    // Only one em-dash max (none for this release version)
    expect(emDashes.length).toBe(0);
  });

  // ── dev version ─────────────────────────────────────────────────────────────

  it("shows the dev build badge when version is dev", () => {
    render(<SystemInfoCard version={devVersion} />);
    expect(screen.getByText("dev build")).toBeInTheDocument();
  });

  it("shows em-dash for commit when commit is 'none'", () => {
    render(<SystemInfoCard version={devVersion} />);
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(1);
  });

  it("shows em-dash for build time when build_time is 'unknown'", () => {
    render(<SystemInfoCard version={devVersion} />);
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(2);
  });

  it("shows the version string even for dev builds", () => {
    render(<SystemInfoCard version={devVersion} />);
    expect(screen.getByText("dev")).toBeInTheDocument();
  });

  // ── no version data ──────────────────────────────────────────────────────────

  it("shows the dev build badge when no version data is provided", () => {
    render(<SystemInfoCard />);
    expect(screen.getByText("dev build")).toBeInTheDocument();
  });

  it("shows em-dash for version when version data is undefined", () => {
    render(<SystemInfoCard />);
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(1);
  });

  // ── edge cases ───────────────────────────────────────────────────────────────

  it("does not show loading skeleton when loading is false (default)", () => {
    const { container } = render(<SystemInfoCard version={releaseVersion} />);
    expect(container.querySelector(".animate-pulse")).not.toBeInTheDocument();
  });

  it("shows em-dash for build time when build_time is undefined", () => {
    render(
      <SystemInfoCard
        version={{ version: "v1.0.0", service: "scanorama", commit: "abc" }}
      />,
    );
    const emDashes = screen.getAllByText("—");
    expect(emDashes.length).toBeGreaterThanOrEqual(1);
  });
});
