import { useState } from "react";
import { useChannels } from "../api/channels";
import "./ChannelSwitcher.css";

interface Props {
  currentChannelId: number;
  onSelect: (channelId: number) => void;
}

export function ChannelSwitcher({ currentChannelId, onSelect }: Props) {
  const { data: channels } = useChannels();
  const [listOpen, setListOpen] = useState(false);
  const enabled = [...(channels ?? [])].filter((c) => c.enabled).sort((a, b) => a.position - b.position);

  function cycle(direction: -1 | 1) {
    if (enabled.length === 0) return;
    const index = enabled.findIndex((c) => c.id === currentChannelId);
    const nextIndex = ((index === -1 ? 0 : index) + direction + enabled.length) % enabled.length;
    onSelect(enabled[nextIndex].id);
  }

  return (
    <div className="channel-switcher">
      <button aria-label="Previous channel" onClick={() => cycle(-1)}>◀</button>
      <button aria-label="Show channel list" onClick={() => setListOpen((v) => !v)}>☰</button>
      <button aria-label="Next channel" onClick={() => cycle(1)}>▶</button>
      {listOpen && (
        <ul className="channel-switcher-list">
          {enabled.map((channel) => (
            <li key={channel.id}>
              <button
                className={channel.id === currentChannelId ? "channel-switcher-active" : undefined}
                onClick={() => {
                  onSelect(channel.id);
                  setListOpen(false);
                }}
              >
                {channel.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
