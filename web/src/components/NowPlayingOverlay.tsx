import { useEffect, useRef, useState } from "react";
import "./NowPlayingOverlay.css";

interface Props {
  title: string;
  currentTimeSec: number;
  durationSec: number;
  nextTitle: string | null;
}

const HIDE_AFTER_MS = 3000;

// Auto-hiding "now playing" bar: visible on mount/activity, fades out after
// a few seconds of no mouse/touch activity anywhere on the page, so the
// video stays unobstructed during normal viewing. Self-contained (listens
// on window directly) so it doesn't need TVScreen to coordinate pointer
// events with it.
export function NowPlayingOverlay({ title, currentTimeSec, durationSec, nextTitle }: Props) {
  const [visible, setVisible] = useState(true);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    function showThenScheduleHide() {
      setVisible(true);
      if (hideTimer.current !== null) clearTimeout(hideTimer.current);
      hideTimer.current = setTimeout(() => setVisible(false), HIDE_AFTER_MS);
    }
    showThenScheduleHide();
    window.addEventListener("mousemove", showThenScheduleHide);
    window.addEventListener("touchstart", showThenScheduleHide);
    return () => {
      window.removeEventListener("mousemove", showThenScheduleHide);
      window.removeEventListener("touchstart", showThenScheduleHide);
      if (hideTimer.current !== null) clearTimeout(hideTimer.current);
    };
  }, []);

  const progressPercent = durationSec > 0 ? Math.min(100, (currentTimeSec / durationSec) * 100) : 0;

  return (
    <div className={`now-playing-overlay${visible ? "" : " now-playing-overlay-hidden"}`}>
      <p className="now-playing-title">{title}</p>
      <div className="now-playing-progress-track">
        <div className="now-playing-progress-fill" style={{ width: `${progressPercent}%` }} />
      </div>
      {nextTitle && <p className="now-playing-next">Next: {nextTitle}</p>}
    </div>
  );
}
