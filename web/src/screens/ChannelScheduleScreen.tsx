import { useState } from "react";
import { useParams } from "react-router-dom";
import { useChannel } from "../api/channels";
import { useMediaItems } from "../api/media";
import { useResolvedSlots, useSlotsForChannel } from "../api/slots";
import type { MediaItem } from "../api/types";
import { addWeeks, slotsForDate, startOfWeekUTC, weekDates } from "../scheduling/week";
import "./ChannelScheduleScreen.css";

const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MS_PER_HOUR = 60 * 60 * 1000;

// No cover-art metadata exists in the backend yet (deferred future work,
// per the design spec) — always undefined, so every slot block below falls
// back to its text title/label. Isolated here so wiring in a real field
// later is a one-line change, not a render-logic rewrite.
function mediaPosterUrl(_item: MediaItem | undefined): string | undefined {
  return undefined;
}

// Drops the leading 3-letter weekday abbreviation from toDateString() (e.g.
// "Sun Aug 23 2026" -> "Aug 23 2026"). Without this, the week-nav range label
// always literally contains "Sun" and "Sat" (the first/last day of any
// Sunday-start week), which collides with the day-column headers' own
// "Sun"/"Sat" text under a substring text query.
function formatWeekBoundary(date: Date): string {
  return date.toDateString().slice(4);
}

export function ChannelScheduleScreen() {
  const params = useParams<{ id: string }>();
  const channelId = Number(params.id);
  const { data: channel, isLoading: channelLoading, isError: channelError } = useChannel(channelId);
  const { data: media } = useMediaItems();
  const { data: slots } = useSlotsForChannel(channelId);

  const [weekAnchor] = useState(() => startOfWeekUTC(new Date()));
  const [weekOffset, setWeekOffset] = useState(0);
  const weekStart = addWeeks(weekAnchor, weekOffset);
  const weekEnd = addWeeks(weekStart, 1);
  const { data: resolved, isLoading: resolvedLoading, isError: resolvedError } = useResolvedSlots(
    channelId,
    weekStart.toISOString(),
    weekEnd.toISOString()
  );

  const mediaById = new Map((media ?? []).map((m) => [m.id, m]));
  const slotsById = new Map((slots ?? []).map((s) => [s.id, s]));
  const days = weekDates(weekStart);

  if (channelLoading || resolvedLoading) return <p>Loading schedule…</p>;
  if (channelError || resolvedError || !channel) return <p role="alert">Failed to load this channel's schedule.</p>;

  return (
    <section>
      <h1>{channel.name}</h1>
      <div className="week-nav">
        <button onClick={() => setWeekOffset((o) => o - 1)}>&lsaquo; Previous week</button>
        <span>{formatWeekBoundary(weekStart)} – {formatWeekBoundary(new Date(weekEnd.getTime() - 1))}</span>
        <button onClick={() => setWeekOffset((o) => o + 1)}>Next week &rsaquo;</button>
      </div>
      <div className="schedule-layout">
        <aside className="media-library-panel">
          <h2>Media library</h2>
          <ul>
            {(media ?? []).map((item) => (
              <li key={item.id} className="media-library-item" draggable>
                <span>{item.title}</span>
              </li>
            ))}
          </ul>
        </aside>
        <div className="week-grid">
          {days.map((day, i) => (
            <div className="day-column" key={day.toISOString()}>
              <div className="day-column-header">
                {DAY_LABELS[i]} {day.getUTCDate()}
              </div>
              {slotsForDate(resolved ?? [], slotsById, day).map(({ slot, resolved: r }) => {
                const heightPercent = ((new Date(r.end_time).getTime() - new Date(r.start_time).getTime()) / (24 * MS_PER_HOUR)) * 100;
                const mediaItem = slot.kind === "media" ? mediaById.get(slot.media_item_id ?? -1) : undefined;
                const label = slot.kind === "gap" ? slot.gap_label || "Gap" : mediaItem?.title ?? `Media #${slot.media_item_id}`;
                const posterUrl = mediaPosterUrl(mediaItem);
                return (
                  <div
                    key={r.program_id}
                    data-testid={`slot-block-${r.program_id}`}
                    className={`slot-block slot-block-${slot.kind}`}
                    style={{ height: `${heightPercent}%` }}
                  >
                    {posterUrl ? <img className="slot-block-poster" src={posterUrl} alt={label} /> : <span>{label}</span>}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
