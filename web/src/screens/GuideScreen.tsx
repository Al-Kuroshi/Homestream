import { useQueries, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { listChannels } from "../api/channels";
import { listMedia } from "../api/media";
import { listPrograms } from "../api/programs";
import { buildTimeline, defaultGuideWindow, joinProgramsWithMedia, type TimelineBlock } from "../scheduling/guide";
import { formatTimeRange } from "../scheduling/time";
import "./GuideScreen.css";

const POLL_INTERVAL_MS = 30_000;
const NOW_TICK_MS = 60_000;

export function GuideScreen() {
  // Bypass the shared useChannels()/useMediaItems() convenience hooks here:
  // this screen needs channels/media to poll every 30s to catch schedule
  // changes made elsewhere (spec §4.1), but adding a default
  // refetchInterval to those hooks would poll on every screen that uses
  // them. Reusing the identical query keys keeps the cache shared with
  // other screens (their reads still benefit from this polling and vice
  // versa) without touching api/channels.ts or api/media.ts.
  const { data: channels, isLoading: channelsLoading, isError: channelsError } = useQuery({
    queryKey: ["channels"],
    queryFn: listChannels,
    refetchInterval: POLL_INTERVAL_MS,
  });
  const { data: media, isLoading: mediaLoading, isError: mediaError } = useQuery({
    queryKey: ["media"],
    queryFn: listMedia,
    refetchInterval: POLL_INTERVAL_MS,
  });

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
    .map((channel, index) => ({
      channel,
      programs: programQueries[index]?.data ?? [],
      isError: programQueries[index]?.isError ?? false,
    }))
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
        {rows.map(({ channel, programs, isError }) => {
          return (
            <div className="guide-row" key={channel.id}>
              <div className="guide-channel-name">{channel.name}</div>
              <div className="guide-timeline">
                {isError ? (
                  <div className="guide-block guide-block-error" style={{ width: "100%" }}>
                    Schedule unavailable
                  </div>
                ) : (
                  buildTimeline(joinProgramsWithMedia(programs, mediaById), windowStart, windowEnd).map(
                    (block, i) => <TimelineBlockView key={i} block={block} totalMs={totalMs} />
                  )
                )}
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
