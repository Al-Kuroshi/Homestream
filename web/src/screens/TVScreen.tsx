import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMediaItems } from "../api/media";
import { useTuneIn } from "../api/playback";
import { ChannelSwitcher } from "../components/ChannelSwitcher";
import { Interstitial } from "../components/Interstitial";
import { NowPlayingOverlay } from "../components/NowPlayingOverlay";
import { VideoPlayer } from "../components/VideoPlayer";
import "./TVScreen.css";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";

export function TVScreen() {
  const params = useParams<{ channelId: string }>();
  const channelId = Number(params.channelId);
  const navigate = useNavigate();
  const { data: media } = useMediaItems();
  const mediaById = useMemo(() => new Map((media ?? []).map((m) => [m.id, m])), [media]);
  const { state, retune } = useTuneIn(channelId);

  const [videoError, setVideoError] = useState(false);
  const [rawCurrentTime, setRawCurrentTime] = useState(0);

  // A fresh tune-in event (new state object) means a fresh attempt: clear
  // any stale video error from the previous attempt and reset the
  // progress-bar clock so it doesn't show the old program's elapsed time.
  useEffect(() => {
    setVideoError(false);
    setRawCurrentTime(0);
  }, [state]);

  useEffect(() => {
    if (state.status === "playing") {
      localStorage.setItem(LAST_CHANNEL_KEY, String(channelId));
    }
  }, [state, channelId]);

  if (state.status === "loading") return <p>Tuning in…</p>;

  if (state.status === "error" || videoError) {
    return (
      <div className="tv-screen tv-screen-error">
        <p role="alert">Something went wrong tuning in.</p>
        <button onClick={retune}>Retry</button>
      </div>
    );
  }

  const nextTitle = state.next ? mediaById.get(state.next.mediaItemId)?.title ?? null : null;

  // Written as `status !== "playing"` (rather than the more obvious
  // `status === "off_air" || status === "unavailable"`) so TypeScript can
  // actually narrow `state` in both branches: since off_air/unavailable
  // share one union member with a two-literal discriminant, an equality
  // check against just one of those literals doesn't get subtracted from
  // that member in the negative branch, leaving `state.mediaItemId` etc.
  // below still typed as possibly missing.
  if (state.status !== "playing") {
    const next = state.next ? { title: nextTitle ?? "Unknown", startTime: state.next.startTime } : null;
    return (
      <div className="tv-screen">
        <Interstitial reason={state.status} next={next} />
        <ChannelSwitcher currentChannelId={channelId} onSelect={(id) => navigate(`/tv/${id}`)} />
      </div>
    );
  }

  // state.status === "playing"
  const item = mediaById.get(state.mediaItemId);
  const src =
    state.mode === "direct"
      ? `/api/media/${state.mediaItemId}/stream`
      : `/api/playback/sessions/${state.sessionId}/playlist.m3u8`;
  // In hls mode the video element's own currentTime starts near 0 (the
  // session's own timeline, already seeked server-side) — add the tune-in
  // offset back on top for display. In direct mode currentTime is already
  // real (VideoPlayer sets it to offsetSec on load), so it needs no
  // adjustment.
  const displayedTimeSec = state.mode === "hls" ? state.offsetSec + rawCurrentTime : rawCurrentTime;

  return (
    <div className="tv-screen">
      <VideoPlayer
        mode={state.mode}
        src={src}
        offsetSec={state.mode === "direct" ? state.offsetSec : undefined}
        onError={() => setVideoError(true)}
        onTimeUpdate={setRawCurrentTime}
      />
      <NowPlayingOverlay
        title={item?.title ?? "Unknown"}
        currentTimeSec={displayedTimeSec}
        durationSec={item?.duration_sec ?? 0}
        nextTitle={nextTitle}
      />
      <ChannelSwitcher currentChannelId={channelId} onSelect={(id) => navigate(`/tv/${id}`)} />
    </div>
  );
}
