# Personal TV — TV/Player Screen — Design Spec

**Status:** Approved by user in brainstorming session, ready for implementation planning.
**Extends:** `docs/design/2026-08-23-personal-tv-frontend-foundation-design.md` (app shell, routing, screen conventions) and `docs/design/2026-08-23-personal-tv-playback-design.md` (the three playback endpoints this screen consumes).
**Depends on:** the playback backend (`docs/plans/2026-08-23-personal-tv-playback-backend.md`), fully implemented and merged to `main`.

## 1. Scope

This is the fifth and final MVP screen (**TV**), deferred by both prior frontend/playback plans specifically until this point. It builds the video player and channel-switching experience against the three existing playback endpoints (`POST /api/channels/{id}/watch`, `GET /api/media/{id}/stream`, `GET /api/playback/sessions/{id}/{file}`) and the existing `GET /api/channels/{id}/now`. **No backend changes** — this is frontend-only.

**Explicitly out of scope for this plan** (confirmed during brainstorming):
- Short interstitial clips (recaps, trailers, ads) during off-air gaps — the `Interstitial` component is deliberately built as the seam for this later, but nothing plays there now beyond the countdown text.
- Docker packaging — a separate follow-up plan, sequenced after this one.
- Any change to the playback backend's API contract or behavior.

## 2. Route & navigation

- `/tv` — resolves to a concrete channel and redirects to `/tv/:channelId`: the last-watched channel (persisted in `localStorage`) if it's still enabled, otherwise the first enabled channel ordered by `position`. If there are zero enabled channels, renders an empty state ("No channels yet — go create one") linking to `/channels` instead of redirecting.
- `/tv/:channelId` — the concrete screen. Persists `channelId` to `localStorage` on every successful tune-in, so `/tv` picks up where the user left off next time.
- The sidebar's existing reserved TV slot (`web/src/components/Sidebar.tsx`) links to `/tv`.

## 3. Data flow — tune-in events, no polling loop

Consistent with the product's core principle that channel state is a pure function of `(schedule, wall-clock time)`, recomputed on demand rather than tracked as ticking state, the player is **event-driven with self-scheduling timers** — there is no `setInterval` polling anywhere in this screen.

A **tune-in event** fires on: initial mount, a channel switch (prev/next or picked from the overlay), the user clicking "Retry" after an error, or a previously-scheduled timer firing. Each tune-in event:

1. Calls `POST /api/channels/{id}/watch` and `GET /api/channels/{id}/now` together (one additional cheap DB-backed GET, not a poll — `/now` is the only source of the real schedule `start_time`/`end_time` for the current and next program, which `/watch`'s response doesn't carry).
2. **If `/watch` reports `status: "playing"`:** render the video (§5). Compute time-remaining from `/now`'s `current.end_time` and set exactly one `setTimeout` for that moment. When it fires, run a new tune-in event — whatever the backend reports as current then (the next program, or off-air) is simply what plays, with no separate "advance to next program" logic on the client. This self-corrects for clock skew: if the timer fires a little early, the re-tune-in just reports the same program still playing with a fresh `offset_sec`, and reschedules.
3. **If `status` is `"off_air"` or `"unavailable"`:** render the interstitial (§6), no video element. If `/now`'s `next` is present, show "Up next: *Title* at HH:MM" with a live client-side countdown, and set a `setTimeout` for `next.start_time` that fires a new tune-in event. If `next` is null (nothing else scheduled on this channel), show a static "nothing else scheduled on this channel" message with no timer.
4. Every timer set in steps 2–3 is cleared on unmount and on the next tune-in event, so a channel switch never leaves a stale timer racing the new one.

Program/media titles for the current and next program come from the existing `useMediaItems()`-style list already used elsewhere (e.g. `GuideScreen.tsx`'s `mediaById` map pattern) joined against `/now`'s `media_item_id` fields — no new media-lookup endpoint needed.

## 4. Session-keepalive note

While in `hls` mode, `hls.js`'s own ongoing segment fetches against `GET /api/playback/sessions/{id}/{file}` already call `Touch()` server-side on every request (per the playback backend's design) — no separate keepalive ping is needed from this screen.

## 5. Video playback

`VideoPlayer.tsx` is a presentational component: given `mode: 'direct' | 'hls'`, a `src`, and (direct mode only) `offsetSec`, it owns nothing about channels or scheduling.

- **`mode: 'direct'`** — `src` is `/api/media/{media_item_id}/stream`. On the `<video>` element's `loadedmetadata` event, set `video.currentTime = offsetSec`; the browser's own subsequent range requests handle seeking within the file for free (matches the playback design spec §2.2).
- **`mode: 'hls'`** — `src` is `/api/playback/sessions/{session_id}/playlist.m3u8`. **`offsetSec` is never applied to the player in this mode** — per the doc comment added to `TuneInResult`/`watchResponse` during the playback backend's final review, `ffmpeg` already seeked via `-ss` when the session started; double-applying the offset client-side would double-seek. Play from the start of the (already-offset) playlist.
- **New dependency: `hls.js`** (MIT-licensed, the standard browser HLS player). `VideoPlayer` feature-detects native HLS support via `video.canPlayType('application/vnd.apple.mpegurl')` (true in Safari) and only loads/constructs `hls.js` when that's false.
- **Autoplay:** call `.play()` once a source is set. If the returned promise rejects (browser autoplay policy — most likely on a fresh page load at `/tv/:id` with no preceding user gesture), show a "tap to play" button overlaying the video instead of failing silently.

## 6. Components

- **`TVScreen.tsx`** — route-level. Owns `channelId` from the URL, the channel list (`useChannels()`, already exists), prev/next cycling, and renders the pieces below based on the current tune-in state.
- **`useTuneIn(channelId)`** (new hook, `web/src/api/playback.ts`) — owns the tune-in event flow (§3): calls `/watch` + `/now`, returns a discriminated state (`loading | playing | off_air | unavailable | error`), manages the self-scheduling timer, cleans it up on unmount/channel change. This is the one nontrivial piece of new logic in this plan; everything else is presentational.
- **`VideoPlayer.tsx`** — §5.
- **`NowPlayingOverlay.tsx`** — auto-hiding overlay (title, progress bar computed from `video.currentTime`/media duration, next-up text). Fades in on mouse-move/tap over the video, auto-hides after a few seconds of inactivity. Video is full-bleed; this floats over it.
- **`Interstitial.tsx`** — one component for both `off_air` and `unavailable` (same shape: a heading plus an optional next-up countdown); a `reason: 'off_air' | 'unavailable'` prop swaps the heading text. The natural seam for a future recap/trailer/ad slot, not built now.
- **`ChannelSwitcher.tsx`** — the toggleable channel-list overlay (grid/list of enabled channels by name, jump directly to one) plus the prev/next cycling logic shared with `TVScreen`'s on-screen controls.

## 7. Error handling

- **`/watch` or `/now` request fails** (network, 404, 500): error state with a "Retry" button that re-runs the tune-in event.
- **HLS session fails mid-playback** (the session-serving endpoint starts 500ing because `Session.Failed()`): `hls.js` surfaces an error event; treated identically to a tune-in failure — error state, "Retry" re-tunes, which starts a fresh transcode session.
- **Direct-play file vanishes mid-playback:** the `<video>` element's own `error` event, same treatment.
- **Zero enabled channels:** empty state at `/tv`, §2.

## 8. Testing strategy

Colocated Vitest/RTL tests, MSW-mocked at the network boundary — matching every other screen's established convention in this repo. `hls.js` depends on MediaSource Extensions, unavailable in jsdom: the `hls.js` module itself is mocked in unit tests (asserting it's constructed with the correct playlist URL, not exercising real segment loading); real HLS playback is verified manually in a browser, the same way the playback backend's `ffmpeg` behavior was verified empirically rather than unit-tested.

Coverage: `useTuneIn`'s full state machine (all four states, timer scheduling and re-tune-in on firing, cleanup on unmount/channel-change — including the self-correction case where a timer fires slightly early and the re-tune-in reports the same program still playing); direct-play `<video>` src/`currentTime` wiring; HLS mode never applying `offsetSec` to the player; the interstitial's countdown and its "nothing else scheduled" fallback; prev/next channel cycling; the zero-channels empty state; and the autoplay-blocked fallback button.

## 9. Open questions

None outstanding — all decisions in this spec were confirmed during the brainstorming session that produced it.
