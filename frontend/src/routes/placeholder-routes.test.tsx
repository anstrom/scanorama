import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";

import { AdminPage } from "./admin";

describe("AdminPage", () => {
  it("renders the coming-soon message", () => {
    render(<AdminPage />);
    expect(screen.getByText("Admin panel coming soon.")).toBeInTheDocument();
  });
});
