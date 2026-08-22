import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppRoutes } from "./AppRoutes";

describe("AppRoutes", () => {
  it("redirects / to the Guide screen", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Guide" })).toBeInTheDocument();
  });

  it("renders the Library screen at /library", () => {
    render(
      <MemoryRouter initialEntries={["/library"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("renders the Channels screen at /channels", () => {
    render(
      <MemoryRouter initialEntries={["/channels"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("renders the Settings screen at /settings", () => {
    render(
      <MemoryRouter initialEntries={["/settings"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Settings" })).toBeInTheDocument();
  });
});
