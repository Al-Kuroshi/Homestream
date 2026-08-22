import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { SettingsScreen } from "./SettingsScreen";

const SOURCES = [{ id: 1, name: "Movies", path: "/media/movies", created_at: "2026-01-01T00:00:00Z" }];
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 100,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];

function renderScreen(sources: typeof SOURCES = SOURCES, media: typeof MEDIA = MEDIA) {
  server.use(
    http.get("/api/sources", () => HttpResponse.json(sources)),
    http.get("/api/media", () => HttpResponse.json(media))
  );
  const client = createTestQueryClient();
  render(<SettingsScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("SettingsScreen", () => {
  it("lists each source with its path and item count", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    expect(screen.getByText("/media/movies — 1 item(s)")).toBeInTheDocument();
  });

  it("shows an empty state with no sources configured", async () => {
    renderScreen([]);
    expect(await screen.findByText("No media sources configured yet.")).toBeInTheDocument();
  });

  it("adds a new source via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/sources", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json({ id: 2, ...(created as object), created_at: "2026-01-01T00:00:00Z" }, { status: 201 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.type(screen.getByLabelText("Name"), "TV");
    await userEvent.type(screen.getByLabelText("Path"), "/media/tv");
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));

    await waitFor(() => expect(created).toEqual({ name: "TV", path: "/media/tv" }));
  });

  it("shows a scanning state while a rescan is in flight", async () => {
    server.use(
      http.post("/api/sources/1/scan", async () => {
        await delay(50);
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.click(screen.getByRole("button", { name: "Rescan" }));
    expect(await screen.findByRole("button", { name: "Scanning…" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Rescan" })).toBeInTheDocument());
  });

  it("requires a confirmation click before removing a source", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/sources/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(deleted).toBe(false);
    expect(screen.getByText("Remove this source and all its media/programs?")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Confirm remove" }));
    await waitFor(() => expect(deleted).toBe(true));
  });
});
