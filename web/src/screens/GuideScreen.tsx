import { useQueries } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useChannels } from "../api/channels";
import { useMediaItems } from "../api/media";
import { listPrograms } from "../api/programs";
import { buildTimeline, defaultGuideWindow, joinProgramsWithMedia, type TimelineBlock } from "../scheduling/guide";
import { formatTimeRange } from "../scheduling/time";
import "./GuideScreen.css";

const POLL_INTERVAL_MS = 30_000;
const NOW_TICK_MS = 60_000;

export function GuideScreen() {
  const { data: channels, isLoading: channelsLoading, isError: channelsError } = useChannels();
  const { data: media, isLoading: mediaLoading, isError: mediaError } = useMediaItems();

  // The default window is anchored once, at mount, rather than recomputed
  // from a moving "now" on every render: recentering it every tick would
  // shift every block's width/position underneath the viewer while they're
  // looking at it. Only the "now" line itself should move; the window it
  // moves across stays fixed for the component's lifetime (spec §4.1's
  // "default visible window").
  const [windowAnchor] = useState(() => new Date());
  const { start: windowStart, end: windowEnd } = defaultGuideWindow(windowAnchor);

  // `tick` is a counter, not a clock: its only job is forcing a re-render
  // every 60s so the freshly-sampled `now` below keeps advancing even when
  // nothing else (e.g. the program poll) happens to re-render first.
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((t) => t + 1), NOW_TICK_MS);
    return () => clearInterval(id);
  }, []);
  const now = new Date();

  const programQueries = useQueries({
    queries: (channels ?? []).map((channel) => ({
      queryKey: ["channels", channel.id, "programs"] as const,
      queryFn: () => listPrograms(channel.id),
      refetchInterval: POLL_INTERVAL_MS,
    })),
  });

  const mediaById = useMemo(() => new Map((media ?? []).map((m) => [m.id, m])), [media]);
  const totalMs = windowEnd.getTime() - windowStart.getTime();

  if (channelsLoading || mediaLoading) return <p>Loading guide…</p>;
  if (channelsError || mediaError) return <p role="alert">Failed to load the guide.</p>;

  const rows = (channels ?? [])
    .map((channel, index) => ({ channel, programs: programQueries[index]?.data ?? [] }))
    .filter((row) => row.channel.enabled);

  const showNowLine = now >= windowStart && now < windowEnd;
  const nowPercent = ((now.getTime() - windowStart.getTime()) / totalMs) * 100;

  return (
    <section>
      <h1>Guide</h1>
      <div className="guide-grid">
        {showNowLine && (
          <div className="guide-now-line" data-testid="now-line" style={{ left: `${nowPercent}%` }} />
        )}
        {rows.map(({ channel, programs }) => {
          const joined = joinProgramsWithMedia(programs, mediaById);
          const timeline = buildTimeline(joined, windowStart, windowEnd);
          return (
            <div className="guide-row" key={channel.id}>
              <div className="guide-channel-name">{channel.name}</div>
              <div className="guide-timeline">
                {timeline.map((block, i) => (
                  <TimelineBlockView key={i} block={block} totalMs={totalMs} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
      {rows.length === 0 && <p>No enabled channels to show.</p>}
    </section>
  );
}

function TimelineBlockView({ block, totalMs }: { block: TimelineBlock; totalMs: number }) {
  const widthPercent = ((block.end.getTime() - block.start.getTime()) / totalMs) * 100;
  if (block.type === "off-air") {
    return <div className="guide-block guide-block-offair" style={{ width: `${widthPercent}%` }}>Off Air</div>;
  }
  return (
    <div
      className="guide-block guide-block-program"
      style={{ width: `${widthPercent}%` }}
      title={formatTimeRange(block.program.start, block.program.end)}
    >
      {block.program.title}
    </div>
  );
}
