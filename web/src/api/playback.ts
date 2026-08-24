import { useCallback, useEffect, useRef, useState } from "react";
import { apiGet, apiPost } from "./http";

export type WatchResponse =
  | { status: "playing"; mode: "direct"; media_item_id: number; offset_sec: number }
  | { status: "playing"; mode: "hls"; media_item_id: number; offset_sec: number; session_id: string }
  | { status: "off_air" }
  | { status: "unavailable" };

export interface ProgramState {
  program_id: number;
  media_item_id: number;
  start_time: string;
  end_time: string;
}

export interface NowResponse {
  channel_id: number;
  current: ProgramState | null;
  offset_sec: number;
  next: ProgramState | null;
}

export function watchChannel(channelId: number): Promise<WatchResponse> {
  return apiPost<WatchResponse>(`/channels/${channelId}/watch`);
}

export function getChannelNow(channelId: number): Promise<NowResponse> {
  return apiGet<NowResponse>(`/channels/${channelId}/now`);
}

interface NextProgram {
  mediaItemId: number;
  startTime: Date;
}

export type TuneInState =
  | { status: "loading" }
  | { status: "error" }
  | {
      status: "playing";
      mode: "direct" | "hls";
      mediaItemId: number;
      offsetSec: number;
      sessionId?: string;
      next: NextProgram | null;
    }
  | { status: "off_air" | "unavailable"; next: NextProgram | null };

// useTuneIn owns the tune-in event flow (design spec §3): on mount, on
// channelId change, or when a self-scheduled timer fires, it calls the
// watch and now endpoints together and derives the next state plus at most
// one setTimeout — for when the current program ends (re-checks what's
// current then) or for when the next program starts (re-checks in case it
// becomes playable). There is deliberately no polling interval anywhere
// here: channel state is a pure function of (schedule, wall-clock time),
// recomputed on demand, matching the same principle the backend scheduler
// already follows.
export function useTuneIn(channelId: number): { state: TuneInState; retune: () => void } {
  const [state, setState] = useState<TuneInState>({ status: "loading" });
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Incremented on every tune-in attempt and on unmount/channelId change; a
  // response is only applied if it's still the most recent attempt, so a
  // slow response for a tune-in event the user has already moved past
  // (channel switch, or a newer timer firing) never overwrites newer state
  // or schedules a stale timer.
  const generationRef = useRef(0);

  const tuneIn = useCallback(() => {
    const myGeneration = ++generationRef.current;
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setState({ status: "loading" });

    Promise.all([watchChannel(channelId), getChannelNow(channelId)])
      .then(([watch, now]) => {
        if (myGeneration !== generationRef.current) return;

        const next: NextProgram | null = now.next
          ? { mediaItemId: now.next.media_item_id, startTime: new Date(now.next.start_time) }
          : null;

        if (watch.status === "playing") {
          setState({
            status: "playing",
            mode: watch.mode,
            mediaItemId: watch.media_item_id,
            offsetSec: watch.offset_sec,
            sessionId: watch.mode === "hls" ? watch.session_id : undefined,
            next,
          });
          if (now.current) {
            const remainingMs = new Date(now.current.end_time).getTime() - Date.now();
            timerRef.current = setTimeout(tuneIn, Math.max(remainingMs, 0));
          }
        } else {
          setState({ status: watch.status, next });
          if (next) {
            const untilNextMs = next.startTime.getTime() - Date.now();
            timerRef.current = setTimeout(tuneIn, Math.max(untilNextMs, 0));
          }
        }
      })
      .catch(() => {
        if (myGeneration !== generationRef.current) return;
        setState({ status: "error" });
      });
  }, [channelId]);

  useEffect(() => {
    tuneIn();
    return () => {
      generationRef.current++;
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [tuneIn]);

  return { state, retune: tuneIn };
}
