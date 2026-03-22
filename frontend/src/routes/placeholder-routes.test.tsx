import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { SchedulesPage } from "./schedules";
import { ProfilesPage } from "./profiles";
import { AdminPage } from "./admin";

describe("SchedulesPage", () => {
  it("renders the coming-soon message", () => {
    render(<SchedulesPage />);
    expect(
      screen.getByText("Schedule management coming soon."),
    ).toBeInTheDocument();
  });
});

describe("ProfilesPage", () => {
  it("renders the coming-soon message", () => {
    render(<ProfilesPage />);
    expect(screen.getByText("Scan profiles coming soon.")).toBeInTheDocument();
  });
});

describe("AdminPage", () => {
  it("renders the coming-soon message", () => {
    render(<AdminPage />);
    expect(screen.getByText("Admin panel coming soon.")).toBeInTheDocument();
  });
});
