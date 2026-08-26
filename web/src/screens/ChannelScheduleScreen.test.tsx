import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
    kind: "media",
    gap_label: "",
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

function dragEventWithData(data: unknown) {
  return { dataTransfer: { getData: () => JSON.stringify(data), setData: () => {} } };
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

  // The grid's day/midnight boundaries are UTC while the Guide and TV
  // screens render clock times locally — the label is what keeps a non-UTC
  // viewer from silently seeing two different answers for the same slot.
  it("labels the grid's times as UTC", async () => {
    renderScreen();
    expect(await screen.findByText("(all times UTC)")).toBeInTheDocument();
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
          kind: "gap",
          gap_label: "Ad Break",
          start_time: MONDAY_THIS_WEEK.toISOString(),
          end_time: new Date(MONDAY_THIS_WEEK.getTime() + 5 * 60 * 1000).toISOString(),
        },
      ],
    });
    const block = await screen.findByTestId("slot-block-2");
    expect(block).toHaveTextContent("Ad Break");
  });

  it("drops a media item from the library panel onto an empty day, adding a recurring slot at position 1000", async () => {
    renderScreen({ slots: [], resolved: [] });
    // Captured rather than asserted inline in the handler — see the "moves
    // an existing slot" test below for why an inline expect() here would
    // never actually gate the test's pass/fail.
    let capturedBody: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/channels/1/slots", async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ id: 9, ...capturedBody }, { status: 201 });
      })
    );

    const mediaItem = await screen.findByText("Fury");
    const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
    fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
    fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));
    // Dropping only stages a pending placement now (Task 11); confirming
    // with the default recurring checkbox fires the actual mutation.
    fireEvent.click(await screen.findByText("Add"));

    await waitFor(() =>
      expect(capturedBody).toMatchObject({ kind: "media", media_item_id: 5, recurring: true, position: 1000 })
    );
  });

  // CSS :hover is not reliable during a native HTML5 drag, so the active
  // state has to come from onDragEnter/onDragLeave.
  it("highlights a drop zone while a drag is over it, and clears the highlight on leave", async () => {
    renderScreen();
    const zone = await screen.findByTestId("day-drop-zone-1-start");
    expect(zone.className).not.toContain("day-drop-zone-active");

    fireEvent.dragEnter(zone);
    expect(zone.className).toContain("day-drop-zone-active");

    fireEvent.dragLeave(zone);
    expect(zone.className).not.toContain("day-drop-zone-active");
  });

  it("clears the drop-zone highlight once the drop happens", async () => {
    renderScreen({ slots: [], resolved: [] });
    const zone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
    fireEvent.dragEnter(zone);
    expect(zone.className).toContain("day-drop-zone-active");

    fireEvent.drop(zone, dragEventWithData({ mediaItemId: 5 }));
    expect(zone.className).not.toContain("day-drop-zone-active");
  });

  it("shows an inline error when the backend rejects a placement", async () => {
    renderScreen({ slots: [], resolved: [] });
    server.use(
      http.post("/api/channels/1/slots", () => HttpResponse.json({ error: "doesn't fit: this day is already full" }, { status: 400 }))
    );

    const mediaItem = await screen.findByText("Fury");
    const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
    fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
    fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));
    fireEvent.click(await screen.findByText("Add"));

    expect(await screen.findByRole("alert")).toHaveTextContent("doesn't fit: this day is already full");
  });

  it("moves an existing slot when it's dragged to a different drop zone", async () => {
    renderScreen(); // defaults: SLOTS has id=1 (day_of_week: 1, position: 1000), RESOLVED has a matching occurrence on MONDAY_THIS_WEEK
    // Captured (rather than asserted inline in the handler) so the test can
    // `await waitFor` on it below — an `expect()` thrown inside an MSW
    // handler that nothing ever awaits doesn't fail the test; it's a
    // rejected promise the test has already returned past. Confirmed by
    // temporarily asserting an impossible value here and observing the
    // test still reported green.
    let capturedBody: Record<string, unknown> | undefined;
    server.use(
      http.put("/api/slots/1", async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ id: 1, ...capturedBody });
      })
    );

    const block = await screen.findByTestId("slot-block-1");
    const endZone = await screen.findByTestId("day-drop-zone-1-end");
    fireEvent.dragStart(block, dragEventWithData({ existingSlotId: 1 }));
    fireEvent.drop(endZone, dragEventWithData({ existingSlotId: 1 }));
    fireEvent.click(await screen.findByText("Add"));

    // position 1000, not 2000: handleDrop's existingPositions excludes the
    // dragged slot's own id from the target day before calling
    // positionForInsert (correctly — a slot's new position shouldn't be
    // computed relative to its own old position). Since slot 1 is this
    // day's only occupant, excluding it leaves existingPositions empty, so
    // positionForInsert returns the empty-list default (1000) regardless of
    // insertBeforeIndex. Verified directly against positionForInsert.
    await waitFor(() => expect(capturedBody).toMatchObject({ recurring: true, position: 1000 }));
  });

  it("dropping the Gap entry opens a duration prompt, and confirming adds a recurring gap slot", async () => {
    renderScreen({ slots: [], resolved: [] });
    let capturedBody: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/channels/1/slots", async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ id: 9, ...capturedBody }, { status: 201 });
      })
    );

    const gapEntry = await screen.findByText("Gap / Break");
    const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
    fireEvent.dragStart(gapEntry, dragEventWithData({ gap: true }));
    fireEvent.drop(dropZone, dragEventWithData({ gap: true }));

    fireEvent.change(await screen.findByLabelText("Gap duration (minutes)"), { target: { value: "5" } });
    fireEvent.click(screen.getByText("Add gap"));

    await waitFor(() => expect(capturedBody).toMatchObject({ kind: "gap", gap_duration_sec: 300, recurring: true, position: 1000 }));
  });

  // PUT /api/slots/{id} is a full replace, so a move that doesn't echo the
  // gap's existing duration/label back silently rewrites them to the "new
  // gap" defaults (5 minutes, "Gap").
  it("preserves an existing gap's duration and label when it's moved", async () => {
    const gapSlot: Slot = {
      id: 2, channel_id: 1, kind: "gap", media_item_id: null, gap_duration_sec: 1800, gap_label: "Ad Break",
      recurring: true, day_of_week: 1, position: 1000, start_time: null, created_at: "", updated_at: "",
    };
    renderScreen({
      slots: [gapSlot],
      resolved: [
        {
          program_id: 2,
          media_item_id: 0,
          kind: "gap",
          gap_label: "Ad Break",
          start_time: MONDAY_THIS_WEEK.toISOString(),
          end_time: new Date(MONDAY_THIS_WEEK.getTime() + 30 * 60 * 1000).toISOString(),
        },
      ],
    });
    let capturedBody: Record<string, unknown> | undefined;
    server.use(
      http.put("/api/slots/2", async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ id: 2, ...capturedBody });
      })
    );

    const block = await screen.findByTestId("slot-block-2");
    const endZone = await screen.findByTestId("day-drop-zone-1-end");
    fireEvent.dragStart(block, dragEventWithData({ existingSlotId: 2 }));
    fireEvent.drop(endZone, dragEventWithData({ existingSlotId: 2 }));
    // Confirm without touching the duration input at all.
    fireEvent.click(await screen.findByText("Add gap"));

    await waitFor(() =>
      expect(capturedBody).toMatchObject({ kind: "gap", gap_duration_sec: 1800, gap_label: "Ad Break" })
    );
  });

  it("deletes a slot when its × button is clicked", async () => {
    renderScreen();
    let deletedId: string | undefined;
    server.use(
      http.delete("/api/slots/:id", ({ params }) => {
        deletedId = params.id as string;
        return new HttpResponse(null, { status: 204 });
      })
    );

    fireEvent.click(await screen.findByRole("button", { name: "Delete Fury" }));

    await waitFor(() => expect(deletedId).toBe("1"));
  });

  it("shows an inline error when deleting a slot fails", async () => {
    renderScreen();
    server.use(
      http.delete("/api/slots/:id", () => HttpResponse.json({ error: "slot is gone" }, { status: 500 }))
    );

    fireEvent.click(await screen.findByRole("button", { name: "Delete Fury" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("slot is gone");
  });

  it("unchecking Repeats weekly places a one-off slot with the dropped column's exact date instead of day_of_week/position", async () => {
    renderScreen({ slots: [], resolved: [] });
    let capturedBody: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/channels/1/slots", async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ id: 9, ...capturedBody }, { status: 201 });
      })
    );

    const mediaItem = await screen.findByText("Fury");
    const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
    fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
    fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));

    fireEvent.click(await screen.findByLabelText("Repeats weekly"));
    fireEvent.click(screen.getByText("Add"));

    await waitFor(() => {
      expect(capturedBody).toMatchObject({ kind: "media", media_item_id: 5, recurring: false });
      expect(capturedBody?.start_time).toBeTruthy();
      expect(capturedBody?.day_of_week).toBeUndefined();
      expect(capturedBody?.position).toBeUndefined();
    });
  });
});
