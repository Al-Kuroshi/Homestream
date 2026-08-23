import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelScheduleScreen } from "./ChannelScheduleScreen";

const CHANNEL = {
  id: 1, name: "Movies", description: "", enabled: true, position: 0,
  created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
};
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 5400,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
const PROGRAMS = [
  { id: 1, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderScreen(programs: typeof PROGRAMS = PROGRAMS) {
  server.use(
    http.get("/api/channels/1", () => HttpResponse.json(CHANNEL)),
    http.get("/api/channels/1/programs", () => HttpResponse.json(programs)),
    http.get("/api/media", () => HttpResponse.json(MEDIA))
  );
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={["/channels/1"]}>
      <Routes>
        <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("ChannelScheduleScreen", () => {
  it("renders the channel name and each program with its computed end time", async () => {
    renderScreen();
    expect(await screen.findByRole("heading", { name: "Movies" })).toBeInTheDocument();
    // "Movie A" also appears as an <option> in the "Add a program" media
    // select below, so scope this assertion to the program list to avoid
    // an ambiguous multi-match.
    const list = screen.getByRole("list");
    expect(within(list).getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("06:00 PM – 07:30 PM")).toBeInTheDocument();
  });

  it("shows an empty state with no programs scheduled", async () => {
    renderScreen([]);
    expect(await screen.findByText("No programs scheduled yet.")).toBeInTheDocument();
  });

  it("adds a program via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/channels/1/programs", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(
          { id: 2, channel_id: 1, ...(created as object), created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
          { status: 201 }
        );
      })
    );
    renderScreen();
    // Wait for the computed end time, not just "Movie A" (which is
    // ambiguous with the media <option> in the add-program form below) —
    // this text only renders once both the programs and media queries
    // have resolved, so it also guarantees the "Media" select is populated.
    await screen.findByText("06:00 PM – 07:30 PM");

    await userEvent.selectOptions(screen.getByLabelText("Media"), "1");
    fireEvent.change(screen.getByLabelText("Start time"), { target: { value: "2026-01-02T10:00" } });
    await userEvent.click(screen.getByRole("button", { name: "Add program" }));

    await waitFor(() =>
      expect(created).toEqual({ media_item_id: 1, start_time: new Date("2026-01-02T10:00").toISOString() })
    );
  });

  it("edits a program's start time", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/programs/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...PROGRAMS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    // Wait for the computed end time, not just "Movie A" (which is
    // ambiguous with the media <option> in the add-program form below) —
    // this text only renders once both the programs and media queries
    // have resolved, so it also guarantees the "Media" select is populated.
    await screen.findByText("06:00 PM – 07:30 PM");

    await userEvent.click(screen.getByRole("button", { name: "Edit start time" }));
    fireEvent.change(screen.getByLabelText("Edit start time"), { target: { value: "2026-01-01T20:00" } });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(putBody).toEqual({ media_item_id: 1, start_time: new Date("2026-01-01T20:00").toISOString() })
    );
  });

  it("removes a program", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/programs/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    // Wait for the computed end time, not just "Movie A" (which is
    // ambiguous with the media <option> in the add-program form below) —
    // this text only renders once both the programs and media queries
    // have resolved, so it also guarantees the "Media" select is populated.
    await screen.findByText("06:00 PM – 07:30 PM");

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(deleted).toBe(true));
  });

  it("shows an error message when removing a program fails", async () => {
    server.use(
      http.delete("/api/programs/1", () => HttpResponse.json({ error: "cannot remove: program is currently airing" }, { status: 500 }))
    );
    renderScreen();
    await screen.findByText("06:00 PM – 07:30 PM");

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("cannot remove: program is currently airing");
  });
});
