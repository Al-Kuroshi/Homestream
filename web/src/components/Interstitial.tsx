import { useEffect, useState } from "react";
import { formatCountdown, formatTime } from "../scheduling/time";
import "./Interstitial.css";

interface NextProgram {
  title: string;
  startTime: Date;
}

interface Props {
  reason: "off_air" | "unavailable";
  next: NextProgram | null;
}

const HEADINGS: Record<Props["reason"], string> = {
  off_air: "Nothing scheduled right now",
  unavailable: "Currently unavailable",
};

// Shown between programs (off_air) or when a scheduled program's file isn't
// playable (unavailable) — a blank screen with a next-up countdown rather
// than silently jumping ahead, per the design spec. The natural seam for a
// future recap/trailer/ad slot, not built now.
export function Interstitial({ reason, next }: Props) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    if (!next) return;
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, [next]);

  return (
    <div className="interstitial">
      <p className="interstitial-heading">{HEADINGS[reason]}</p>
      {next ? (
        <p className="interstitial-next">
          Up next: {next.title} at {formatTime(next.startTime)} — starts in{" "}
          {formatCountdown(next.startTime.getTime() - now.getTime())}
        </p>
      ) : (
        <p className="interstitial-next">Nothing else scheduled on this channel.</p>
      )}
    </div>
  );
}
