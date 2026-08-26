import { useState } from "react";
import type { DragEvent } from "react";
import { useParams } from "react-router-dom";
import { useChannel } from "../api/channels";
import { useMediaItems } from "../api/media";
import { useAddSlot, useDeleteSlot, useResolvedSlots, useSlotsForChannel, useUpdateSlot } from "../api/slots";
import type { MediaItem } from "../api/types";
import { MutationError } from "../components/MutationError";
import { addWeeks, positionForInsert, slotsForDate, startOfWeekUTC, weekDates } from "../scheduling/week";
import "./ChannelScheduleScreen.css";

const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MS_PER_HOUR = 60 * 60 * 1000;

type PendingPlacement =
  | { kind: "media"; mediaItemId?: number; dayOfWeek: number; date: Date; insertBeforeIndex: number; existingSlotId?: number }
  // gapLabel is carried on the pending placement (rather than re-derived at
  // confirm time) because PUT /api/slots/{id} is a full replace: moving an
  // existing gap has to send back the label it already had, not a fresh
  // default, or the move silently renames it.
  | { kind: "gap"; dayOfWeek: number; date: Date; insertBeforeIndex: number; existingSlotId?: number; gapLabel?: string };

const DEFAULT_GAP_MINUTES = "5";
const DEFAULT_GAP_LABEL = "Gap";

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

  const addSlotMutation = useAddSlot(channelId);
  const updateSlotMutation = useUpdateSlot(channelId);
  const deleteSlotMutation = useDeleteSlot(channelId);

  const [pending, setPending] = useState<PendingPlacement | null>(null);
  const [pendingRecurring, setPendingRecurring] = useState(true);
  const [pendingGapMinutes, setPendingGapMinutes] = useState(DEFAULT_GAP_MINUTES);

  // Which drop zone the pointer is currently over mid-drag. Tracked in state
  // (fed by onDragEnter/onDragLeave) rather than left to CSS :hover, which
  // most browsers suppress while a native HTML5 drag is in flight.
  const [dragOverZoneId, setDragOverZoneId] = useState<string | null>(null);

  // Everything a drop zone needs, in one place, so the "start of day" zone
  // and the between-blocks zones can't drift apart.
  function dropZoneProps(zoneId: string, dayOfWeek: number, date: Date, insertBeforeIndex: number) {
    return {
      className: dragOverZoneId === zoneId ? "day-drop-zone day-drop-zone-active" : "day-drop-zone",
      "data-testid": zoneId,
      onDragOver: (e: DragEvent) => e.preventDefault(),
      onDragEnter: () => setDragOverZoneId(zoneId),
      // Only clear if this zone is still the active one: if the pointer has
      // already entered the next zone, that zone's enter must win.
      onDragLeave: () => setDragOverZoneId((current) => (current === zoneId ? null : current)),
      onDrop: (e: DragEvent) => {
        setDragOverZoneId(null);
        handleDrop(e, dayOfWeek, date, insertBeforeIndex);
      },
    };
  }

  function handleDragStartMedia(e: DragEvent, mediaItemId: number) {
    e.dataTransfer.setData("application/json", JSON.stringify({ mediaItemId }));
  }

  function handleDragStartSlot(e: DragEvent, existingSlotId: number) {
    e.dataTransfer.setData("application/json", JSON.stringify({ existingSlotId }));
  }

  function handleDrop(e: DragEvent, dayOfWeek: number, date: Date, insertBeforeIndex: number) {
    e.preventDefault();
    const payload = JSON.parse(e.dataTransfer.getData("application/json")) as
      | { mediaItemId: number }
      | { existingSlotId: number }
      | { gap: true };

    setPendingRecurring(true);
    if ("gap" in payload) {
      setPendingGapMinutes(DEFAULT_GAP_MINUTES);
      setPending({ kind: "gap", dayOfWeek, date, insertBeforeIndex });
      return;
    }
    if ("mediaItemId" in payload) {
      setPending({ kind: "media", mediaItemId: payload.mediaItemId, dayOfWeek, date, insertBeforeIndex });
      return;
    }
    const existing = slotsById.get(payload.existingSlotId);
    if (!existing) return;
    if (existing.kind === "gap") {
      // Seed the form (and the pending placement) from what this gap
      // already is, so a plain move round-trips its own duration/label
      // instead of replacing them with whatever the last *new* gap used.
      setPendingGapMinutes(existing.gap_duration_sec ? String(existing.gap_duration_sec / 60) : DEFAULT_GAP_MINUTES);
      setPending({
        kind: "gap",
        dayOfWeek,
        date,
        insertBeforeIndex,
        existingSlotId: existing.id,
        gapLabel: existing.gap_label || DEFAULT_GAP_LABEL,
      });
    } else {
      setPending({
        kind: "media",
        mediaItemId: existing.media_item_id ?? undefined,
        dayOfWeek,
        date,
        insertBeforeIndex,
        existingSlotId: existing.id,
      });
    }
  }

  function computePosition(dayOfWeek: number, insertBeforeIndex: number, excludeSlotId?: number) {
    const daySlots = (slots ?? [])
      .filter((s) => s.recurring && s.day_of_week === dayOfWeek)
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    const excludeIndex = excludeSlotId !== undefined ? daySlots.findIndex((s) => s.id === excludeSlotId) : -1;
    const adjustedIndex = excludeIndex !== -1 && excludeIndex < insertBeforeIndex ? insertBeforeIndex - 1 : insertBeforeIndex;
    const existingPositions = daySlots.filter((s) => s.id !== excludeSlotId).map((s) => s.position ?? 0);
    return positionForInsert(existingPositions, adjustedIndex);
  }

  function computeOneOffStartTime(date: Date, insertBeforeIndex: number): string {
    const dayBlocks = slotsForDate(resolved ?? [], slotsById, date);
    if (insertBeforeIndex <= 0) return date.toISOString();
    const previous = dayBlocks[insertBeforeIndex - 1];
    return previous ? previous.resolved.end_time : date.toISOString();
  }

  function confirmPending() {
    if (!pending) return;
    const base = {
      channelId,
      recurring: pendingRecurring,
      ...(pendingRecurring
        ? { day_of_week: pending.dayOfWeek, position: computePosition(pending.dayOfWeek, pending.insertBeforeIndex, pending.existingSlotId) }
        : { start_time: computeOneOffStartTime(pending.date, pending.insertBeforeIndex) }),
    };
    const body =
      pending.kind === "gap"
        ? {
            ...base,
            kind: "gap" as const,
            gap_duration_sec: Number(pendingGapMinutes) * 60,
            gap_label: pending.gapLabel ?? DEFAULT_GAP_LABEL,
          }
        : { ...base, kind: "media" as const, media_item_id: pending.mediaItemId! };

    if (pending.existingSlotId !== undefined) {
      const existing = slotsById.get(pending.existingSlotId)!;
      updateSlotMutation.mutate({ id: existing.id, ...body });
    } else {
      addSlotMutation.mutate(body);
    }
    setPending(null);
  }

  if (channelLoading || resolvedLoading) return <p>Loading schedule…</p>;
  if (channelError || resolvedError || !channel) return <p role="alert">Failed to load this channel's schedule.</p>;

  return (
    <section>
      <h1>{channel.name}</h1>
      <div className="week-nav">
        <button onClick={() => setWeekOffset((o) => o - 1)}>&lsaquo; Previous week</button>
        <span>{formatWeekBoundary(weekStart)} – {formatWeekBoundary(new Date(weekEnd.getTime() - 1))}</span>
        {/* This grid's day columns and its midnight boundaries are computed
            in UTC (an approved global constraint), while the Guide and TV
            screens render clock times in the viewer's local timezone. For a
            non-UTC viewer the same slot can therefore appear under a
            different day/time on those screens — say so here rather than
            leaving it to be discovered. */}
        <span className="week-nav-timezone-note">(all times UTC)</span>
        <button onClick={() => setWeekOffset((o) => o + 1)}>Next week &rsaquo;</button>
      </div>
      <div className="schedule-layout">
        <aside className="media-library-panel">
          <h2>Media library</h2>
          <ul>
            <li
              className="media-library-item media-library-gap-item"
              draggable
              onDragStart={(e) => e.dataTransfer.setData("application/json", JSON.stringify({ gap: true }))}
            >
              <span>Gap / Break</span>
            </li>
            {(media ?? []).map((item) => (
              <li
                key={item.id}
                className="media-library-item"
                draggable
                onDragStart={(e) => handleDragStartMedia(e, item.id)}
              >
                <span>{item.title}</span>
              </li>
            ))}
          </ul>
        </aside>
        <div className="week-grid">
          <MutationError isError={addSlotMutation.isError} error={addSlotMutation.error} />
          <MutationError isError={updateSlotMutation.isError} error={updateSlotMutation.error} />
          <MutationError isError={deleteSlotMutation.isError} error={deleteSlotMutation.error} />
          {pending && (
            <div className="pending-placement-form" role="dialog" aria-label="Confirm placement">
              <label>
                <input
                  type="checkbox"
                  checked={pendingRecurring}
                  onChange={(e) => setPendingRecurring(e.target.checked)}
                  aria-label="Repeats weekly"
                />
                Repeats weekly
              </label>
              {pending.kind === "gap" && (
                <label>
                  Gap duration (minutes)
                  <input
                    type="number"
                    min={1}
                    value={pendingGapMinutes}
                    onChange={(e) => setPendingGapMinutes(e.target.value)}
                    aria-label="Gap duration (minutes)"
                  />
                </label>
              )}
              <button onClick={confirmPending}>{pending.kind === "gap" ? "Add gap" : "Add"}</button>
              <button onClick={() => setPending(null)}>Cancel</button>
            </div>
          )}
          {days.map((day, i) => {
            const dayOfWeek = day.getUTCDay();
            const daySlots = slotsForDate(resolved ?? [], slotsById, day);
            return (
              <div className="day-column" key={day.toISOString()}>
                <div className="day-column-header">
                  {DAY_LABELS[i]} {day.getUTCDate()}
                </div>
                <div {...dropZoneProps(`day-drop-zone-${dayOfWeek}-start`, dayOfWeek, day, 0)} />
                {daySlots.map(({ slot, resolved: r }, index) => {
                  const heightPercent = ((new Date(r.end_time).getTime() - new Date(r.start_time).getTime()) / (24 * MS_PER_HOUR)) * 100;
                  const mediaItem = slot.kind === "media" ? mediaById.get(slot.media_item_id ?? -1) : undefined;
                  const label = slot.kind === "gap" ? slot.gap_label || "Gap" : mediaItem?.title ?? `Media #${slot.media_item_id}`;
                  const posterUrl = mediaPosterUrl(mediaItem);
                  return (
                    <div key={r.program_id}>
                      <div
                        data-testid={`slot-block-${r.program_id}`}
                        className={`slot-block slot-block-${slot.kind}`}
                        style={{ height: `${heightPercent}%` }}
                        draggable
                        onDragStart={(e) => handleDragStartSlot(e, slot.id)}
                      >
                        {posterUrl ? <img className="slot-block-poster" src={posterUrl} alt={label} /> : <span>{label}</span>}
                        {/* draggable={false} + stopPropagation keep this button
                            from hijacking the parent block's own HTML5 drag:
                            without them, a press-and-move that starts on the ×
                            would begin (or abort) a slot move instead of
                            behaving like an ordinary button. */}
                        <button
                          type="button"
                          className="slot-block-delete"
                          draggable={false}
                          aria-label={`Delete ${label}`}
                          title={`Delete ${label}`}
                          onDragStart={(e) => e.stopPropagation()}
                          onClick={(e) => {
                            e.stopPropagation();
                            deleteSlotMutation.mutate({ id: slot.id, channelId });
                          }}
                        >
                          ×
                        </button>
                      </div>
                      <div
                        {...dropZoneProps(
                          `day-drop-zone-${dayOfWeek}-${index === daySlots.length - 1 ? "end" : index}`,
                          dayOfWeek,
                          day,
                          index + 1
                        )}
                      />
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
