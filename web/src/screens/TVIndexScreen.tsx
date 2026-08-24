import { Link, Navigate } from "react-router-dom";
import { useChannels } from "../api/channels";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";

// Reads the last-watched channel id, tolerating localStorage being
// unavailable (e.g. Safari private browsing, or site data blocked) rather
// than letting a thrown SecurityError/QuotaExceededError take down the
// whole /tv route mid-render. NaN correctly falls through to the existing
// enabled.find(...) ?? enabled[0] fallback below — no other change needed.
function readLastChannelId(): number {
  try {
    return Number(localStorage.getItem(LAST_CHANNEL_KEY));
  } catch {
    return NaN;
  }
}

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

  const lastId = readLastChannelId();
  const target = enabled.find((c) => c.id === lastId) ?? enabled[0];
  return <Navigate to={`/tv/${target.id}`} replace />;
}
