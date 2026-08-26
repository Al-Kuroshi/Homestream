import type { ResolvedSlot, Slot } from "../api/types";

export const MS_PER_DAY = 24 * 60 * 60 * 1000;

// Sunday 00:00 UTC on or before `date` — day_of_week 0 = Sunday throughout
// this codebase (see the implementation plan's Global Constraints),
// matching Date.getUTCDay() directly.
export function startOfWeekUTC(date: Date): Date {
  const midnight = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
  midnight.setUTCDate(midnight.getUTCDate() - midnight.getUTCDay());
  return midnight;
}

export function weekDates(weekStart: Date): Date[] {
  return Array.from({ length: 7 }, (_, i) => new Date(weekStart.getTime() + i * MS_PER_DAY));
}

export function addWeeks(date: Date, weeks: number): Date {
  return new Date(date.getTime() + weeks * 7 * MS_PER_DAY);
}

// Sparse-integer position for a new slot inserted at insertBeforeIndex
// among existingPositions (already known to be sorted the same way the
// caller will render them). Mirrors the backend's own sparse-position
// convention (increments of 1000) so most inserts never need to renumber
// sibling slots.
// The result is ALWAYS an integer. The API's `position` is a Go `*int`, so a
// fractional value (which repeated midpoint inserts at the same boundary
// produce: 1500 -> 1250 -> 1125 -> 1062.5) fails JSON decoding server-side
// and surfaces the raw Go decoder error to the user in the mutation-error
// banner.
export function positionForInsert(existingPositions: number[], insertBeforeIndex: number): number {
  const sorted = [...existingPositions].sort((a, b) => a - b);
  if (sorted.length === 0) return 1000;
  if (insertBeforeIndex <= 0) return Math.round(sorted[0]) - 1000;
  if (insertBeforeIndex >= sorted.length) return Math.round(sorted[sorted.length - 1]) + 1000;

  const before = sorted[insertBeforeIndex - 1];
  const after = sorted[insertBeforeIndex];
  const midpoint = Math.round((before + after) / 2);
  if (midpoint > before && midpoint < after) return midpoint;

  // Rounding collapsed onto a neighbour, which can only happen when the two
  // neighbours are adjacent integers (or duplicates) — there is simply no
  // integer strictly between them to nudge to. Positions start 1000 apart,
  // so reaching this needs ~10 inserts at the exact same boundary. Tying
  // with the following slot's position is harmless rather than a failure:
  // the backend only sorts by position, so the slot lands next to where it
  // was dropped and the user can re-drag it. A full renumbering pass to
  // reopen space is deliberately out of scope.
  return Math.round(after);
}

export interface DaySlotBlock {
  slot: Slot;
  resolved: ResolvedSlot;
}

// Picks the resolved occurrences whose start falls on the UTC calendar date
// of `date`, joins each back to its originating Slot (for kind/gap_label/
// recurring, which ResolvedSlot alone doesn't carry), sorted by start time.
export function slotsForDate(resolved: ResolvedSlot[], slotsById: Map<number, Slot>, date: Date): DaySlotBlock[] {
  const dayStart = Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate());
  const dayEnd = dayStart + MS_PER_DAY;
  return resolved
    .filter((r) => {
      const t = new Date(r.start_time).getTime();
      return t >= dayStart && t < dayEnd;
    })
    .map((r) => ({ slot: slotsById.get(r.program_id), resolved: r }))
    .filter((b): b is DaySlotBlock => b.slot !== undefined)
    .sort((a, b) => new Date(a.resolved.start_time).getTime() - new Date(b.resolved.start_time).getTime());
}
