import { describe, expect, it } from "vitest";
import type { MediaItem, ResolvedSlot } from "../api/types";
import { buildTimeline, defaultGuideWindow, joinResolvedSlotsWithMedia } from "./guide";

const WINDOW_START = new Date("2026-01-01T17:00:00Z");
const WINDOW_END = new Date("2026-01-01T23:00:00Z");

function guideProgram(id: number, startIso: string, endIso: string) {
  return { programId: id, mediaItemId: id, title: `Program ${id}`, start: new Date(startIso), end: new Date(endIso) };
}

describe("buildTimeline", () => {
  it("returns a single off-air block spanning the window when there are no programs", () => {
    expect(buildTimeline([], WINDOW_START, WINDOW_END)).toEqual([
      { type: "off-air", start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("returns just the program block when it exactly fills the window", () => {
    const p = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T23:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("adds an off-air block before a program that starts after the window start", () => {
    const p = guideProgram(1, "2026-01-01T18:00:00Z", "2026-01-01T19:00:00Z");
    const blocks = buildTimeline([p], WINDOW_START, WINDOW_END);
    expect(blocks[0]).toEqual({ type: "off-air", start: WINDOW_START, end: p.start });
    expect(blocks[1]).toEqual({ type: "program", program: p, start: p.start, end: p.end });
  });

  it("adds an off-air block after a program that ends before the window end", () => {
    const p = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T18:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: p.end },
      { type: "off-air", start: p.end, end: WINDOW_END },
    ]);
  });

  it("adds an off-air block between two non-contiguous programs", () => {
    const a = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T18:00:00Z");
    const b = guideProgram(2, "2026-01-01T19:00:00Z", "2026-01-01T20:00:00Z");
    const blocks = buildTimeline([a, b], WINDOW_START, WINDOW_END);
    expect(blocks[0]).toMatchObject({ type: "program", program: a });
    expect(blocks[1]).toEqual({ type: "off-air", start: a.end, end: b.start });
    expect(blocks[2]).toMatchObject({ type: "program", program: b });
  });

  it("clips a program that runs across both window edges to the window bounds", () => {
    const p = guideProgram(1, "2026-01-01T10:00:00Z", "2026-01-02T00:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("does not render a second overlapping block for a program nested entirely inside a prior one", () => {
    const a = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T21:00:00Z");
    const b = guideProgram(2, "2026-01-01T18:00:00Z", "2026-01-01T19:00:00Z");
    const blocks = buildTimeline([a, b], WINDOW_START, WINDOW_END);
    // Exactly one program block (for a, spanning its full clipped range),
    // no block at all for b (fully subsumed by a), and no off-air gap
    // inserted between them.
    expect(blocks).toEqual([
      { type: "program", program: a, start: WINDOW_START, end: a.end },
      { type: "off-air", start: a.end, end: WINDOW_END },
    ]);
  });

  it("excludes programs entirely outside the window", () => {
    const before = guideProgram(1, "2026-01-01T10:00:00Z", "2026-01-01T11:00:00Z");
    const inWindow = guideProgram(2, "2026-01-01T18:00:00Z", "2026-01-01T19:00:00Z");
    const blocks = buildTimeline([before, inWindow], WINDOW_START, WINDOW_END);
    expect(blocks.some((b) => b.type === "program" && b.program.programId === 1)).toBe(false);
  });
});

describe("joinResolvedSlotsWithMedia", () => {
  const mediaItem: MediaItem = {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  };

  it("joins each resolved slot with its media title, sorted by start time", () => {
    const media = new Map([[1, mediaItem]]);
    const resolved: ResolvedSlot[] = [
      { program_id: 2, media_item_id: 1, start_time: "2026-01-01T20:00:00Z", end_time: "2026-01-01T21:00:00Z" },
      { program_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z", end_time: "2026-01-01T19:00:00Z" },
    ];
    const joined = joinResolvedSlotsWithMedia(resolved, media);
    expect(joined.map((p) => p.programId)).toEqual([1, 2]);
    expect(joined[0]).toMatchObject({
      title: "Movie A",
      start: new Date("2026-01-01T18:00:00Z"),
      end: new Date("2026-01-01T19:00:00Z"),
    });
  });

  it("falls back to a placeholder title when the media item is missing", () => {
    const resolved: ResolvedSlot[] = [
      { program_id: 1, media_item_id: 99, start_time: "2026-01-01T18:00:00Z", end_time: "2026-01-01T19:00:00Z" },
    ];
    const joined = joinResolvedSlotsWithMedia(resolved, new Map());
    expect(joined[0].title).toBe("Media #99");
  });
});

describe("defaultGuideWindow", () => {
  it("returns 1 hour before now to 5 hours after now", () => {
    const now = new Date("2026-01-01T18:00:00Z");
    expect(defaultGuideWindow(now)).toEqual({
      start: new Date("2026-01-01T17:00:00Z"),
      end: new Date("2026-01-01T23:00:00Z"),
    });
  });
});
