import { fireEvent, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { TVScreen } from "./TVScreen";

vi.mock("../components/VideoPlayer", () => ({
  VideoPlayer: (props: { mode: string; src: string; offsetSec?: number }) => (
    <div data-testid="video-player" data-mode={props.mode} data-src={props.src} data-offset={props.offsetSec} />
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
});
