# Personal TV — TV/Player Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the TV/player screen — the fifth and final MVP frontend screen — a video player with channel switching, an off-air/unavailable interstitial with a next-up countdown, and an auto-hiding now-playing overlay, driven entirely by the three existing playback endpoints and the existing `/now` endpoint. Frontend-only; no backend changes.

**Architecture:** A new `useTuneIn` hook (`web/src/api/playback.ts`) owns an event-driven tune-in flow with no polling loop: it calls `POST /channels/{id}/watch` + `GET /channels/{id}/now` together, derives a discriminated state, and sets at most one self-scheduling `setTimeout` per tune-in (for when the current program ends, or when the next one starts) rather than an interval poll — matching the product's "pure function of schedule + wall clock, recomputed on demand" principle. `TVScreen.tsx` (route: `/tv/:channelId`) consumes that hook and renders one of: a loading state, an error state with retry, `Interstitial` (off-air/unavailable, with a live countdown to the next program), or `VideoPlayer` (direct `<video>` playback, or `hls.js` for transcoded sessions) plus `NowPlayingOverlay` (auto-hiding title/progress/next-up) and `ChannelSwitcher` (prev/next + a channel-list overlay). `TVIndexScreen.tsx` (route: `/tv`) resolves to a concrete channel (last-watched via `localStorage`, or the first enabled one) and redirects.

**Tech Stack:** React + TypeScript (existing `web/` app), TanStack Query (only for the existing `useChannels`/`useMediaItems` hooks this screen reuses — `useTuneIn` itself is plain `useState`/`useEffect`, not a query, since its POST side effect starts a transcode session and must not be subject to query caching/retry), React Router, Vitest/RTL/MSW, and a new dependency: `hls.js` (MIT-licensed, the standard browser HLS player, needed for every browser except Safari, which plays HLS natively).

**Spec:** `docs/design/2026-08-24-personal-tv-player-design.md`

## Global Constraints

- Frontend-only. No changes to any Go file, any backend route, or the API contract — this plan only ever calls existing endpoints (`POST /channels/{id}/watch`, `GET /channels/{id}/now`, `GET /media/{id}/stream`, `GET /playback/sessions/{id}/{file}`).
- **No polling loop anywhere in this plan.** Every re-check of channel state is a single `setTimeout` scheduled from a known schedule time (a program's `end_time`, or the next program's `start_time`), never a `setInterval`.
- **`offset_sec` must never be applied to the video element in `hls` mode** — `ffmpeg` already seeked via `-ss` when the session started server-side (per `TuneInResult`'s doc comment, added during the playback backend's final review); only `direct` mode sets `video.currentTime = offsetSec`.
- Colocated Vitest/RTL tests (`Foo.tsx` / `Foo.test.tsx`), black-box by default (render and assert on what a user would see), MSW-mocked at the network boundary — matching every existing screen/component in `web/src`.
- Plain CSS per component (`Foo.css` alongside `Foo.tsx`), no UI framework — matching `Sidebar.css`/`MutationError.css`/every existing screen's own `.css` file.
- All new frontend types/API functions follow the exact conventions already established in `web/src/api/*.ts`: `apiGet`/`apiPost` from `./http`, `snake_case` JSON field names matching the Go backend's response bodies exactly, hooks named `use*`.
- `cd web && npx tsc -b && npm run lint && npm test` must all stay clean throughout (matching `CLAUDE.md`'s existing frontend verification commands).

---

## Task 1: `useTuneIn` — the tune-in event flow

**Files:**
- Create: `web/src/api/playback.ts`
- Test: `web/src/api/playback.test.ts`

**Interfaces:**
- Consumes: `apiGet`/`apiPost` from `web/src/api/http.ts` (existing).
- Produces: `WatchResponse`, `NowResponse`, `ProgramState`, `TuneInState`, `watchChannel(channelId: number): Promise<WatchResponse>`, `getChannelNow(channelId: number): Promise<NowResponse>`, `useTuneIn(channelId: number): { state: TuneInState; retune: () => void }`. **Task 6's `TVScreen` is the only consumer of `useTuneIn`; `watchChannel`/`getChannelNow` have no other in-plan consumer (they're exercised only through the hook, matching this codebase's existing convention of testing hooks rather than the raw fetch functions they wrap — see `web/src/api/channels.ts`/`channels.test.ts`).**

- [ ] **Step 1: Write the failing tests**

`web/src/api/playback.test.ts`:

```ts
import { act, renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "../test/server";
import { useTuneIn } from "./playback";

describe("useTuneIn", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports playing/direct with the tuned-in mode, offset, and next program", async () => {
    server.use(
      http.post("/api/channels/1/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 42 })
      ),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: { program_id: 1, media_item_id: 5, start_time: "2026-01-01T17:59:18Z", end_time: "2026-01-01T19:00:00Z" },
          offset_sec: 42,
          next: { program_id: 2, media_item_id: 6, start_time: "2026-01-01T19:00:00Z", end_time: "2026-01-01T20:00:00Z" },
        })
      )
    );

    const { result } = renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(result.current.state).toEqual({
      status: "playing",
      mode: "direct",
      mediaItemId: 5,
      offsetSec: 42,
      sessionId: undefined,
      next: { mediaItemId: 6, startTime: new Date("2026-01-01T19:00:00Z") },
    });
  });

  it("re-tunes in automatically when the current program's scheduled end time arrives", async () => {
    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", () => {
        watchCallCount += 1;
        return watchCallCount === 1
          ? HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 5, offset_sec: 42 })
          : HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 6, offset_sec: 0 });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: { program_id: 1, media_item_id: 5, start_time: "2026-01-01T17:59:18Z", end_time: "2026-01-01T18:00:10Z" },
          offset_sec: 42,
          next: null,
        })
      )
    );

    const { result } = renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.state).toMatchObject({ status: "playing", mediaItemId: 5 });

    // current.end_time is 18:00:10, 10s after the mocked "now" (18:00:00) —
    // advancing 10s should fire the self-scheduled timer and trigger a
    // second tune-in event.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(watchCallCount).toBe(2);
    expect(result.current.state).toMatchObject({ status: "playing", mediaItemId: 6 });
  });

  it("shows off_air with the next program and re-tunes in when it starts", async () => {
    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", () => {
        watchCallCount += 1;
        return watchCallCount === 1
          ? HttpResponse.json({ status: "off_air" })
          : HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 7, offset_sec: 0 });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: null,
          offset_sec: 0,
          next: { program_id: 3, media_item_id: 7, start_time: "2026-01-01T18:00:20Z", end_time: "2026-01-01T19:00:00Z" },
        })
      )
    );

    const { result } = renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.state).toEqual({
      status: "off_air",
      next: { mediaItemId: 7, startTime: new Date("2026-01-01T18:00:20Z") },
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(20_000);
    });
    expect(watchCallCount).toBe(2);
    expect(result.current.state).toMatchObject({ status: "playing", mediaItemId: 7 });
  });

  it("does not schedule a timer, and never re-fetches, when off_air has no next program", async () => {
    let watchCallCount = 0;
    server.use(
      http.post("/api/channels/1/watch", () => {
        watchCallCount += 1;
        return HttpResponse.json({ status: "off_air" });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      )
    );

    renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(watchCallCount).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10 * 60 * 1000); // 10 minutes — nothing should fire
    });
    expect(watchCallCount).toBe(1);
  });

  it("reports an error status when the watch request fails, and retune() retries it", async () => {
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

    const { result } = renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.state).toEqual({ status: "error" });

    act(() => {
      result.current.retune();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(watchCallCount).toBe(2);
    expect(result.current.state).toEqual({ status: "off_air", next: null });
  });

  it("ignores a stale in-flight response after the channel changes, and tunes in to the new channel", async () => {
    server.use(
      http.post("/api/channels/1/watch", async () => {
        // Deliberately slow, so channel 2's tune-in resolves and updates
        // state first — proves the generation guard discards this once it
        // finally arrives.
        await new Promise((resolve) => setTimeout(resolve, 5000));
        return HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 1, offset_sec: 0 });
      }),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({ channel_id: 1, current: null, offset_sec: 0, next: null })
      ),
      http.post("/api/channels/2/watch", () =>
        HttpResponse.json({ status: "playing", mode: "direct", media_item_id: 2, offset_sec: 0 })
      ),
      http.get("/api/channels/2/now", () =>
        HttpResponse.json({ channel_id: 2, current: null, offset_sec: 0, next: null })
      )
    );

    const { result, rerender } = renderHook(({ channelId }) => useTuneIn(channelId), {
      initialProps: { channelId: 1 },
    });

    rerender({ channelId: 2 });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.state).toMatchObject({ status: "playing", mediaItemId: 2 });

    // Let channel 1's slow response finally arrive; it must not overwrite
    // channel 2's state.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(result.current.state).toMatchObject({ status: "playing", mediaItemId: 2 });
  });

  it("clears its timer on unmount without throwing or updating state", async () => {
    server.use(
      http.post("/api/channels/1/watch", () => HttpResponse.json({ status: "off_air" })),
      http.get("/api/channels/1/now", () =>
        HttpResponse.json({
          channel_id: 1,
          current: null,
          offset_sec: 0,
          next: { program_id: 1, media_item_id: 1, start_time: "2026-01-01T18:05:00Z", end_time: "2026-01-01T19:00:00Z" },
        })
      )
    );

    const { unmount } = renderHook(() => useTuneIn(1));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    unmount();

    // next.start_time (18:05:00) is 5 minutes after mocked "now" — advancing
    // past it must not throw (e.g. a setState-after-unmount issue) now that
    // the component is gone.
    await expect(
      act(async () => {
        await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
      })
    ).resolves.not.toThrow();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/api/playback.test.ts`
Expected: FAIL — build error (`./playback` module doesn't exist yet).

- [ ] **Step 3: Write the implementation**

`web/src/api/playback.ts`:

```ts
import { useCallback, useEffect, useRef, useState } from "react";
import { apiGet, apiPost } from "./http";

export type WatchResponse =
  | { status: "playing"; mode: "direct"; media_item_id: number; offset_sec: number }
  | { status: "playing"; mode: "hls"; media_item_id: number; offset_sec: number; session_id: string }
  | { status: "off_air" }
  | { status: "unavailable" };

export interface ProgramState {
  program_id: number;
  media_item_id: number;
  start_time: string;
  end_time: string;
}

export interface NowResponse {
  channel_id: number;
  current: ProgramState | null;
  offset_sec: number;
  next: ProgramState | null;
}

export function watchChannel(channelId: number): Promise<WatchResponse> {
  return apiPost<WatchResponse>(`/channels/${channelId}/watch`);
}

export function getChannelNow(channelId: number): Promise<NowResponse> {
  return apiGet<NowResponse>(`/channels/${channelId}/now`);
}

interface NextProgram {
  mediaItemId: number;
  startTime: Date;
}

export type TuneInState =
  | { status: "loading" }
  | { status: "error" }
  | {
      status: "playing";
      mode: "direct" | "hls";
      mediaItemId: number;
      offsetSec: number;
      sessionId?: string;
      next: NextProgram | null;
    }
  | { status: "off_air" | "unavailable"; next: NextProgram | null };

// useTuneIn owns the tune-in event flow (design spec §3): on mount, on
// channelId change, or when a self-scheduled timer fires, it calls the
// watch and now endpoints together and derives the next state plus at most
// one setTimeout — for when the current program ends (re-checks what's
// current then) or for when the next program starts (re-checks in case it
// becomes playable). There is deliberately no polling interval anywhere
// here: channel state is a pure function of (schedule, wall-clock time),
// recomputed on demand, matching the same principle the backend scheduler
// already follows.
export function useTuneIn(channelId: number): { state: TuneInState; retune: () => void } {
  const [state, setState] = useState<TuneInState>({ status: "loading" });
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Incremented on every tune-in attempt and on unmount/channelId change; a
  // response is only applied if it's still the most recent attempt, so a
  // slow response for a tune-in event the user has already moved past
  // (channel switch, or a newer timer firing) never overwrites newer state
  // or schedules a stale timer.
  const generationRef = useRef(0);

  const tuneIn = useCallback(() => {
    const myGeneration = ++generationRef.current;
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setState({ status: "loading" });

    Promise.all([watchChannel(channelId), getChannelNow(channelId)])
      .then(([watch, now]) => {
        if (myGeneration !== generationRef.current) return;

        const next: NextProgram | null = now.next
          ? { mediaItemId: now.next.media_item_id, startTime: new Date(now.next.start_time) }
          : null;

        if (watch.status === "playing") {
          setState({
            status: "playing",
            mode: watch.mode,
            mediaItemId: watch.media_item_id,
            offsetSec: watch.offset_sec,
            sessionId: watch.mode === "hls" ? watch.session_id : undefined,
            next,
          });
          if (now.current) {
            const remainingMs = new Date(now.current.end_time).getTime() - Date.now();
            timerRef.current = setTimeout(tuneIn, Math.max(remainingMs, 0));
          }
        } else {
          setState({ status: watch.status, next });
          if (next) {
            const untilNextMs = next.startTime.getTime() - Date.now();
            timerRef.current = setTimeout(tuneIn, Math.max(untilNextMs, 0));
          }
        }
      })
      .catch(() => {
        if (myGeneration !== generationRef.current) return;
        setState({ status: "error" });
      });
  }, [channelId]);

  useEffect(() => {
    tuneIn();
    return () => {
      generationRef.current++;
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [tuneIn]);

  return { state, retune: tuneIn };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/api/playback.test.ts`
Expected: PASS (7/7)

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/api/playback.ts web/src/api/playback.test.ts
git commit -m "feat: add useTuneIn hook for event-driven channel tune-in"
```

---

## Task 2: `VideoPlayer` — direct/HLS playback wrapper

**Files:**
- Create: `web/src/components/VideoPlayer.tsx`
- Create: `web/src/components/VideoPlayer.css`
- Test: `web/src/components/VideoPlayer.test.tsx`
- Modify: `web/package.json` (add `hls.js` dependency)

**Interfaces:**
- Consumes: nothing from Task 1 (a self-contained presentational component; `TVScreen`, Task 6, is what wires it to `useTuneIn`'s state).
- Produces: `VideoPlayer({ mode: "direct" | "hls"; src: string; offsetSec?: number; onError: () => void; onTimeUpdate?: (currentTimeSec: number) => void })`. **Task 6's `TVScreen` is the only consumer.**

- [ ] **Step 1: Add the `hls.js` dependency**

```bash
cd /home/daslaptop/HomeStreamProject/web
npm install hls.js
```

- [ ] **Step 2: Write the failing tests**

`web/src/components/VideoPlayer.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { VideoPlayer } from "./VideoPlayer";

const hlsInstances: Array<{
  loadSource: ReturnType<typeof vi.fn>;
  attachMedia: ReturnType<typeof vi.fn>;
  destroy: ReturnType<typeof vi.fn>;
  on: ReturnType<typeof vi.fn>;
}> = [];

vi.mock("hls.js", () => {
  class MockHls {
    static Events = { ERROR: "hlsError" };
    static isSupported = vi.fn(() => true);
    loadSource = vi.fn();
    attachMedia = vi.fn();
    destroy = vi.fn();
    on = vi.fn();
    constructor() {
      hlsInstances.push(this as unknown as (typeof hlsInstances)[number]);
    }
  }
  return { default: MockHls };
});

describe("VideoPlayer", () => {
  beforeEach(() => {
    hlsInstances.length = 0;
    window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    window.HTMLMediaElement.prototype.load = vi.fn();
    window.HTMLMediaElement.prototype.canPlayType = vi.fn().mockReturnValue("");
  });

  it("sets the video src directly and applies offsetSec on loadedmetadata for direct mode", () => {
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" offsetSec={42} onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    expect(video.src).toContain("/api/media/5/stream");

    fireEvent.loadedMetadata(video);
    expect(video.currentTime).toBe(42);
    expect(video.play).toHaveBeenCalled();
  });

  it("constructs hls.js and loads the playlist for hls mode without native support", () => {
    render(<VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" onError={vi.fn()} />);
    expect(hlsInstances).toHaveLength(1);
    expect(hlsInstances[0].loadSource).toHaveBeenCalledWith("/api/playback/sessions/abc/playlist.m3u8");
    expect(hlsInstances[0].attachMedia).toHaveBeenCalled();
  });

  it("never applies offsetSec to the video element in hls mode", () => {
    render(<VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" offsetSec={99} onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    fireEvent.loadedMetadata(video);
    expect(video.currentTime).toBe(0);
  });

  it("calls onError when the video element errors", () => {
    const onError = vi.fn();
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={onError} />);
    fireEvent.error(screen.getByTestId("video-el"));
    expect(onError).toHaveBeenCalled();
  });

  it("reports currentTime via onTimeUpdate as the video plays", () => {
    const onTimeUpdate = vi.fn();
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={vi.fn()} onTimeUpdate={onTimeUpdate} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    video.currentTime = 12.5;
    fireEvent.timeUpdate(video);
    expect(onTimeUpdate).toHaveBeenCalledWith(12.5);
  });

  it("shows a tap-to-play button when autoplay is blocked, and retries play on click", async () => {
    window.HTMLMediaElement.prototype.play = vi.fn().mockRejectedValue(new Error("blocked"));
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    fireEvent.loadedMetadata(video);

    const button = await screen.findByText("▶ Tap to play");
    expect(button).toBeInTheDocument();

    window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    fireEvent.click(button);
    expect(screen.queryByText("▶ Tap to play")).not.toBeInTheDocument();
  });

  it("does not reset the video src or restart hls.js when only the onError/onTimeUpdate callback identities change", () => {
    const { rerender } = render(
      <VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" onError={() => {}} />
    );
    expect(hlsInstances).toHaveLength(1);

    rerender(
      <VideoPlayer
        mode="hls"
        src="/api/playback/sessions/abc/playlist.m3u8"
        onError={() => {}}
        onTimeUpdate={() => {}}
      />
    );
    expect(hlsInstances).toHaveLength(1);
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/components/VideoPlayer.test.tsx`
Expected: FAIL — build error (`./VideoPlayer` module doesn't exist yet).

- [ ] **Step 4: Write the implementation**

`web/src/components/VideoPlayer.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import "./VideoPlayer.css";

interface Props {
  mode: "direct" | "hls";
  src: string;
  offsetSec?: number;
  onError: () => void;
  onTimeUpdate?: (currentTimeSec: number) => void;
}

// Dumb wrapper around <video>: knows nothing about channels, scheduling, or
// tune-in state. mode selects native playback (direct) or hls.js — or
// native HLS on Safari — for hls; offsetSec (direct mode only) is applied
// once metadata loads. Per the playback backend's design (TuneInResult's
// doc comment): in hls mode the offset was already applied server-side via
// ffmpeg's -ss seek, so it must never be applied here too.
export function VideoPlayer({ mode, src, offsetSec, onError, onTimeUpdate }: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [blockedByAutoplay, setBlockedByAutoplay] = useState(false);

  // Keep the latest callbacks in refs so the setup effect below doesn't
  // need them as dependencies — depending on them directly would re-run the
  // whole video/hls.js setup (restarting playback) every time a parent
  // re-render happens to pass a new function identity, which has nothing to
  // do with mode/src/offsetSec actually changing.
  const onErrorRef = useRef(onError);
  const onTimeUpdateRef = useRef(onTimeUpdate);
  useEffect(() => {
    onErrorRef.current = onError;
    onTimeUpdateRef.current = onTimeUpdate;
  });

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    setBlockedByAutoplay(false);

    let hls: Hls | undefined;

    function attemptPlay() {
      video!.play().catch(() => setBlockedByAutoplay(true));
    }

    function handleLoadedMetadata() {
      if (mode === "direct" && offsetSec !== undefined) {
        video!.currentTime = offsetSec;
      }
      attemptPlay();
    }

    function handleError() {
      onErrorRef.current();
    }

    function handleTimeUpdate() {
      onTimeUpdateRef.current?.(video!.currentTime);
    }

    video.addEventListener("loadedmetadata", handleLoadedMetadata);
    video.addEventListener("error", handleError);
    video.addEventListener("timeupdate", handleTimeUpdate);

    if (mode === "hls" && !video.canPlayType("application/vnd.apple.mpegurl")) {
      if (Hls.isSupported()) {
        hls = new Hls();
        hls.on(Hls.Events.ERROR, (_event, data) => {
          if (data.fatal) onErrorRef.current();
        });
        hls.loadSource(src);
        hls.attachMedia(video);
      } else {
        onErrorRef.current();
      }
    } else {
      video.src = src;
    }

    return () => {
      video.removeEventListener("loadedmetadata", handleLoadedMetadata);
      video.removeEventListener("error", handleError);
      video.removeEventListener("timeupdate", handleTimeUpdate);
      hls?.destroy();
    };
  }, [mode, src, offsetSec]);

  return (
    <div className="video-player">
      <video ref={videoRef} className="video-player-el" data-testid="video-el" />
      {blockedByAutoplay && (
        <button
          className="video-player-tap-to-play"
          onClick={() => {
            videoRef.current?.play();
            setBlockedByAutoplay(false);
          }}
        >
          ▶ Tap to play
        </button>
      )}
    </div>
  );
}
```

`web/src/components/VideoPlayer.css`:

```css
.video-player {
  position: relative;
  width: 100%;
  height: 100%;
  background: #000;
}

.video-player-el {
  width: 100%;
  height: 100%;
  display: block;
}

.video-player-tap-to-play {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  padding: 12px 24px;
  font-size: 1.1rem;
  cursor: pointer;
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/components/VideoPlayer.test.tsx`
Expected: PASS (7/7)

- [ ] **Step 6: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/package.json web/package-lock.json web/src/components/VideoPlayer.tsx web/src/components/VideoPlayer.css web/src/components/VideoPlayer.test.tsx
git commit -m "feat: add VideoPlayer (direct playback + hls.js) component"
```

---

## Task 3: Countdown/time formatting + `Interstitial`

**Files:**
- Modify: `web/src/scheduling/time.ts` (add `formatTime`, `formatCountdown`)
- Modify: `web/src/scheduling/time.test.ts` (append tests for both)
- Create: `web/src/components/Interstitial.tsx`
- Create: `web/src/components/Interstitial.css`
- Test: `web/src/components/Interstitial.test.tsx`

**Interfaces:**
- Consumes: nothing from Tasks 1-2.
- Produces: `formatTime(date: Date): string`, `formatCountdown(ms: number): string` (both exported from `web/src/scheduling/time.ts`, alongside the existing `computeEndTime`/`formatTimeRange`/`toDatetimeLocalValue`). `Interstitial({ reason: "off_air" | "unavailable"; next: { title: string; startTime: Date } | null })`. **Task 6's `TVScreen` is the only consumer of `Interstitial`.**

- [ ] **Step 1: Write the failing tests for the time helpers**

Append to `web/src/scheduling/time.test.ts` (add `formatCountdown, formatTime` to the existing import from `"./time"`):

```ts
describe("formatTime", () => {
  it("formats a single time as UTC hour:minute", () => {
    expect(formatTime(new Date("2026-01-01T18:00:00Z"))).toBe("06:00 PM");
  });
});

describe("formatCountdown", () => {
  it("formats a millisecond duration as H:MM:SS", () => {
    expect(formatCountdown(30_000)).toBe("0:00:30");
    expect(formatCountdown(65_000)).toBe("0:01:05");
    expect(formatCountdown(3_661_000)).toBe("1:01:01");
  });

  it("clamps a negative duration to 0:00:00", () => {
    expect(formatCountdown(-5000)).toBe("0:00:00");
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/scheduling/time.test.ts`
Expected: FAIL — `formatTime`/`formatCountdown` are not exported yet.

- [ ] **Step 3: Write the time helper implementations**

Append to `web/src/scheduling/time.ts`:

```ts
// Single-time formatter (unlike formatTimeRange's start–end pair), for "Up
// next: X at HH:MM" style copy.
export function formatTime(date: Date): string {
  return new Intl.DateTimeFormat("en-US", { hour: "2-digit", minute: "2-digit" }).format(date);
}

// Formats a millisecond duration as H:MM:SS (hours always shown, even if 0,
// for a stable countdown width that doesn't reflow as it ticks across an
// hour boundary).
export function formatCountdown(ms: number): string {
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${hours}:${pad(minutes)}:${pad(seconds)}`;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/scheduling/time.test.ts`
Expected: PASS

- [ ] **Step 5: Write the failing tests for `Interstitial`**

`web/src/components/Interstitial.test.tsx`:

```tsx
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Interstitial } from "./Interstitial";

describe("Interstitial", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("shows the off_air heading and a next-up countdown", () => {
    render(
      <Interstitial reason="off_air" next={{ title: "Movie B", startTime: new Date("2026-01-01T18:00:30Z") }} />
    );
    expect(screen.getByText("Nothing scheduled right now")).toBeInTheDocument();
    expect(screen.getByText(/Up next: Movie B/)).toBeInTheDocument();
    expect(screen.getByText(/starts in 0:00:30/)).toBeInTheDocument();
  });

  it("shows the unavailable heading and no-next fallback", () => {
    render(<Interstitial reason="unavailable" next={null} />);
    expect(screen.getByText("Currently unavailable")).toBeInTheDocument();
    expect(screen.getByText("Nothing else scheduled on this channel.")).toBeInTheDocument();
  });

  it("ticks the countdown down every second", () => {
    render(
      <Interstitial reason="off_air" next={{ title: "Movie B", startTime: new Date("2026-01-01T18:00:30Z") }} />
    );
    expect(screen.getByText(/starts in 0:00:30/)).toBeInTheDocument();

    act(() => vi.advanceTimersByTime(5000));
    expect(screen.getByText(/starts in 0:00:25/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/components/Interstitial.test.tsx`
Expected: FAIL — build error (`./Interstitial` module doesn't exist yet).

- [ ] **Step 7: Write the `Interstitial` implementation**

`web/src/components/Interstitial.tsx`:

```tsx
import { useEffect, useState } from "react";
import { formatCountdown, formatTime } from "../scheduling/time";
import "./Interstitial.css";

interface NextProgram {
  title: string;
  startTime: Date;
}

interface Props {
  reason: "off_air" | "unavailable";
  next: NextProgram | null;
}

const HEADINGS: Record<Props["reason"], string> = {
  off_air: "Nothing scheduled right now",
  unavailable: "Currently unavailable",
};

// Shown between programs (off_air) or when a scheduled program's file isn't
// playable (unavailable) — a blank screen with a next-up countdown rather
// than silently jumping ahead, per the design spec. The natural seam for a
// future recap/trailer/ad slot, not built now.
export function Interstitial({ reason, next }: Props) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    if (!next) return;
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, [next]);

  return (
    <div className="interstitial">
      <p className="interstitial-heading">{HEADINGS[reason]}</p>
      {next ? (
        <p className="interstitial-next">
          Up next: {next.title} at {formatTime(next.startTime)} — starts in{" "}
          {formatCountdown(next.startTime.getTime() - now.getTime())}
        </p>
      ) : (
        <p className="interstitial-next">Nothing else scheduled on this channel.</p>
      )}
    </div>
  );
}
```

`web/src/components/Interstitial.css`:

```css
.interstitial {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  background: #000;
  color: #f5f5f5;
  text-align: center;
  gap: 12px;
}

.interstitial-heading {
  font-size: 1.5rem;
  font-weight: 600;
}

.interstitial-next {
  color: #ccc;
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/components/Interstitial.test.tsx src/scheduling/time.test.ts`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/scheduling/time.ts web/src/scheduling/time.test.ts web/src/components/Interstitial.tsx web/src/components/Interstitial.css web/src/components/Interstitial.test.tsx
git commit -m "feat: add off-air/unavailable Interstitial with next-up countdown"
```

---

## Task 4: `NowPlayingOverlay`

**Files:**
- Create: `web/src/components/NowPlayingOverlay.tsx`
- Create: `web/src/components/NowPlayingOverlay.css`
- Test: `web/src/components/NowPlayingOverlay.test.tsx`

**Interfaces:**
- Consumes: nothing from Tasks 1-3.
- Produces: `NowPlayingOverlay({ title: string; currentTimeSec: number; durationSec: number; nextTitle: string | null })`. **Task 6's `TVScreen` is the only consumer.**

- [ ] **Step 1: Write the failing tests**

`web/src/components/NowPlayingOverlay.test.tsx`:

```tsx
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NowPlayingOverlay } from "./NowPlayingOverlay";

describe("NowPlayingOverlay", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows the title, progress, and next-up text", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle="Movie B" />);
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Next: Movie B")).toBeInTheDocument();
  });

  it("omits the next-up line when there is no next program", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    expect(screen.queryByText(/^Next:/)).not.toBeInTheDocument();
  });

  it("is visible on mount and hides after a period of inactivity", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).not.toHaveClass(
      "now-playing-overlay-hidden"
    );

    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).toHaveClass("now-playing-overlay-hidden");
  });

  it("re-shows and resets the hide timer on mouse movement", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).toHaveClass("now-playing-overlay-hidden");

    act(() => {
      fireEvent.mouseMove(window);
    });
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).not.toHaveClass(
      "now-playing-overlay-hidden"
    );
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/components/NowPlayingOverlay.test.tsx`
Expected: FAIL — build error (`./NowPlayingOverlay` module doesn't exist yet).

- [ ] **Step 3: Write the implementation**

`web/src/components/NowPlayingOverlay.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import "./NowPlayingOverlay.css";

interface Props {
  title: string;
  currentTimeSec: number;
  durationSec: number;
  nextTitle: string | null;
}

const HIDE_AFTER_MS = 3000;

// Auto-hiding "now playing" bar: visible on mount/activity, fades out after
// a few seconds of no mouse/touch activity anywhere on the page, so the
// video stays unobstructed during normal viewing. Self-contained (listens
// on window directly) so it doesn't need TVScreen to coordinate pointer
// events with it.
export function NowPlayingOverlay({ title, currentTimeSec, durationSec, nextTitle }: Props) {
  const [visible, setVisible] = useState(true);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    function showThenScheduleHide() {
      setVisible(true);
      if (hideTimer.current !== null) clearTimeout(hideTimer.current);
      hideTimer.current = setTimeout(() => setVisible(false), HIDE_AFTER_MS);
    }
    showThenScheduleHide();
    window.addEventListener("mousemove", showThenScheduleHide);
    window.addEventListener("touchstart", showThenScheduleHide);
    return () => {
      window.removeEventListener("mousemove", showThenScheduleHide);
      window.removeEventListener("touchstart", showThenScheduleHide);
      if (hideTimer.current !== null) clearTimeout(hideTimer.current);
    };
  }, []);

  const progressPercent = durationSec > 0 ? Math.min(100, (currentTimeSec / durationSec) * 100) : 0;

  return (
    <div className={`now-playing-overlay${visible ? "" : " now-playing-overlay-hidden"}`}>
      <p className="now-playing-title">{title}</p>
      <div className="now-playing-progress-track">
        <div className="now-playing-progress-fill" style={{ width: `${progressPercent}%` }} />
      </div>
      {nextTitle && <p className="now-playing-next">Next: {nextTitle}</p>}
    </div>
  );
}
```

`web/src/components/NowPlayingOverlay.css`:

```css
.now-playing-overlay {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 16px 24px;
  background: linear-gradient(to top, rgba(0, 0, 0, 0.85), transparent);
  color: #fff;
  transition: opacity 0.3s ease;
  opacity: 1;
}

.now-playing-overlay-hidden {
  opacity: 0;
  pointer-events: none;
}

.now-playing-title {
  font-size: 1.1rem;
  font-weight: 600;
  margin: 0 0 8px;
}

.now-playing-progress-track {
  height: 4px;
  background: rgba(255, 255, 255, 0.3);
  border-radius: 2px;
  overflow: hidden;
}

.now-playing-progress-fill {
  height: 100%;
  background: #f5f5f5;
}

.now-playing-next {
  margin: 8px 0 0;
  color: #ccc;
  font-size: 0.9rem;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/components/NowPlayingOverlay.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/components/NowPlayingOverlay.tsx web/src/components/NowPlayingOverlay.css web/src/components/NowPlayingOverlay.test.tsx
git commit -m "feat: add auto-hiding NowPlayingOverlay"
```

---

## Task 5: `ChannelSwitcher`

**Files:**
- Create: `web/src/components/ChannelSwitcher.tsx`
- Create: `web/src/components/ChannelSwitcher.css`
- Test: `web/src/components/ChannelSwitcher.test.tsx`

**Interfaces:**
- Consumes: `useChannels` from `web/src/api/channels.ts` (existing, Plan 2).
- Produces: `ChannelSwitcher({ currentChannelId: number; onSelect: (channelId: number) => void })`. **Task 6's `TVScreen` is the only consumer** (wires `onSelect` to `useNavigate()`, keeping this component router-agnostic and independently testable).

- [ ] **Step 1: Write the failing tests**

`web/src/components/ChannelSwitcher.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelSwitcher } from "./ChannelSwitcher";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Comedy", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 3, name: "Off", description: "", enabled: false, position: 2, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderSwitcher(currentChannelId: number, onSelect = vi.fn()) {
  server.use(http.get("/api/channels", () => HttpResponse.json(CHANNELS)));
  const client = createTestQueryClient();
  render(<ChannelSwitcher currentChannelId={currentChannelId} onSelect={onSelect} />, {
    wrapper: wrapWithQueryClient(client),
  });
  return onSelect;
}

describe("ChannelSwitcher", () => {
  it("cycles to the next enabled channel", async () => {
    const onSelect = renderSwitcher(1);
    fireEvent.click(await screen.findByLabelText("Next channel"));
    expect(onSelect).toHaveBeenCalledWith(2);
  });

  it("wraps from the last enabled channel back to the first, skipping disabled ones", async () => {
    const onSelect = renderSwitcher(2);
    fireEvent.click(await screen.findByLabelText("Next channel"));
    expect(onSelect).toHaveBeenCalledWith(1);
  });

  it("cycles to the previous enabled channel", async () => {
    const onSelect = renderSwitcher(2);
    fireEvent.click(await screen.findByLabelText("Previous channel"));
    expect(onSelect).toHaveBeenCalledWith(1);
  });

  it("toggles a list of enabled channels and selects one directly", async () => {
    const onSelect = renderSwitcher(1);
    fireEvent.click(await screen.findByLabelText("Show channel list"));
    expect(await screen.findByText("Comedy")).toBeInTheDocument();
    expect(screen.queryByText("Off")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Comedy"));
    expect(onSelect).toHaveBeenCalledWith(2);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/components/ChannelSwitcher.test.tsx`
Expected: FAIL — build error (`./ChannelSwitcher` module doesn't exist yet).

- [ ] **Step 3: Write the implementation**

`web/src/components/ChannelSwitcher.tsx`:

```tsx
import { useState } from "react";
import { useChannels } from "../api/channels";
import "./ChannelSwitcher.css";

interface Props {
  currentChannelId: number;
  onSelect: (channelId: number) => void;
}

export function ChannelSwitcher({ currentChannelId, onSelect }: Props) {
  const { data: channels } = useChannels();
  const [listOpen, setListOpen] = useState(false);
  const enabled = [...(channels ?? [])].filter((c) => c.enabled).sort((a, b) => a.position - b.position);

  function cycle(direction: -1 | 1) {
    if (enabled.length === 0) return;
    const index = enabled.findIndex((c) => c.id === currentChannelId);
    const nextIndex = ((index === -1 ? 0 : index) + direction + enabled.length) % enabled.length;
    onSelect(enabled[nextIndex].id);
  }

  return (
    <div className="channel-switcher">
      <button aria-label="Previous channel" onClick={() => cycle(-1)}>◀</button>
      <button aria-label="Show channel list" onClick={() => setListOpen((v) => !v)}>☰</button>
      <button aria-label="Next channel" onClick={() => cycle(1)}>▶</button>
      {listOpen && (
        <ul className="channel-switcher-list">
          {enabled.map((channel) => (
            <li key={channel.id}>
              <button
                className={channel.id === currentChannelId ? "channel-switcher-active" : undefined}
                onClick={() => {
                  onSelect(channel.id);
                  setListOpen(false);
                }}
              >
                {channel.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
```

`web/src/components/ChannelSwitcher.css`:

```css
.channel-switcher {
  position: absolute;
  top: 16px;
  right: 16px;
  display: flex;
  gap: 8px;
}

.channel-switcher button {
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  border: none;
  border-radius: 4px;
  padding: 8px 12px;
  cursor: pointer;
}

.channel-switcher-list {
  position: absolute;
  top: 100%;
  right: 0;
  margin-top: 8px;
  background: rgba(0, 0, 0, 0.9);
  border-radius: 4px;
  padding: 8px;
  list-style: none;
  min-width: 160px;
}

.channel-switcher-list button {
  width: 100%;
  text-align: left;
  background: none;
}

.channel-switcher-active {
  font-weight: 600;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/components/ChannelSwitcher.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/components/ChannelSwitcher.tsx web/src/components/ChannelSwitcher.css web/src/components/ChannelSwitcher.test.tsx
git commit -m "feat: add ChannelSwitcher (prev/next + channel list overlay)"
```

---

## Task 6: `TVScreen`, `TVIndexScreen`, routing, and navigation wiring

**Files:**
- Create: `web/src/screens/TVScreen.tsx`
- Create: `web/src/screens/TVScreen.css`
- Create: `web/src/screens/TVIndexScreen.tsx`
- Test: `web/src/screens/TVScreen.test.tsx`
- Test: `web/src/screens/TVIndexScreen.test.tsx`
- Modify: `web/src/AppRoutes.tsx` (add `/tv` and `/tv/:channelId` routes)
- Modify: `web/src/AppRoutes.test.tsx` (append a route-smoke test)
- Modify: `web/src/components/Sidebar.tsx` (add the TV nav item)
- Modify: `web/src/components/Sidebar.test.tsx` (update for five screens)

**Interfaces:**
- Consumes: `useTuneIn` (Task 1), `VideoPlayer` (Task 2), `Interstitial` (Task 3), `NowPlayingOverlay` (Task 4), `ChannelSwitcher` (Task 5), `useChannels` (`web/src/api/channels.ts`, existing), `useMediaItems` (`web/src/api/media.ts`, existing).
- Produces: the `/tv` and `/tv/:channelId` routes — the plan's final product-facing deliverable, no further in-plan consumer.

- [ ] **Step 1: Write the failing tests for `TVScreen`**

`web/src/screens/TVScreen.test.tsx`:

```tsx
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
```

- [ ] **Step 2: Write the failing tests for `TVIndexScreen`**

`web/src/screens/TVIndexScreen.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { TVIndexScreen } from "./TVIndexScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Comedy", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

// Reads the matched route param from React Router's in-memory location
// (not window.location, which MemoryRouter never touches) so the test can
// assert which channel TVIndexScreen actually redirected to.
function LandedOnChannel() {
  const { channelId } = useParams<{ channelId: string }>();
  return <p>Landed on channel {channelId}</p>;
}

function renderIndex(channels: typeof CHANNELS) {
  server.use(http.get("/api/channels", () => HttpResponse.json(channels)));
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={["/tv"]}>
      <Routes>
        <Route path="/tv" element={<TVIndexScreen />} />
        <Route path="/tv/:channelId" element={<LandedOnChannel />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("TVIndexScreen", () => {
  beforeEach(() => localStorage.clear());

  it("redirects to the first enabled channel when there's no last-watched channel", async () => {
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 1")).toBeInTheDocument();
  });

  it("redirects to the last-watched channel when it's still enabled", async () => {
    localStorage.setItem("personaltv.tv.lastChannelId", "2");
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 2")).toBeInTheDocument();
  });

  it("falls back to the first enabled channel when the last-watched one is gone or disabled", async () => {
    localStorage.setItem("personaltv.tv.lastChannelId", "999");
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 1")).toBeInTheDocument();
  });

  it("shows an empty state instead of redirecting when there are no enabled channels", async () => {
    renderIndex([]);
    expect(await screen.findByText(/No channels yet/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/screens/TVScreen.test.tsx src/screens/TVIndexScreen.test.tsx`
Expected: FAIL — build error (`./TVScreen`/`./TVIndexScreen` modules don't exist yet).

- [ ] **Step 4: Write the `TVScreen` implementation**

`web/src/screens/TVScreen.tsx`:

```tsx
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMediaItems } from "../api/media";
import { useTuneIn } from "../api/playback";
import { ChannelSwitcher } from "../components/ChannelSwitcher";
import { Interstitial } from "../components/Interstitial";
import { NowPlayingOverlay } from "../components/NowPlayingOverlay";
import { VideoPlayer } from "../components/VideoPlayer";
import "./TVScreen.css";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";

export function TVScreen() {
  const params = useParams<{ channelId: string }>();
  const channelId = Number(params.channelId);
  const navigate = useNavigate();
  const { data: media } = useMediaItems();
  const mediaById = useMemo(() => new Map((media ?? []).map((m) => [m.id, m])), [media]);
  const { state, retune } = useTuneIn(channelId);

  const [videoError, setVideoError] = useState(false);
  const [rawCurrentTime, setRawCurrentTime] = useState(0);

  // A fresh tune-in event (new state object) means a fresh attempt: clear
  // any stale video error from the previous attempt and reset the
  // progress-bar clock so it doesn't show the old program's elapsed time.
  useEffect(() => {
    setVideoError(false);
    setRawCurrentTime(0);
  }, [state]);

  useEffect(() => {
    if (state.status === "playing") {
      localStorage.setItem(LAST_CHANNEL_KEY, String(channelId));
    }
  }, [state, channelId]);

  if (state.status === "loading") return <p>Tuning in…</p>;

  if (state.status === "error" || videoError) {
    return (
      <div className="tv-screen tv-screen-error">
        <p role="alert">Something went wrong tuning in.</p>
        <button onClick={retune}>Retry</button>
      </div>
    );
  }

  const nextTitle = state.next ? mediaById.get(state.next.mediaItemId)?.title ?? null : null;

  if (state.status === "off_air" || state.status === "unavailable") {
    const next = state.next ? { title: nextTitle ?? "Unknown", startTime: state.next.startTime } : null;
    return (
      <div className="tv-screen">
        <Interstitial reason={state.status} next={next} />
        <ChannelSwitcher currentChannelId={channelId} onSelect={(id) => navigate(`/tv/${id}`)} />
      </div>
    );
  }

  // state.status === "playing"
  const item = mediaById.get(state.mediaItemId);
  const src =
    state.mode === "direct"
      ? `/api/media/${state.mediaItemId}/stream`
      : `/api/playback/sessions/${state.sessionId}/playlist.m3u8`;
  // In hls mode the video element's own currentTime starts near 0 (the
  // session's own timeline, already seeked server-side) — add the tune-in
  // offset back on top for display. In direct mode currentTime is already
  // real (VideoPlayer sets it to offsetSec on load), so it needs no
  // adjustment.
  const displayedTimeSec = state.mode === "hls" ? state.offsetSec + rawCurrentTime : rawCurrentTime;

  return (
    <div className="tv-screen">
      <VideoPlayer
        mode={state.mode}
        src={src}
        offsetSec={state.mode === "direct" ? state.offsetSec : undefined}
        onError={() => setVideoError(true)}
        onTimeUpdate={setRawCurrentTime}
      />
      <NowPlayingOverlay
        title={item?.title ?? "Unknown"}
        currentTimeSec={displayedTimeSec}
        durationSec={item?.duration_sec ?? 0}
        nextTitle={nextTitle}
      />
      <ChannelSwitcher currentChannelId={channelId} onSelect={(id) => navigate(`/tv/${id}`)} />
    </div>
  );
}
```

`web/src/screens/TVScreen.css`:

```css
.tv-screen {
  position: relative;
  width: 100%;
  height: calc(100vh - 48px);
  background: #000;
}

.tv-screen-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #f5f5f5;
}
```

- [ ] **Step 5: Write the `TVIndexScreen` implementation**

`web/src/screens/TVIndexScreen.tsx`:

```tsx
import { Link, Navigate } from "react-router-dom";
import { useChannels } from "../api/channels";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";

// Resolves "/tv" to a concrete "/tv/:channelId": the last-watched channel
// (persisted by TVScreen) if it's still enabled, otherwise the first
// enabled channel by position. If there are no enabled channels, shows an
// empty state instead of redirecting anywhere.
export function TVIndexScreen() {
  const { data: channels, isLoading } = useChannels();

  if (isLoading) return <p>Loading…</p>;

  const enabled = [...(channels ?? [])].filter((c) => c.enabled).sort((a, b) => a.position - b.position);
  if (enabled.length === 0) {
    return (
      <section>
        <h1>TV</h1>
        <p>
          No channels yet — <Link to="/channels">go create one</Link>.
        </p>
      </section>
    );
  }

  const lastId = Number(localStorage.getItem(LAST_CHANNEL_KEY));
  const target = enabled.find((c) => c.id === lastId) ?? enabled[0];
  return <Navigate to={`/tv/${target.id}`} replace />;
}
```

- [ ] **Step 6: Wire the routes and Sidebar nav item**

In `web/src/AppRoutes.tsx`, add the imports and the two new routes (route order doesn't matter for correctness here, but group them at the top to mirror the PRD's screen ordering):

```tsx
import { Navigate, Route, Routes } from "react-router-dom";
import { ChannelScheduleScreen } from "./screens/ChannelScheduleScreen";
import { ChannelsListScreen } from "./screens/ChannelsListScreen";
import { GuideScreen } from "./screens/GuideScreen";
import { LibraryScreen } from "./screens/LibraryScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { TVIndexScreen } from "./screens/TVIndexScreen";
import { TVScreen } from "./screens/TVScreen";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/guide" replace />} />
      <Route path="/tv" element={<TVIndexScreen />} />
      <Route path="/tv/:channelId" element={<TVScreen />} />
      <Route path="/guide" element={<GuideScreen />} />
      <Route path="/library" element={<LibraryScreen />} />
      <Route path="/channels" element={<ChannelsListScreen />} />
      <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      <Route path="/settings" element={<SettingsScreen />} />
    </Routes>
  );
}
```

In `web/src/components/Sidebar.tsx`, add the TV nav item (first, matching the PRD's documented screen order):

```tsx
const NAV_ITEMS = [
  { to: "/tv", label: "TV" },
  { to: "/guide", label: "Guide" },
  { to: "/library", label: "Library" },
  { to: "/channels", label: "Channels" },
  { to: "/settings", label: "Settings" },
] as const;
```

- [ ] **Step 7: Update `Sidebar.test.tsx` and add a route smoke test to `AppRoutes.test.tsx`**

In `web/src/components/Sidebar.test.tsx`, update the first test's list and description:

```tsx
  it("renders a link for each of the five screens", () => {
    render(
      <MemoryRouter initialEntries={["/guide"]}>
        <Sidebar />
      </MemoryRouter>
    );
    for (const label of ["TV", "Guide", "Library", "Channels", "Settings"]) {
      expect(screen.getByRole("link", { name: label })).toBeInTheDocument();
    }
  });
```

Append to `web/src/AppRoutes.test.tsx`:

```tsx
  it("renders the TV screen's empty state at /tv when there are no channels", async () => {
    server.use(http.get("/api/channels", () => HttpResponse.json([])));
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <MemoryRouter initialEntries={["/tv"]}>
          <AppRoutes />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(await screen.findByRole("heading", { name: "TV" })).toBeInTheDocument();
  });
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/screens/TVScreen.test.tsx src/screens/TVIndexScreen.test.tsx src/components/Sidebar.test.tsx src/AppRoutes.test.tsx`
Expected: PASS

- [ ] **Step 9: Run the full frontend verification suite**

```bash
cd /home/daslaptop/HomeStreamProject/web
npx tsc -b
npm run lint
npm test
npm run build
```

Expected: all pass/exit 0.

- [ ] **Step 10: Manual sanity check**

Not automatable in this plan, but worth doing once, mirroring how Plan 3's playback backend was manually curl-verified: `cd /home/daslaptop/HomeStreamProject && go build ./... && go run ./cmd/personaltv`, then in a browser, create a source/channel/program (or reuse existing ones), navigate to `/tv`, confirm it redirects to a channel and plays (or shows off-air/unavailable correctly), and confirm channel switching and the tap-to-play fallback (if autoplay is blocked) work.

- [ ] **Step 11: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/TVScreen.tsx web/src/screens/TVScreen.css web/src/screens/TVIndexScreen.tsx web/src/screens/TVScreen.test.tsx web/src/screens/TVIndexScreen.test.tsx web/src/AppRoutes.tsx web/src/AppRoutes.test.tsx web/src/components/Sidebar.tsx web/src/components/Sidebar.test.tsx
git commit -m "feat: add TV/player screen, routing, and navigation wiring"
```

---

## Definition of Done

`cd web && npx tsc -b && npm run lint && npm test && npm run build` all pass/exit 0 from a clean state. `cd /home/daslaptop/HomeStreamProject && go build ./... && go run ./cmd/personaltv` serves a working TV screen at `/tv`: it resolves to a channel, plays direct or HLS content correctly (offset applied only in direct mode), shows the off-air/unavailable interstitial with a live next-up countdown when nothing playable is on, auto-advances via its self-scheduled timers with no polling loop, supports prev/next channel switching and a channel-list overlay, and the now-playing overlay auto-hides after inactivity. No backend changes (`go test ./... -race` from before this plan stays unaffected, since no Go file changes).
