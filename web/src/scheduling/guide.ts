import type { MediaItem, Program } from "../api/types";
import { computeEndTime } from "./time";

export interface GuideProgram {
  programId: number;
  mediaItemId: number;
  title: string;
  start: Date;
  end: Date;
}

export type TimelineBlock =
  | { type: "program"; program: GuideProgram; start: Date; end: Date }
  | { type: "off-air"; start: Date; end: Date };

export function joinProgramsWithMedia(programs: Program[], mediaById: Map<number, MediaItem>): GuideProgram[] {
  return programs
    .map((p) => {
      const item = mediaById.get(p.media_item_id);
      return {
        programId: p.id,
        mediaItemId: p.media_item_id,
        title: item?.title ?? `Media #${p.media_item_id}`,
        start: new Date(p.start_time),
        end: computeEndTime(p.start_time, item?.duration_sec ?? 0),
      };
    })
    .sort((a, b) => a.start.getTime() - b.start.getTime());
}

// buildTimeline turns a channel's programs (already sorted by start time —
// see joinProgramsWithMedia) into a contiguous sequence of blocks spanning
// [windowStart, windowEnd) with no gaps: every moment in the window is
// covered by exactly one program block or one off-air block. Off-air is a
// first-class state (spec §4.1), mirroring the backend scheduler's
// CurrentState.Current == nil (internal/scheduler/scheduler.go).
export function buildTimeline(programs: GuideProgram[], windowStart: Date, windowEnd: Date): TimelineBlock[] {
  const blocks: TimelineBlock[] = [];
  const relevant = programs.filter((p) => p.end > windowStart && p.start < windowEnd);

  let cursor = windowStart;
  for (const program of relevant) {
    if (program.start > cursor) {
      blocks.push({ type: "off-air", start: cursor, end: program.start });
    }
    // Clip the rendered start to windowStart AND cursor: an overlapping
    // program nested inside one already rendered (e.g. a prior, longer
    // program's block already covers this span) must not produce a second,
    // overlapping block. If clipping collapses start >= end, the program is
    // fully subsumed and contributes no block at all.
    const start = new Date(Math.max(program.start.getTime(), windowStart.getTime(), cursor.getTime()));
    const end = program.end > windowEnd ? windowEnd : program.end;
    if (start < end) {
      blocks.push({ type: "program", program, start, end });
    }
    if (program.end > cursor) cursor = program.end;
  }
  if (cursor < windowEnd) {
    blocks.push({ type: "off-air", start: cursor, end: windowEnd });
  }
  return blocks;
}

export function defaultGuideWindow(now: Date): { start: Date; end: Date } {
  return {
    start: new Date(now.getTime() - 60 * 60 * 1000),
    end: new Date(now.getTime() + 5 * 60 * 60 * 1000),
  };
}
