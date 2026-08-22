import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { LibraryScreen } from "./LibraryScreen";

const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3725,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: 2, source_id: 1, rel_path: "b.mp4", title: "Broken B", duration_sec: 0,
    video_codec: "", audio_codec: "", container: "", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: true,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: 3, source_id: 2, rel_path: "c.mp4", title: "Show C", duration_sec: 1500,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
const SOURCES = [
  { id: 1, name: "Movies", path: "/media/movies", created_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "TV", path: "/media/tv", created_at: "2026-01-01T00:00:00Z" },
];

function renderScreen() {
  server.use(
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/sources", () => HttpResponse.json(SOURCES))
  );
  const client = createTestQueryClient();
  render(<LibraryScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("LibraryScreen", () => {
  it("renders every media item as a row, with duration formatted h:mm:ss", async () => {
    renderScreen();
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Broken B")).toBeInTheDocument();
    expect(screen.getByText("1:02:05")).toBeInTheDocument();
  });

  it("shows the owning source's name and an Invalid status for broken items", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    // Scoped to table cells so this doesn't also match the "Filter by
    // source" <select>'s <option>Movies</option>, which shares the same text.
    expect(screen.getAllByRole("cell", { name: "Movies" })).toHaveLength(2);
    expect(screen.getByText("Invalid")).toBeInTheDocument();
    expect(screen.getAllByText("OK")).toHaveLength(2); // Movie A and Show C
  });

  it("filters by search text", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.type(screen.getByLabelText("Search titles"), "Movie A");
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.queryByText("Broken B")).not.toBeInTheDocument();
  });

  it("filters to invalid-only items", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.click(screen.getByLabelText("Invalid only"));
    expect(screen.queryByText("Movie A")).not.toBeInTheDocument();
    expect(screen.getByText("Broken B")).toBeInTheDocument();
  });

  it("filters by source", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    expect(screen.getByText("Show C")).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText("Filter by source"), "1");
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Broken B")).toBeInTheDocument();
    expect(screen.queryByText("Show C")).not.toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText("Filter by source"), "all");
    expect(screen.getByText("Show C")).toBeInTheDocument();
  });

  it("shows an empty-state message when no media matches the filters", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.type(screen.getByLabelText("Search titles"), "nothing matches this");
    expect(screen.getByText("No media matches the current filters.")).toBeInTheDocument();
  });
});
