import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useChannels, useCreateChannel, useDeleteChannel, useUpdateChannel } from "../api/channels";
import type { Channel } from "../api/types";
import "./ChannelsListScreen.css";

export function ChannelsListScreen() {
  const { data: channels, isLoading, isError } = useChannels();
  const createChannel = useCreateChannel();
  const updateChannel = useUpdateChannel();
  const deleteChannel = useDeleteChannel();
  const [name, setName] = useState("");
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renameValue, setRenameValue] = useState("");

  const sorted = [...(channels ?? [])].sort((a, b) => a.position - b.position);

  function handleCreate(e: FormEvent) {
    e.preventDefault();
    createChannel.mutate({ name, position: sorted.length }, { onSuccess: () => setName("") });
  }

  function startRename(channel: Channel) {
    setRenamingId(channel.id);
    setRenameValue(channel.name);
  }

  function commitRename(channel: Channel) {
    updateChannel.mutate({
      id: channel.id, name: renameValue, description: channel.description,
      enabled: channel.enabled, position: channel.position,
    });
    setRenamingId(null);
  }

  function toggleEnabled(channel: Channel) {
    updateChannel.mutate({
      id: channel.id, name: channel.name, description: channel.description,
      enabled: !channel.enabled, position: channel.position,
    });
  }

  function move(channel: Channel, direction: -1 | 1) {
    const index = sorted.findIndex((c) => c.id === channel.id);
    const other = sorted[index + direction];
    if (!other) return;
    updateChannel.mutate({
      id: channel.id, name: channel.name, description: channel.description,
      enabled: channel.enabled, position: other.position,
    });
    updateChannel.mutate({
      id: other.id, name: other.name, description: other.description,
      enabled: other.enabled, position: channel.position,
    });
  }

  if (isLoading) return <p>Loading channels…</p>;
  if (isError) return <p role="alert">Failed to load channels.</p>;

  return (
    <section>
      <h1>Channels</h1>
      <ul className="channel-list">
        {sorted.map((channel, index) => (
          <li key={channel.id}>
            <div className="channel-reorder">
              <button aria-label={`Move ${channel.name} up`} onClick={() => move(channel, -1)} disabled={index === 0}>↑</button>
              <button aria-label={`Move ${channel.name} down`} onClick={() => move(channel, 1)} disabled={index === sorted.length - 1}>↓</button>
            </div>
            {renamingId === channel.id ? (
              <>
                <input
                  value={renameValue}
                  onChange={(e) => setRenameValue(e.target.value)}
                  aria-label={`Rename ${channel.name}`}
                />
                <button onClick={() => commitRename(channel)}>Save</button>
                <button onClick={() => setRenamingId(null)}>Cancel</button>
              </>
            ) : (
              <Link to={`/channels/${channel.id}`}>{channel.name}</Link>
            )}
            <label>
              <input type="checkbox" checked={channel.enabled} onChange={() => toggleEnabled(channel)} />
              Enabled
            </label>
            {renamingId !== channel.id && <button onClick={() => startRename(channel)}>Rename</button>}
            <button onClick={() => deleteChannel.mutate(channel.id)}>Delete</button>
          </li>
        ))}
      </ul>
      {sorted.length === 0 && <p>No channels yet.</p>}

      <h2>Create a channel</h2>
      <form onSubmit={handleCreate}>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <button type="submit" disabled={createChannel.isPending}>Create channel</button>
      </form>
    </section>
  );
}
