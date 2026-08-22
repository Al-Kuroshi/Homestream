import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";
import { useChannel } from "../api/channels";
import { useMediaItems } from "../api/media";
import { useAddProgram, useDeleteProgram, useProgramsForChannel, useUpdateProgram } from "../api/programs";
import { computeEndTime, formatTimeRange, toDatetimeLocalValue } from "../scheduling/time";
import "./ChannelScheduleScreen.css";

export function ChannelScheduleScreen() {
  const params = useParams<{ id: string }>();
  const channelId = Number(params.id);
  const { data: channel, isLoading: channelLoading, isError: channelError } = useChannel(channelId);
  const { data: programs, isLoading: programsLoading, isError: programsError } = useProgramsForChannel(channelId);
  const { data: media } = useMediaItems();
  const addProgram = useAddProgram(channelId);
  const updateProgram = useUpdateProgram(channelId);
  const deleteProgram = useDeleteProgram(channelId);

  const [mediaItemId, setMediaItemId] = useState<number | "">("");
  const [startTime, setStartTime] = useState("");
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editStartTime, setEditStartTime] = useState("");

  const mediaById = new Map((media ?? []).map((m) => [m.id, m]));
  const sortedPrograms = [...(programs ?? [])].sort(
    (a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
  );

  function handleAdd(e: FormEvent) {
    e.preventDefault();
    if (mediaItemId === "" || !startTime) return;
    addProgram.mutate(
      { channelId, media_item_id: mediaItemId, start_time: new Date(startTime).toISOString() },
      { onSuccess: () => { setMediaItemId(""); setStartTime(""); } }
    );
  }

  function startEdit(programId: number, currentStart: string) {
    setEditingId(programId);
    setEditStartTime(toDatetimeLocalValue(currentStart));
  }

  function commitEdit(programId: number, mediaItemIdForProgram: number) {
    updateProgram.mutate({
      id: programId, channelId, media_item_id: mediaItemIdForProgram,
      start_time: new Date(editStartTime).toISOString(),
    });
    setEditingId(null);
  }

  if (channelLoading || programsLoading) return <p>Loading schedule…</p>;
  if (channelError || programsError || !channel) return <p role="alert">Failed to load this channel's schedule.</p>;

  return (
    <section>
      <h1>{channel.name}</h1>
      <ul className="program-list">
        {sortedPrograms.map((program) => {
          const item = mediaById.get(program.media_item_id);
          const end = computeEndTime(program.start_time, item?.duration_sec ?? 0);
          return (
            <li key={program.id}>
              <span>{item?.title ?? `Media #${program.media_item_id}`}</span>
              {editingId === program.id ? (
                <>
                  <input
                    type="datetime-local"
                    value={editStartTime}
                    onChange={(e) => setEditStartTime(e.target.value)}
                    aria-label="Edit start time"
                  />
                  <button onClick={() => commitEdit(program.id, program.media_item_id)}>Save</button>
                  <button onClick={() => setEditingId(null)}>Cancel</button>
                </>
              ) : (
                <>
                  <span>{formatTimeRange(new Date(program.start_time), end)}</span>
                  <button onClick={() => startEdit(program.id, program.start_time)}>Edit start time</button>
                </>
              )}
              <button onClick={() => deleteProgram.mutate({ id: program.id, channelId })}>Remove</button>
            </li>
          );
        })}
      </ul>
      {sortedPrograms.length === 0 && <p>No programs scheduled yet.</p>}

      <h2>Add a program</h2>
      <form onSubmit={handleAdd}>
        <label>
          Media
          <select
            value={mediaItemId}
            onChange={(e) => setMediaItemId(e.target.value === "" ? "" : Number(e.target.value))}
            aria-label="Media"
            required
          >
            <option value="">Select media…</option>
            {(media ?? []).map((m) => (
              <option key={m.id} value={m.id}>{m.title}</option>
            ))}
          </select>
        </label>
        <label>
          Start time
          <input
            type="datetime-local"
            value={startTime}
            onChange={(e) => setStartTime(e.target.value)}
            aria-label="Start time"
            required
          />
        </label>
        <button type="submit" disabled={addProgram.isPending}>Add program</button>
      </form>
    </section>
  );
}
