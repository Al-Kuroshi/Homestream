import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelsListScreen } from "./ChannelsListScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Sitcoms", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderScreen(channels: typeof CHANNELS = CHANNELS) {
  server.use(http.get("/api/channels", () => HttpResponse.json(channels)));
  const client = createTestQueryClient();
  render(
    <MemoryRouter>
      <ChannelsListScreen />
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("ChannelsListScreen", () => {
  it("lists channels ordered by position, each linking to its schedule editor", async () => {
    renderScreen();
    expect(await screen.findByRole("link", { name: "Movies" })).toHaveAttribute("href", "/channels/1");
    expect(screen.getByRole("link", { name: "Sitcoms" })).toHaveAttribute("href", "/channels/2");
  });

  it("shows an empty state with no channels", async () => {
    renderScreen([]);
    expect(await screen.findByText("No channels yet.")).toBeInTheDocument();
  });

  it("creates a channel via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/channels", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(
          { id: 3, description: "", enabled: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", ...(created as object) },
          { status: 201 }
        );
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.type(screen.getByLabelText("Name"), "News");
    await userEvent.click(screen.getByRole("button", { name: "Create channel" }));

    await waitFor(() => expect(created).toEqual({ name: "News", position: 2 }));
  });

  it("toggles a channel's enabled state", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/channels/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...CHANNELS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("checkbox", { name: "Enabled" })[0]);

    await waitFor(() => expect(putBody).toMatchObject({ enabled: false }));
  });

  it("renames a channel", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/channels/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...CHANNELS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("button", { name: "Rename" })[0]);
    const input = screen.getByLabelText("Rename Movies");
    await userEvent.clear(input);
    await userEvent.type(input, "Movies HD");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(putBody).toMatchObject({ name: "Movies HD" }));
  });

  it("swaps positions when moving a channel down", async () => {
    const putBodies: unknown[] = [];
    server.use(
      http.put("/api/channels/:id", async ({ request }) => {
        putBodies.push(await request.json());
        return HttpResponse.json({});
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getByRole("button", { name: "Move Movies down" }));

    await waitFor(() => expect(putBodies).toHaveLength(2));
    expect(putBodies).toEqual(
      expect.arrayContaining([expect.objectContaining({ position: 1 }), expect.objectContaining({ position: 0 })])
    );
  });

  it("deletes a channel", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/channels/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("button", { name: "Delete" })[0]);

    await waitFor(() => expect(deleted).toBe(true));
  });

  it("shows an error message when deleting a channel fails", async () => {
    server.use(
      http.delete("/api/channels/1", () => HttpResponse.json({ error: "cannot delete: channel has active viewers" }, { status: 500 }))
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("button", { name: "Delete" })[0]);

    expect(await screen.findByRole("alert")).toHaveTextContent("cannot delete: channel has active viewers");
  });
});
