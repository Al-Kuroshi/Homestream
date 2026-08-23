// computeEndTime is the one place "end time = start time + media duration"
// (the PRD's end-time-from-duration rule, already implemented server-side
// in internal/scheduler's ScheduledProgram.EndTime()) is computed
// client-side. Every screen that needs a program's end time imports this
// instead of recomputing it.
export function computeEndTime(startTimeIso: string, durationSec: number): Date {
  return new Date(new Date(startTimeIso).getTime() + durationSec * 1000);
}

// Formats in the browser's local timezone — matching toDatetimeLocalValue
// below, since <input type="datetime-local"> is always local time per the
// HTML spec. Start times are entered in local time (via that input), so
// they must be displayed in local time too; hardcoding UTC here would
// silently show users a different time than the one they typed. The test
// suite pins TZ=UTC (see web/src/test/setup.ts) so this stays deterministic
// without mocking the system clock.
export function formatTimeRange(start: Date, end: Date): string {
  const fmt = new Intl.DateTimeFormat("en-US", { hour: "2-digit", minute: "2-digit" });
  return `${fmt.format(start)} – ${fmt.format(end)}`;
}

// Unlike formatTimeRange, this feeds an <input type="datetime-local">,
// which the HTML spec defines as always local time — so this one
// intentionally uses the browser's local timezone via the Date object's
// local getters.
export function toDatetimeLocalValue(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
