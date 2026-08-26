import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { GuideScreen } from "./GuideScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Off Channel", description: "", enabled: false, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
// end_time = start_time + MEDIA[0].duration_sec (3600s = 1h), matching what
// the resolved-window endpoint always returns as a concrete field now.
const RESOLVED_CH1 = [
  { program_id: 1, media_item_id: 1, start_time: "2026-01-01T19:00:00Z", end_time: "2026-01-01T20:00:00Z" },
];

function renderScreen() {
  server.use(
    http.get("/api/channels", () => HttpResponse.json(CHANNELS)),
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json(RESOLVED_CH1)),
    http.get("/api/channels/2/slots/resolved", () => HttpResponse.json([]))
  );
  const client = createTestQueryClient();
  return render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("GuideScreen", () => {
  beforeEach(() => {
    // Only Date is faked (not setTimeout/setInterval) so React Testing
    // Library's async findBy*/waitFor polling — which relies on real
    // timers — keeps working; only "what time is it" is under test control.
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders a row only for enabled channels", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    expect(screen.queryByText("Off Channel")).not.toBeInTheDocument();
  });

  it("renders the scheduled program and Off Air blocks for the surrounding gaps", async () => {
    renderScreen();
    await screen.findByText("Movies");
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    // Window is [17:00, 23:00); the one program runs 19:00-20:00, so there
    // are two gaps: before it (17:00-19:00) and after it (20:00-23:00).
    expect(screen.getAllByText("Off Air")).toHaveLength(2);
  });

  it("shows the now-line when the current time falls within the default window", async () => {
    renderScreen();
    await screen.findByText("Movies");
    expect(screen.getByTestId("now-line")).toBeInTheDocument();
  });

  it("hides the now-line once the live clock drifts outside the default (mount-anchored) window", async () => {
    // The default window is anchored at mount, not recentered every render
    // (see GuideScreen.tsx) — so to observe it going stale we must mount at
    // baseline, advance the clock afterward, and force a re-render, rather
    // than jumping the clock before the first render (which would just
    // anchor a brand-new window around the jumped time).
    const { rerender } = renderScreen();
    await screen.findByText("Movies");
    expect(screen.getByTestId("now-line")).toBeInTheDocument();

    vi.setSystemTime(new Date("2026-01-03T00:00:00Z"));
    rerender(<GuideScreen />);

    expect(screen.queryByTestId("now-line")).not.toBeInTheDocument();
  });

  it("polls channels and media every 30s to catch schedule changes made elsewhere", async () => {
    server.use(
      http.get("/api/channels", () => HttpResponse.json(CHANNELS)),
      http.get("/api/media", () => HttpResponse.json(MEDIA)),
      http.get("/api/channels/1/slots/resolved", () => HttpResponse.json(RESOLVED_CH1)),
      http.get("/api/channels/2/slots/resolved", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });
    await screen.findByText("Movies");

    function refetchIntervalFor(queryKey: string[]) {
      const options = client.getQueryCache().find({ queryKey })?.options as { refetchInterval?: number } | undefined;
      return options?.refetchInterval;
    }
    expect(refetchIntervalFor(["channels"])).toBe(30_000);
    expect(refetchIntervalFor(["media"])).toBe(30_000);
  });

  it("shows a Schedule unavailable state (not Off Air) when a channel's programs request fails, without affecting other rows", async () => {
    server.use(
      http.get("/api/channels", () => HttpResponse.json(CHANNELS)),
      http.get("/api/media", () => HttpResponse.json(MEDIA)),
      http.get("/api/channels/1/slots/resolved", () => HttpResponse.json(RESOLVED_CH1)),
      http.get("/api/channels/2/slots/resolved", () => HttpResponse.json([]))
    );
    // CHANNELS only has one enabled channel (id 1, "Movies"); add a second
    // enabled channel whose programs request fails, so we can assert its
    // row shows "Schedule unavailable" while channel 1's row is unaffected.
    const CHANNELS_WITH_FAILING = [
      CHANNELS[0],
      { id: 3, name: "Broken", description: "", enabled: true, position: 2, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
    ];
    server.use(
      http.get("/api/channels", () => HttpResponse.json(CHANNELS_WITH_FAILING)),
      http.get("/api/channels/3/slots/resolved", () => HttpResponse.json({ error: "boom" }, { status: 500 }))
    );
    const client = createTestQueryClient();
    render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });

    expect(await screen.findByText("Schedule unavailable")).toBeInTheDocument();
    // Channel 1's row is unaffected: it still renders its real program and
    // Off Air gaps, not the error state.
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    expect(screen.getAllByText("Off Air")).toHaveLength(2);
  });

  it("shows an empty-state message when there are no enabled channels", async () => {
    server.use(
      http.get("/api/channels", () => HttpResponse.json([CHANNELS[1]])),
      http.get("/api/media", () => HttpResponse.json([])),
      http.get("/api/channels/2/slots/resolved", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });
    expect(await screen.findByText("No enabled channels to show.")).toBeInTheDocument();
  });
});
