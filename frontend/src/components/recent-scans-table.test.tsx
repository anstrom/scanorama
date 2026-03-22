import { screen, within } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { renderWithRouter } from "../test/utils";
import { RecentScansTable } from "./recent-scans-table";

const mockScans = [
  {
    id: "scan-1",
    status: "completed",
    targets: ["192.168.1.0/24"],
    ports_scanned: "22,80,443",
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
  },
  {
    id: "scan-2",
    status: "running",
    targets: ["10.0.0.0/8", "172.16.0.0/12"],
    ports_scanned: "1-1024",
    started_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
  },
];

describe("RecentScansTable", () => {
  // 1. Loading skeletons
  it("shows loading skeletons when loading is true", async () => {
    const { container } = await renderWithRouter(
      <RecentScansTable loading={true} />,
    );
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("does not show the table or empty message when loading", async () => {
    await renderWithRouter(<RecentScansTable loading={true} />);
    expect(screen.queryByText("No scans found.")).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  // 2. No scans found — undefined
  it("shows 'No scans found.' when scans is undefined", async () => {
    await renderWithRouter(<RecentScansTable />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  // 3. No scans found — empty array
  it("shows 'No scans found.' when scans is an empty array", async () => {
    await renderWithRouter(<RecentScansTable scans={[]} />);
    expect(screen.getByText("No scans found.")).toBeInTheDocument();
  });

  // 4. Table headers — Status, Targets, Ports, When (no Hosts column)
  it("renders all table column headers when scans are provided", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(
      screen.getByRole("columnheader", { name: "Status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Targets" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Ports" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "When" }),
    ).toBeInTheDocument();
  });

  it("does not render a Hosts column header", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(
      screen.queryByRole("columnheader", { name: "Hosts" }),
    ).not.toBeInTheDocument();
  });

  // 5. Scan data in rows
  it("renders a row for each scan", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    const rows = screen.getAllByRole("row");
    // 1 header row + 2 data rows
    expect(rows).toHaveLength(3);
  });

  it("renders ports_scanned string values in the Ports column", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("22,80,443")).toBeInTheDocument();
    expect(screen.getByText("1-1024")).toBeInTheDocument();
  });

  // 6. StatusBadge renders status text
  it("displays the status text for each scan via StatusBadge", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("running")).toBeInTheDocument();
  });

  // 7. Multiple targets joined with comma
  it("joins multiple targets with a comma separator", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("10.0.0.0/8, 172.16.0.0/12")).toBeInTheDocument();
  });

  it("renders a single target without a trailing comma", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();
  });

  // 8. Em-dash for missing optional fields
  it("shows em-dash for missing targets", async () => {
    const scans = [{ id: "scan-x", status: "completed" }];
    await renderWithRouter(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // Targets cell is index 1
    expect(cells[1]).toHaveTextContent("—");
  });

  it("shows em-dash for missing ports_scanned", async () => {
    const scans = [
      { id: "scan-x", status: "completed", targets: ["10.0.0.1"] },
    ];
    await renderWithRouter(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // Ports cell is index 2
    expect(cells[2]).toHaveTextContent("—");
  });

  it("shows em-dash for missing started_at and created_at", async () => {
    const scans = [
      { id: "scan-x", status: "completed", targets: ["10.0.0.1"] },
    ];
    await renderWithRouter(<RecentScansTable scans={scans} />);
    const rows = screen.getAllByRole("row");
    const dataRow = rows[1];
    const cells = within(dataRow).getAllByRole("cell");
    // When cell is index 3
    expect(cells[3]).toHaveTextContent("—");
  });

  // 9. Timestamp: started_at preferred over created_at (Bug 21)
  it("shows 'just now' for a scan with a recent started_at", async () => {
    const scans = [
      {
        id: "scan-x",
        status: "completed",
        targets: ["10.0.0.1"],
        started_at: new Date().toISOString(),
        created_at: new Date(Date.now() - 3_600_000).toISOString(), // 1h ago
      },
    ];
    await renderWithRouter(<RecentScansTable scans={scans} />);
    // started_at is "just now"; if created_at were used it would show "1h ago"
    expect(screen.getByText("just now")).toBeInTheDocument();
  });

  it("falls back to created_at when started_at is absent", async () => {
    const scans = [
      {
        id: "scan-x",
        status: "pending",
        targets: ["10.0.0.1"],
        created_at: new Date().toISOString(),
      },
    ];
    await renderWithRouter(<RecentScansTable scans={scans} />);
    expect(screen.getByText("just now")).toBeInTheDocument();
  });

  // 10. "Recent Scans" heading always present
  it("shows the 'Recent Scans' heading when loading", async () => {
    await renderWithRouter(<RecentScansTable loading={true} />);
    expect(
      screen.getByRole("heading", { name: "Recent Scans" }),
    ).toBeInTheDocument();
  });

  it("shows the 'Recent Scans' heading when scans is undefined", async () => {
    await renderWithRouter(<RecentScansTable />);
    expect(
      screen.getByRole("heading", { name: "Recent Scans" }),
    ).toBeInTheDocument();
  });

  it("shows the 'Recent Scans' heading when scans are provided", async () => {
    await renderWithRouter(<RecentScansTable scans={mockScans} />);
    expect(
      screen.getByRole("heading", { name: "Recent Scans" }),
    ).toBeInTheDocument();
  });
});
