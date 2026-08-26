import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient } from "./test/queryClient";
import { server } from "./test/server";
import { App } from "./App";

describe("App", () => {
  it("renders the sidebar navigation and the routed content area", async () => {
    server.use(
      http.get("/api/channels", () => HttpResponse.json([])),
      http.get("/api/media", () => HttpResponse.json([]))
    );
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <App />
      </QueryClientProvider>
    );
    expect(screen.getByRole("navigation", { name: "Main navigation" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "TV" })).toBeInTheDocument();
  });
});
