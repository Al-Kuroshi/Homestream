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
