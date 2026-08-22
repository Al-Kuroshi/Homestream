import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { createTestQueryClient } from "./test/queryClient";
import { server } from "./test/server";
import { AppRoutes } from "./AppRoutes";

describe("AppRoutes", () => {
  it("redirects / to the Guide screen", () => {
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <MemoryRouter initialEntries={["/"]}>
          <AppRoutes />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(screen.getByRole("heading", { name: "Guide" })).toBeInTheDocument();
  });

  it("renders the Library screen at /library", async () => {
    server.use(
      http.get("/api/media", () => HttpResponse.json([])),
      http.get("/api/sources", () => HttpResponse.json([]))
    );
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <MemoryRouter initialEntries={["/library"]}>
          <AppRoutes />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(await screen.findByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("renders the Channels screen at /channels", () => {
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <MemoryRouter initialEntries={["/channels"]}>
          <AppRoutes />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("renders the Settings screen at /settings", async () => {
    server.use(
      http.get("/api/sources", () => HttpResponse.json([])),
      http.get("/api/media", () => HttpResponse.json([]))
    );
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <MemoryRouter initialEntries={["/settings"]}>
          <AppRoutes />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(await screen.findByRole("heading", { name: "Settings" })).toBeInTheDocument();
  });
});
