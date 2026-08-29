import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMediaItems } from "../api/media";
import { useTuneIn, type NextProgram } from "../api/playback";
import type { MediaItem } from "../api/types";
import { ChannelSwitcher } from "../components/ChannelSwitcher";
import { Interstitial } from "../components/Interstitial";
import { NowPlayingOverlay } from "../components/NowPlayingOverlay";
import { VideoPlayer } from "../components/VideoPlayer";
import "./TVScreen.css";

const LAST_CHANNEL_KEY = "personaltv.tv.lastChannelId";
const VOLUME_KEY = "personaltv.tv.volume";
const MUTED_KEY = "personaltv.tv.muted";

function readStoredVolume(): number {
  try {
    const stored = localStorage.getItem(VOLUME_KEY);
    const parsed = stored === null ? NaN : Number(stored);
    return Number.isFinite(parsed) && parsed >= 0 && parsed <= 1 ? parsed : 1;
  } catch {
    return 1;
  }
}

function readStoredMuted(): boolean {
  try {
    return localStorage.getItem(MUTED_KEY) === "true";
  } catch {
    return false;
  }
}

// What to call the next-up entry. A gap slot has no media item at all
// (mediaItemId is 0), so the media lookup would always miss and render the
// deliberate break as "Unknown" — its own label is the right name for it.
function nextUpTitle(next: NextProgram | null, mediaById: Map<number, MediaItem>): string | null {
  if (!next) return null;
  if (next.kind === "gap") return next.gapLabel || "Gap";
  return mediaById.get(next.mediaItemId)?.title ?? null;
}

export function TVScreen() {
  const params = useParams<{ channelId: string }>();
  const channelId = Number(params.channelId);
  const navigate = useNavigate();
  const { data: media } = useMediaItems();
  const mediaById = useMemo(() => new Map((media ?? []).map((m) => [m.id, m])), [media]);
  const { state, retune } = useTuneIn(channelId);

  const [videoError, setVideoError] = useState(false);
  const [rawCurrentTime, setRawCurrentTime] = useState(0);
  const [volume, setVolume] = useState(readStoredVolume);
  const [muted, setMuted] = useState(readStoredMuted);

  function handleVolumeChange(next: number) {
    setVolume(next);
    try {
      localStorage.setItem(VOLUME_KEY, String(next));
    } catch {
      // Best-effort, same as the last-watched-channel persistence below.
    }
  }

  function handleMuteToggle() {
    setMuted((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(MUTED_KEY, String(next));
      } catch {
        // Best-effort, same as the last-watched-channel persistence below.
      }
      return next;
    });
  }

  // A fresh tune-in event (new state object) means a fresh attempt: clear
  // any stale video error from the previous attempt and reset the
  // progress-bar clock so it doesn't show the old program's elapsed time.
  useEffect(() => {
    setVideoError(false);
    setRawCurrentTime(0);
  }, [state]);

  useEffect(() => {
    if (state.status === "playing") {
      try {
        localStorage.setItem(LAST_CHANNEL_KEY, String(channelId));
      } catch {
        // Best-effort: e.g. Safari private browsing (or blocked site data)
        // throws on write. The last-watched channel just won't persist
        // across reloads — not worth surfacing to the viewer.
      }
    }
  }, [state, channelId]);

  const isError = state.status === "error" || videoError;

  // The `.tv-screen` wrapper and ChannelSwitcher below are rendered exactly
  // once, by every reachable state — only `content` varies by branch. This
  // makes "a branch forgets the chrome" structurally impossible: useTuneIn
  // sets status back to "loading" on every re-tune-in (each program
  // boundary, each channel switch), not just on first mount, so a branch
  // that skipped the wrapper/switcher would flash the page's background
  // and drop channel controls on every ordinary program change, not just
  // at startup. It's also the escape hatch for a persistently broken
  // channel: ChannelSwitcher (and thus a way off the channel) must stay
  // reachable from the error branch too.
  let content: ReactNode;

  if (state.status === "loading") {
    content = <p>Tuning in…</p>;
  } else if (isError) {
    content = (
      <>
        <p role="alert">Something went wrong tuning in.</p>
        <button onClick={retune}>Retry</button>
      </>
    );
  } else if (state.status !== "playing") {
    // state.status === "off_air" | "unavailable" here. Written as a
    // fallthrough (not `status === "off_air" || status === "unavailable"`)
    // for the same TypeScript discriminated-union narrowing reason as
    // before: off_air/unavailable share one union member with a
    // two-literal discriminant, so checking `!== "playing"` is what lets
    // TS narrow it correctly in this branch.
    const nextTitle = nextUpTitle(state.next, mediaById);
    const next = state.next ? { title: nextTitle ?? "Unknown", startTime: state.next.startTime } : null;
    content = <Interstitial reason={state.status} next={next} />;
  } else {
    // state.status === "playing"
    const item = mediaById.get(state.mediaItemId);
    const nextTitle = nextUpTitle(state.next, mediaById);
    const src =
      state.mode === "direct"
        ? `/api/media/${state.mediaItemId}/stream`
        : `/api/playback/sessions/${state.sessionId}/playlist.m3u8`;
    // In hls mode the video element's own currentTime starts near 0 (the
    // session's own timeline, already seeked server-side) — add the
    // tune-in offset back on top for display. In direct mode currentTime
    // is already real (VideoPlayer sets it to offsetSec on load), so it
    // needs no adjustment.
    const displayedTimeSec = state.mode === "hls" ? state.offsetSec + rawCurrentTime : rawCurrentTime;

    content = (
      <>
        <VideoPlayer
          mode={state.mode}
          src={src}
          offsetSec={state.mode === "direct" ? state.offsetSec : undefined}
          volume={volume}
          muted={muted}
          onError={() => setVideoError(true)}
          onTimeUpdate={setRawCurrentTime}
        />
        <NowPlayingOverlay
          title={item?.title ?? "Unknown"}
          currentTimeSec={displayedTimeSec}
          durationSec={item?.duration_sec ?? 0}
          nextTitle={nextTitle}
          volume={volume}
          muted={muted}
          onVolumeChange={handleVolumeChange}
          onMuteToggle={handleMuteToggle}
        />
      </>
    );
  }

  return (
    <div className={isError ? "tv-screen tv-screen-error" : "tv-screen"}>
      {content}
      <ChannelSwitcher currentChannelId={channelId} onSelect={(id) => navigate(`/tv/${id}`)} />
    </div>
  );
}
