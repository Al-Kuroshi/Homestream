import { Link, Navigate } from "react-router-dom";
import { useChannels } from "../api/channels";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";

// Resolves "/tv" to a concrete "/tv/:channelId": the last-watched channel
// (persisted by TVScreen) if it's still enabled, otherwise the first
// enabled channel by position. If there are no enabled channels, shows an
// empty state instead of redirecting anywhere.
export function TVIndexScreen() {
  const { data: channels, isLoading } = useChannels();

  if (isLoading) return <p>Loading…</p>;

  const enabled = [...(channels ?? [])].filter((c) => c.enabled).sort((a, b) => a.position - b.position);
  if (enabled.length === 0) {
    return (
      <section>
        <h1>TV</h1>
        <p>
          No channels yet — <Link to="/channels">go create one</Link>.
        </p>
      </section>
    );
  }

  const lastId = Number(localStorage.getItem(LAST_CHANNEL_KEY));
  const target = enabled.find((c) => c.id === lastId) ?? enabled[0];
  return <Navigate to={`/tv/${target.id}`} replace />;
}
