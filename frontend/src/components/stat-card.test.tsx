import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StatCard } from "./stat-card";

function MockIcon(props: React.SVGProps<SVGSVGElement>) {
  return <svg data-testid="stat-icon" {...props} />;
}

describe("StatCard", () => {
  it("renders label and value", () => {
    render(<StatCard label="Hosts" value={42} />);
    expect(screen.getByText("Hosts")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("renders string value", () => {
    render(<StatCard label="Status" value="Online" />);
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Online")).toBeInTheDocument();
  });

  it("renders subtext when provided", () => {
    render(<StatCard label="Hosts" value={42} subtext="across all networks" />);
    expect(screen.getByText("across all networks")).toBeInTheDocument();
  });

  it("does not render subtext when omitted", () => {
    render(<StatCard label="Hosts" value={42} />);
    expect(screen.queryByText("across all networks")).not.toBeInTheDocument();
  });

  it("renders icon when provided", () => {
    render(<StatCard label="Hosts" value={42} icon={MockIcon} />);
    expect(screen.getByTestId("stat-icon")).toBeInTheDocument();
  });

  it("does not render icon when omitted", () => {
    render(<StatCard label="Hosts" value={42} />);
    expect(screen.queryByTestId("stat-icon")).not.toBeInTheDocument();
  });

  it("shows loading skeleton with animate-pulse when loading is true", () => {
    const { container } = render(<StatCard label="Hosts" value={42} loading />);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });

  it("hides label when loading", () => {
    render(<StatCard label="Hosts" value={42} loading />);
    expect(screen.queryByText("Hosts")).not.toBeInTheDocument();
  });

  it("hides value when loading", () => {
    render(<StatCard label="Hosts" value={42} loading />);
    expect(screen.queryByText("42")).not.toBeInTheDocument();
  });

  it("hides subtext when loading", () => {
    render(
      <StatCard label="Hosts" value={42} subtext="across all networks" loading />,
    );
    expect(screen.queryByText("across all networks")).not.toBeInTheDocument();
  });

  it("hides icon when loading", () => {
    render(<StatCard label="Hosts" value={42} icon={MockIcon} loading />);
    expect(screen.queryByTestId("stat-icon")).not.toBeInTheDocument();
  });

  it("renders positive trend with text-success and + prefix", () => {
    render(<StatCard label="Hosts" value={42} trend={{ value: 12 }} />);
    const trendEl = screen.getByText("+12%");
    expect(trendEl).toBeInTheDocument();
    expect(trendEl).toHaveClass("text-success");
  });

  it("renders negative trend with text-danger", () => {
    render(<StatCard label="Hosts" value={42} trend={{ value: -5 }} />);
    const trendEl = screen.getByText("-5%");
    expect(trendEl).toBeInTheDocument();
    expect(trendEl).toHaveClass("text-danger");
  });

  it("renders zero trend with text-text-muted", () => {
    render(<StatCard label="Hosts" value={42} trend={{ value: 0 }} />);
    const trendEl = screen.getByText("0%");
    expect(trendEl).toBeInTheDocument();
    expect(trendEl).toHaveClass("text-text-muted");
  });

  it("renders trend label when provided", () => {
    render(
      <StatCard label="Hosts" value={42} trend={{ value: 8, label: "vs last week" }} />,
    );
    expect(screen.getByText("+8% vs last week")).toBeInTheDocument();
  });

  it("renders negative trend label when provided", () => {
    render(
      <StatCard label="Hosts" value={42} trend={{ value: -3, label: "vs last month" }} />,
    );
    expect(screen.getByText("-3% vs last month")).toBeInTheDocument();
  });

  it("renders trend without label when label is omitted", () => {
    render(<StatCard label="Hosts" value={42} trend={{ value: 5 }} />);
    expect(screen.getByText("+5%")).toBeInTheDocument();
  });

  it("renders both trend and subtext together", () => {
    render(
      <StatCard
        label="Hosts"
        value={42}
        trend={{ value: 10 }}
        subtext="across all networks"
      />,
    );
    expect(screen.getByText("+10%")).toBeInTheDocument();
    expect(screen.getByText("across all networks")).toBeInTheDocument();
  });
});
