// computeEndTime is the one place "end time = start time + media duration"
// (the PRD's end-time-from-duration rule, already implemented server-side
// in internal/scheduler's ScheduledProgram.EndTime()) is computed
// client-side. Every screen that needs a program's end time imports this
// instead of recomputing it.
export function computeEndTime(startTimeIso: string, durationSec: number): Date {
  return new Date(new Date(startTimeIso).getTime() + durationSec * 1000);
}

// Deliberately UTC: the MVP has no per-user timezone setting anywhere in
// the backend or this plan, and formatting in the viewer's local timezone
// here would make this function's output timezone-dependent and untestable
// without mocking the system clock. Displaying local time is a reasonable
// future enhancement, not built in this plan.
export function formatTimeRange(start: Date, end: Date): string {
  const fmt = new Intl.DateTimeFormat("en-US", { hour: "2-digit", minute: "2-digit", timeZone: "UTC" });
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
