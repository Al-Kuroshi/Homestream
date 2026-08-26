import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { ResolvedSlot, Slot } from "../api/types";
import { startOfWeekUTC } from "../scheduling/week";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelScheduleScreen } from "./ChannelScheduleScreen";

// The screen only ever requests/renders the *current* real-clock week (week
// navigation past that is out of this task's read-only scope), so the
// fixture's resolved-slot occurrence must land inside whatever week is
// "current" when the suite actually runs. Deriving it from the real clock
// (rather than a hardcoded date) keeps the test deterministic on any
// calendar date, with no system-clock mocking required.
const MS_PER_HOUR = 60 * 60 * 1000;
const MONDAY_THIS_WEEK = new Date(startOfWeekUTC(new Date()).getTime() + 24 * MS_PER_HOUR);

const CHANNEL = { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "", updated_at: "" };
const MEDIA = [
  { id: 5, source_id: 1, rel_path: "a.mp4", title: "Fury", duration_sec: 3600, video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1, mod_time: "", invalid: false, created_at: "", updated_at: "" },
];
const SLOTS: Slot[] = [
  { id: 1, channel_id: 1, kind: "media", media_item_id: 5, gap_duration_sec: null, gap_label: "", recurring: true, day_of_week: 1, position: 1000, start_time: null, created_at: "", updated_at: "" },
];
const RESOLVED: ResolvedSlot[] = [
  {
    program_id: 1,
    media_item_id: 5,
    start_time: MONDAY_THIS_WEEK.toISOString(),
    end_time: new Date(MONDAY_THIS_WEEK.getTime() + MS_PER_HOUR).toISOString(),
  },
];

// Takes overrides so every route is registered in one server.use() call —
// registering a test's overrides in a separate, earlier server.use() call
// (as a naive version of this helper did) loses: MSW's later-registered
// handler wins, so a per-test override registered before renderScreen()'s
// own defaults gets shadowed by them instead of taking priority.
function renderScreen(overrides?: { slots?: Slot[]; resolved?: ResolvedSlot[] }) {
  server.use(
    http.get("/api/channels/1", () => HttpResponse.json(CHANNEL)),
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/channels/1/slots", () => HttpResponse.json(overrides?.slots ?? SLOTS)),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json(overrides?.resolved ?? RESOLVED))
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
  it("renders the channel name, 7 day columns, and a media-library panel entry per media item", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    for (const day of ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]) {
      expect(screen.getByText(day, { exact: false })).toBeInTheDocument();
    }
    expect(await screen.findByText("Fury", { selector: ".media-library-item *, .media-library-item" })).toBeTruthy();
  });

  it("renders a resolved slot as a block on its day, falling back to the media title (no cover art wired up yet)", async () => {
    renderScreen();
    const block = await screen.findByTestId("slot-block-1");
    expect(block).toHaveTextContent("Fury");
    expect(block.querySelector("img")).toBeNull();
  });

  it("renders a gap slot's label instead of a media title", async () => {
    renderScreen({
      slots: [
        { id: 2, channel_id: 1, kind: "gap", media_item_id: null, gap_duration_sec: 300, gap_label: "Ad Break", recurring: true, day_of_week: 1, position: 1000, start_time: null, created_at: "", updated_at: "" },
      ],
      resolved: [
        {
          program_id: 2,
          media_item_id: 0,
          start_time: MONDAY_THIS_WEEK.toISOString(),
          end_time: new Date(MONDAY_THIS_WEEK.getTime() + 5 * 60 * 1000).toISOString(),
        },
      ],
    });
    const block = await screen.findByTestId("slot-block-2");
    expect(block).toHaveTextContent("Ad Break");
  });
});
