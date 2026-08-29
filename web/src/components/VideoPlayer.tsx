import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import "./VideoPlayer.css";

interface Props {
  mode: "direct" | "hls";
  src: string;
  offsetSec?: number;
  volume?: number;
  muted?: boolean;
  onError: () => void;
  onTimeUpdate?: (currentTimeSec: number) => void;
}

// Dumb wrapper around <video>: knows nothing about channels, scheduling, or
// tune-in state. mode selects native playback (direct) or hls.js — or
// native HLS on Safari — for hls; offsetSec (direct mode only) is applied
// once metadata loads. Per the playback backend's design (TuneInResult's
// doc comment): in hls mode the offset was already applied server-side via
// ffmpeg's -ss seek, so it must never be applied here too.
export function VideoPlayer({ mode, src, offsetSec, volume = 1, muted = false, onError, onTimeUpdate }: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [blockedByAutoplay, setBlockedByAutoplay] = useState(false);

  // Keep the latest callbacks in refs so the setup effect below doesn't
  // need them as dependencies — depending on them directly would re-run the
  // whole video/hls.js setup (restarting playback) every time a parent
  // re-render happens to pass a new function identity, which has nothing to
  // do with mode/src/offsetSec actually changing.
  const onErrorRef = useRef(onError);
  const onTimeUpdateRef = useRef(onTimeUpdate);
  useEffect(() => {
    onErrorRef.current = onError;
    onTimeUpdateRef.current = onTimeUpdate;
  });

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    setBlockedByAutoplay(false);

    let hls: Hls | undefined;

    function attemptPlay() {
      video!.play().catch(() => setBlockedByAutoplay(true));
    }

    function handleLoadedMetadata() {
      if (mode === "direct" && offsetSec !== undefined) {
        video!.currentTime = offsetSec;
      }
      attemptPlay();
    }

    function handleError() {
      onErrorRef.current();
    }

    function handleTimeUpdate() {
      onTimeUpdateRef.current?.(video!.currentTime);
    }

    video.addEventListener("loadedmetadata", handleLoadedMetadata);
    video.addEventListener("error", handleError);
    video.addEventListener("timeupdate", handleTimeUpdate);

    if (mode === "hls" && !video.canPlayType("application/vnd.apple.mpegurl")) {
      if (Hls.isSupported()) {
        hls = new Hls();
        hls.on(Hls.Events.ERROR, (_event, data) => {
          if (data.fatal) onErrorRef.current();
        });
        hls.loadSource(src);
        hls.attachMedia(video);
      } else {
        onErrorRef.current();
      }
    } else {
      video.src = src;
    }

    return () => {
      video.removeEventListener("loadedmetadata", handleLoadedMetadata);
      video.removeEventListener("error", handleError);
      video.removeEventListener("timeupdate", handleTimeUpdate);
      hls?.destroy();
    };
  }, [mode, src, offsetSec]);

  // Kept separate from the setup effect above: volume/muted must apply live
  // without tearing down and recreating hls.js or reloading src.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = volume;
    video.muted = muted;
  }, [volume, muted]);

  return (
    <div className="video-player">
      <video ref={videoRef} className="video-player-el" data-testid="video-el" />
      {blockedByAutoplay && (
        <button
          className="video-player-tap-to-play"
          onClick={() => {
            videoRef.current?.play();
            setBlockedByAutoplay(false);
          }}
        >
          ▶ Tap to play
        </button>
      )}
    </div>
  );
}
