import { act, fireEvent, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { TVScreen } from "./TVScreen";

// The mock exposes buttons that call the onError/onTimeUpdate props it
// receives, so tests can simulate video playback events (a fatal video
// error, or the video element reporting its currentTime) that the real
// <video> element would normally trigger — neither of which vitest/jsdom
// can produce on its own.
vi.mock("../components/VideoPlayer", () => ({
  VideoPlayer: (props: {
    mode: string;
    src: string;
    offsetSec?: number;
    onError: () => void;
    onTimeUpdate?: (currentTimeSec: number) => void;
  }) => (
    <div data-testid="video-player" data-mode={props.mode} data-src={props.src} data-offset={props.offsetSec}>
      <button onClick={() => props.onError()}>Simulate video error</button>
      <button onClick={() => props.onTimeUpdate?.(50)}>Report currentTime 50</button>
      <button onClick={() => props.onTimeUpdate?.(8)}>Report currentTime 8</button>
    </div>
  ),
}));

// Mocked (rather than left real) so tests can assert on the exact
// currentTimeSec TVScreen computes and passes down, the same way the
// VideoPlayer mock above exposes data-mode/data-src/data-offset. Still
// renders title/next as plain text so the earlier tests that assert on
// "Movie A" / "Next: Movie B" keep working unchanged.
vi.mock("../components/NowPlayingOverlay", () => ({
  NowPlayingOverlay: (props: {
    title: string;
    currentTimeSec: number;
    durationSec: number;
    nextTitle: string | null;
  }) => (
    <div data-testid="now-playing-overlay" data-current-time-sec={props.currentTimeSec} data-duration-sec={props.durationSec}>
      <p>{props.title}</p>
      {props.nextTitle && <p>Next: {props.nextTitle}</p>}
    </div>
  ),
}));

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Comedy", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];
const MEDIA = [
  {
    id: 5, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: 6, source_id: 1, rel_path: "b.mp4", title: "Movie B", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];

function renderScreen(channelPath = "/tv/1") {
  server.use(http.get("/api/channels", () => HttpResponse.json(CHANNELS)), http.get("/api/media", () => HttpResponse.json(MEDIA)));
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={[channelPath]}>
      <Routes>
        <Route path="/tv/:channelId" element={<TVScreen />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("TVScreen", () => {
  beforeEach(() => {
    localStorage.clear();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders VideoPlayer with the direct-play src and offset, and the now-playing title/next", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 42 })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: { program_id: 1, media_item_id: 5, start_time: "2026-01-01T17:59:18Z", end_time: "2026-01-01T20:00:00Z" },
          offset_sec: 42,
          next: { program_id: 2, media_item_id: 6, start_time: "2026-01-01T20:00:00Z", end_time: "2026-01-01T21:00:00Z" },
        })
      )
    );
    renderScreen();

    const player = await screen.findByTestId("video-player");
    expect(player).toHaveAttribute("data-mode", "direct");
    expect(player).toHaveAttribute("data-src", "/api/media/5/stream");
    expect(player).toHaveAttribute("data-offset", "42");
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    expect(await screen.findByText("Next: Movie B")).toBeInTheDocument();
  });

  it("renders VideoPlayer with the hls session playlist src and no offset", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "hls", media_item_id: 5, offset_sec: 42, session_id: "abc" })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: { program_id: 1, media_item_id: 5, start_time: "2026-01-01T17:59:18Z", end_time: "2026-01-01T20:00:00Z" },
          offset_sec: 42,
          next: null,
        })
      )
    );
    renderScreen();

    const player = await screen.findByTestId("video-player");
    expect(player).toHaveAttribute("data-mode", "hls");
    expect(player).toHaveAttribute("data-src", "/api/playback/sessions/abc/playlist.m3u8");
    expect(player.getAttribute("data-offset")).toBeNull();
  });

  it("shows the off_air interstitial with the next program's title", async () => {
    server.use(
      http.post("/api/channels/1/watch", () => HttpResponse.json({ status: "off_air" })),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: null,
          offset_sec: 0,
          next: { program_id: 2, media_item_id: 6, start_time: "2026-01-01T18:05:00Z", end_time: "2026-01-01T19:00:00Z" },
        })
      )
    );
    renderScreen();

    expect(await screen.findByText("Nothing scheduled right now")).toBeInTheDocument();
    expect(await screen.findByText(/Up next: Movie B/)).toBeInTheDocument();
  });

  it("shows the unavailable interstitial", async () => {
    server.use(
      http.post("/api/channels/1/watch", () => HttpResponse.json({ status: "unavailable" })),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      )
    );
    renderScreen();

    expect(await screen.findByText("Currently unavailable")).toBeInTheDocument();
  });

  it("shows an error state with a Retry button when tuning in fails, and retry re-attempts it", async () => {
    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", () => {
        watchCallCount += 1;
        return watchCallCount === 1
          ? HttpResponse.json({ error: "boom" }, { status: 500 })
          : HttpResponse.json({ status: "off_air" });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      )
    );
    renderScreen();

    fireEvent.click(await screen.findByText("Retry"));
    expect(await screen.findByText("Nothing scheduled right now")).toBeInTheDocument();
    expect(watchCallCount).toBe(2);
  });

  it("persists the tuned-in channel id to localStorage", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 0 })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      )
    );
    renderScreen();

    await screen.findByTestId("video-player");
    expect(localStorage.getItem("personaltv.tv.lastChannelId")).toBe("1");
  });

  it("switches channels via ChannelSwitcher's next-channel control", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 0 })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      ),
      http.post("/api/channels/2/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 6, offset_sec: 0 })
      ),
      http.get("/api/channels/2/now", () =>
        HttpResponse.json({ channel_id: 2, current: null, offset_sec: 0, next: null })
      )
    );
    renderScreen();

    await screen.findByText("Movie A");
    fireEvent.click(await screen.findByLabelText("Next channel"));
    expect(await screen.findByText("Movie B")).toBeInTheDocument();
  });

  it("shows the error state with Retry when VideoPlayer reports a playback error, and retry re-attempts tune-in", async () => {
    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", () => {
        watchCallCount += 1;
        return HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 0 });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      )
    );
    renderScreen();

    await screen.findByTestId("video-player");
    expect(watchCallCount).toBe(1);

    fireEvent.click(screen.getByText("Simulate video error"));

    expect(await screen.findByText("Something went wrong tuning in.")).toBeInTheDocument();
    expect(screen.getByText("Retry")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Retry"));

    await screen.findByTestId("video-player");
    expect(watchCallCount).toBe(2);
  });

  it("passes the raw reported currentTime straight through to NowPlayingOverlay in direct mode", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 42 })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 42, next: null })
      )
    );
    renderScreen();

    await screen.findByTestId("video-player");
    fireEvent.click(screen.getByText("Report currentTime 50"));

    const overlay = await screen.findByTestId("now-playing-overlay");
    expect(overlay).toHaveAttribute("data-current-time-sec", "50");
  });

  it("adds offsetSec back on top of the raw reported currentTime in hls mode", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "hls", media_item_id: 5, offset_sec: 42, session_id: "abc" })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 42, next: null })
      )
    );
    renderScreen();

    await screen.findByTestId("video-player");
    fireEvent.click(screen.getByText("Report currentTime 8"));

    const overlay = await screen.findByTestId("now-playing-overlay");
    expect(overlay).toHaveAttribute("data-current-time-sec", "50");
  });

  it("keeps the .tv-screen chrome and ChannelSwitcher mounted through the loading window of a self-scheduled re-tune-in", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));

    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", async () => {
        watchCallCount += 1;
        if (watchCallCount === 1) {
          return HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 0 });
        }
        // Deliberately slow on the second call, so the test can observe
        // the in-between "loading" state useTuneIn sets synchronously
        // before this resolves — the exact window the chrome must survive
        // (a program boundary or channel switch, not just first mount).
        await new Promise((resolve) => setTimeout(resolve, 10_000));
        return HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 6, offset_sec: 0 });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: { program_id: 1, media_item_id: 5, start_time: "2026-01-01T17:59:55Z", end_time: "2026-01-01T18:00:05Z" },
          offset_sec: 5,
          next: null,
        })
      )
    );

    renderScreen();

    // Testing Library's findBy*/waitFor only auto-detects Jest's fake
    // timers (it checks for a global `jest`), not Vitest's — under
    // vi.useFakeTimers() it falls back to polling via a real setInterval,
    // which is itself faked and never fires on its own. So every
    // assertion below uses the synchronous getByTestId/queryByTestId
    // (never findBy*) once each act(async advanceTimersByTimeAsync(...))
    // has already flushed the relevant state/render — the same pattern
    // api/playback.test.ts uses for this hook's own timer tests.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId("video-player")).toBeInTheDocument();
    expect(watchCallCount).toBe(1);

    // current.end_time is 5s after mocked "now" — advancing past it fires
    // useTuneIn's self-scheduled re-tune-in timer, which sets state back
    // to "loading" before the (deliberately slow) second /watch response
    // resolves.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });

    expect(screen.getByText("Tuning in…")).toBeInTheDocument();
    expect(screen.getByLabelText("Next channel")).toBeInTheDocument();
    expect(screen.queryByTestId("video-player")).not.toBeInTheDocument();

    // Let the slow second response settle so nothing leaks past the test.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(screen.getByTestId("video-player")).toBeInTheDocument();
    expect(watchCallCount).toBe(2);
  });
});
