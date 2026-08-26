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
export function positionForInsert(existingPositions: number[], insertBeforeIndex: number): number {
  const sorted = [...existingPositions].sort((a, b) => a - b);
  if (sorted.length === 0) return 1000;
  if (insertBeforeIndex <= 0) return sorted[0] - 1000;
  if (insertBeforeIndex >= sorted.length) return sorted[sorted.length - 1] + 1000;
  return (sorted[insertBeforeIndex - 1] + sorted[insertBeforeIndex]) / 2;
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
