import { describe, expect, it } from "vitest";
import type { ResolvedSlot, Slot } from "../api/types";
import { addWeeks, positionForInsert, slotsForDate, startOfWeekUTC, weekDates } from "./week";

describe("startOfWeekUTC", () => {
  it("returns the Sunday 00:00 UTC on or before the given date", () => {
    // 2026-09-02 is a Wednesday.
    const got = startOfWeekUTC(new Date("2026-09-02T15:30:00Z"));
    expect(got.toISOString()).toBe("2026-08-30T00:00:00.000Z");
  });

  it("returns the date itself when it's already a Sunday at midnight", () => {
    const got = startOfWeekUTC(new Date("2026-08-30T00:00:00Z"));
    expect(got.toISOString()).toBe("2026-08-30T00:00:00.000Z");
  });
});

describe("weekDates", () => {
  it("returns 7 consecutive UTC midnights starting at weekStart", () => {
    const weekStart = new Date("2026-08-30T00:00:00Z");
    const dates = weekDates(weekStart);
    expect(dates).toHaveLength(7);
    expect(dates[0].toISOString()).toBe("2026-08-30T00:00:00.000Z");
    expect(dates[6].toISOString()).toBe("2026-09-05T00:00:00.000Z");
  });
});

describe("addWeeks", () => {
  it("shifts a date forward by the given number of weeks", () => {
    const got = addWeeks(new Date("2026-08-30T00:00:00Z"), 1);
    expect(got.toISOString()).toBe("2026-09-06T00:00:00.000Z");
  });

  it("shifts a date backward for a negative count", () => {
    const got = addWeeks(new Date("2026-08-30T00:00:00Z"), -1);
    expect(got.toISOString()).toBe("2026-08-23T00:00:00.000Z");
  });
});

describe("positionForInsert", () => {
  it("returns 1000 when the day is empty", () => {
    expect(positionForInsert([], 0)).toBe(1000);
  });

  it("returns 1000 less than the first slot when inserting at the start", () => {
    expect(positionForInsert([2000, 3000], 0)).toBe(1000);
  });

  it("returns 1000 more than the last slot when inserting at the end", () => {
    expect(positionForInsert([1000, 2000], 2)).toBe(3000);
  });

  it("returns the midpoint when inserting between two slots", () => {
    expect(positionForInsert([1000, 3000], 1)).toBe(2000);
  });
});

describe("slotsForDate", () => {
  const slot: Slot = {
    id: 1, channel_id: 1, kind: "media", media_item_id: 5, gap_duration_sec: null, gap_label: "",
    recurring: true, day_of_week: 1, position: 1000, start_time: null,
    created_at: "", updated_at: "",
  };
  const resolved: ResolvedSlot[] = [
    { program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" },
    { program_id: 2, media_item_id: 5, start_time: "2026-09-01T00:00:00Z", end_time: "2026-09-01T01:00:00Z" },
  ];

  it("returns only the occurrences whose start falls on the given UTC date, sorted by start time", () => {
    const slotsById = new Map([[1, slot]]);
    const got = slotsForDate(resolved, slotsById, new Date("2026-08-31T00:00:00Z"));
    expect(got).toHaveLength(1);
    expect(got[0].resolved.program_id).toBe(1);
    expect(got[0].slot).toBe(slot);
  });

  it("omits an occurrence whose originating slot isn't in slotsById", () => {
    const got = slotsForDate(resolved, new Map(), new Date("2026-08-31T00:00:00Z"));
    expect(got).toHaveLength(0);
  });
});
