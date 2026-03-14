import { render, screen, within } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { RecentScansTable } from "./recent-scans-table";

const mockScans = [
  {
    id: "scan-1",
    status: "completed",
    targets: ["192.168.1.0/24"],
    hosts_discovered: 25,
    ports_scanned: 2500,
    created_at: new Date().toISOString(),
  },
  {
    id: "scan-2",
    status: "running",
    targets: ["10.0.0.0/8", "172.16.0.0/12"],
    hosts_discovered: 10,
    ports_scanned: 500,
    created_at: new Date().toISOString(),
  },
];

describe("RecentScansTable", () => {
  // 1. Loading skeletons
  it("shows loading skeletons when loading is true", () => {
    const { container } = render(<RecentScansTable loading={true} />);
    const skeletons = container.querySelectorAll(".animate-pulse");
    // 4 skeleton divs per row × 5 rows = 20
    expect(skeletons).toHaveLength(20);
  });

  it("does not show the table or empty message when loading", () => {
    render(<RecentScansTable loading={true} />);
    expect(screen.queryByText("No scans found.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  // 2. No scans found — undefined
  it("shows 'No scans found.' when scans is undefined", () => {
    render(<RecentScansTable />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  // 3. No scans found — empty array
  it("shows 'No scans found.' when scans is an empty array", () => {
    render(<RecentScansTable scans={[]} />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  // 4. Table headers
  it("renders all table column headers when scans are provided", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByRole("columnheader", { name: "Status" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Targets" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Hosts" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Ports" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "When" })).toBeInTheDocument();
  });

  // 5. Scan data in rows
  it("renders a row for each scan", () => {
    render(<RecentScansTable scans={mockScans} />);
    const rows = screen.getAllByRole("row");
    // 1 header row + 2 data rows
    expect(rows).toHaveLength(3);
  });

  it("renders numeric hosts_discovered and ports_scanned values", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("25")).toBeInTheDocument();
    expect(screen.getByText("2500")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
  });

  // 6. StatusBadge renders status text
  it("displays the status text for each scan via StatusBadge", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("running")).toBeInTheDocument();
  });

  // 7. Multiple targets joined with comma
  it("joins multiple targets with a comma separator", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("10.0.0.0/8, 172.16.0.0/12")).toBeInTheDocument();
  });

  it("renders a single target without a trailing comma", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  // 8. Em-dash for missing optional fields
  it("shows em-dash for missing targets", () => {
    const scans = [{ id: "scan-x", status: "completed" }];
    render(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // Targets cell is index 1
    expect(cells[1]).toHaveTextContent("—");
  });

  it("shows em-dash for missing hosts_discovered", () => {
    const scans = [{ id: "scan-x", status: "completed", targets: ["10.0.0.1"] }];
    render(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // Hosts cell is index 2
    expect(cells[2]).toHaveTextContent("—");
  });

  it("shows em-dash for missing ports_scanned", () => {
    const scans = [{ id: "scan-x", status: "completed", targets: ["10.0.0.1"] }];
    render(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // Ports cell is index 3
    expect(cells[3]).toHaveTextContent("—");
  });

  it("shows em-dash for missing created_at", () => {
    const scans = [{ id: "scan-x", status: "completed", targets: ["10.0.0.1"] }];
    render(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // When cell is index 4
    expect(cells[4]).toHaveTextContent("—");
  });

  // 9. "Recent Scans" heading always present
  it("shows the 'Recent Scans' heading when loading", () => {
    render(<RecentScansTable loading={true} />);
    expect(screen.getByRole("heading", { name: "Recent Scans" })).toBeInTheDocument();
  });

  it("shows the 'Recent Scans' heading when scans is undefined", () => {
    render(<RecentScansTable />);
    expect(screen.getByRole("heading", { name: "Recent Scans" })).toBeInTheDocument();
  });

  it("shows the 'Recent Scans' heading when scans are provided", () => {
    render(<RecentScansTable scans={mockScans} />);
    expect(screen.getByRole("heading", { name: "Recent Scans" })).toBeInTheDocument();
  });
});
