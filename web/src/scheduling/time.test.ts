import { describe, expect, it } from "vitest";
import { computeEndTime, formatTimeRange, toDatetimeLocalValue } from "./time";

describe("computeEndTime", () => {
  it("adds the media duration (in seconds) to the start time", () => {
    const end = computeEndTime("2026-01-01T18:00:00Z", 5400); // 1.5 hours
    expect(end.toISOString()).toBe("2026-01-01T19:30:00.000Z");
  });

  it("returns the start time unchanged when duration is zero", () => {
    const end = computeEndTime("2026-01-01T18:00:00Z", 0);
    expect(end.toISOString()).toBe("2026-01-01T18:00:00.000Z");
  });
});

describe("formatTimeRange", () => {
  it("formats a start/end pair as UTC hour:minute", () => {
    const start = new Date("2026-01-01T18:00:00Z");
    const end = new Date("2026-01-01T19:30:00Z");
    expect(formatTimeRange(start, end)).toBe("06:00 PM – 07:30 PM");
  });
});

describe("toDatetimeLocalValue", () => {
  it("round-trips an ISO string to a datetime-local input value in local time", () => {
    // toDatetimeLocalValue feeds an <input type="datetime-local">, which is
    // always local-time by spec, so the expected value is reconstructed
    // from the same Date object's local getters rather than a fixed string
    // — that stays correct regardless of the test machine's timezone.
    const iso = "2026-01-01T18:00:00Z";
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, "0");
    const expected = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    expect(toDatetimeLocalValue(iso)).toBe(expected);
  });
});
