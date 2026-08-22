import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { Sidebar } from "./Sidebar";

describe("Sidebar", () => {
  it("renders a link for each of the four screens", () => {
    render(
      <MemoryRouter initialEntries={["/guide"]}>
        <Sidebar />
      </MemoryRouter>
    );
    for (const label of ["Guide", "Library", "Channels", "Settings"]) {
      expect(screen.getByRole("link", { name: label })).toBeInTheDocument();
    }
  });

  it("marks the link matching the current route as active", () => {
    render(
      <MemoryRouter initialEntries={["/library"]}>
        <Sidebar />
      </MemoryRouter>
    );
    expect(screen.getByRole("link", { name: "Library" })).toHaveClass("active");
    expect(screen.getByRole("link", { name: "Guide" })).not.toHaveClass("active");
  });
});
