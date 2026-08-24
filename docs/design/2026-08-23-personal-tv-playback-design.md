# Personal TV — Playback Backend — Design Spec

**Status:** Approved by user in brainstorming session, ready for implementation planning.
**Extends:** `docs/design/2026-08-21-personal-tv-design.md` §4.4 (`playback`), which already fixed the core architectural shape (per-viewer lazy sessions, direct-play-vs-transcode decision, HLS for incompatible sources, missing-file skip-forward). This spec makes that concrete enough to plan and build.
**Depends on:** the core backend (`docs/plans/2026-08-21-personal-tv-core-backend.md`) and the frontend foundation (`docs/plans/2026-08-23-personal-tv-frontend-foundation.md`), both merged to `main`.

## 1. Scope

This plan builds the **playback backend only**: a new `internal/playback` package, three new HTTP endpoints, and the `ffmpeg`-based transcode/session machinery. It does **not** build the TV/player screen — that consumes these endpoints from the frontend and is a separate, smaller follow-up plan once these endpoints exist and are curl-able, mirroring how the core backend (Plan 1) shipped and was verified end-to-end via `curl` before any UI existed.

**Explicitly out of scope for this plan** (confirmed during brainstorming):
- Seeking backward to before the tune-in point (forward-only, "join a live broadcast" semantics — matches the product's own framing).
- Multi-viewer session sharing (each viewer gets an independent transcode session; a documented future optimization per the original design spec, not built now).
- Partial stream-copy optimization (e.g. copying an already-compatible video stream while only re-encoding audio). When transcoding, both video and audio are always fully re-encoded to `h264`/`aac`, even if only one track is technically incompatible. Simpler, matches the original spec's binary direct-play/transcode split, costs some transcode CPU in the mixed-incompatibility case — an acceptable MVP simplification, not a hidden gap.
- Subtitles (PRD non-goal, unchanged).
- Any TV/player UI.

## 2. API contract

Three new endpoints, all under the existing `internal/api` package (extending `Routes()` the same additive way `SetStaticHandler` was added — no changes to any existing route).

### 2.1 `POST /api/channels/{id}/watch` — tune in

Asks the already-built `channels.Service.CurrentState` for what's playing right now, resolves it to a playable file (walking forward through the schedule if the scheduled file is missing — see §4), decides direct-play vs. transcode (see §3), and — for a transcode — starts a session.

Always returns `200` with a discriminated JSON body, matching the existing `/api/channels/{id}/now` endpoint's own precedent of "always 200, `current: null` for off-air" rather than mixing in special-case status codes for a normal product state:

```json
// currently playing, direct-play compatible
{ "status": "playing", "mode": "direct", "media_item_id": 42, "offset_sec": 137.5 }

// currently playing, needs transcoding
{ "status": "playing", "mode": "hls", "session_id": "a1b2c3d4-...", "offset_sec": 137.5 }

// off-air: a valid state, nothing scheduled right now (mirrors /now's current: null)
{ "status": "off_air" }

// unavailable: programs are scheduled, but none from now onward have a playable file
{ "status": "unavailable" }
```

`404` if the channel itself doesn't exist (matching the existing `GetChannel`-first pattern used elsewhere in the API).

### 2.2 `GET /api/media/{id}/stream` — direct play

Serves the media item's raw file bytes via Go's standard library `http.ServeContent`, which handles `Range` request headers (seeking) correctly with no custom byte-math. Resolves the media item to an absolute path via its `MediaSource.Path` + `MediaItem.RelPath` (the same join the scanner already does internally, exposed here as a small resolver function). `404` if the media item doesn't exist or its file is missing on disk (the same "missing file" condition §4 handles at tune-in time, surfaced directly here for a client that already has a direct-play URL and the file vanished mid-playback).

The frontend sets `video.currentTime = offset_sec` once the element's metadata loads; the browser's own subsequent range requests handle seeking within the file for free — no offset-aware logic needed in this endpoint itself.

### 2.3 `GET /api/playback/sessions/{id}/{file}` — HLS playlist and segments

Serves the session's `playlist.m3u8` (content-type `application/vnd.apple.mpegurl`) and `.ts` segment files (content-type `video/mp2t`) from that session's temp directory. Every request — playlist or segment — updates the session's last-access timestamp (this is the sole signal the idle-timeout sweep uses, per §5). `404` if the session ID doesn't exist (already torn down, or never existed) or the requested file isn't in that session's directory (no path traversal outside it).

## 3. Compatibility matrix

A pure function, `IsDirectPlayCompatible(videoCodec, audioCodec, container string) bool`, taking exactly the fields already stored on `MediaItem` from scan-time `ffprobe` — no new probing at tune-in time:

- **Video codec:** `h264` only.
- **Audio codec:** `aac` or `mp3`.
- **Container:** `ffprobe`'s `format_name` for the mp4 family is the literal string `"mov,mp4,m4a,3gp,3g2,mj2"` (already observed verbatim in `MediaItem.Container` for scanned files) — this exact string is the only accepted container.

Anything else — `h265`/`hevc`, `vp9`, `av1` video; `ac3`/`dts`/other audio; an `mkv`/`avi`/`webm` container even if the codecs inside are otherwise compatible (browsers generally cannot demux `mkv` via a plain `<video>` element regardless of what's inside it) — transcodes. Deliberately the narrowest matrix that covers "a typical h264/aac mp4 rip plays with zero CPU cost": false negatives just cost transcode CPU, false positives mean a broken player, so the criteria stay conservative rather than maximizing direct-play coverage.

## 4. Missing-file handling (tune-in time)

> **Superseded by the implementation plan** (`docs/plans/2026-08-23-personal-tv-playback-backend.md`, Global Constraints): the "walk forward to the next playable program" behavior described below was deliberately overridden before implementation. The shipped behavior never advances to a program before its own scheduled `start_time` — a missing file at the *current* slot is reported as `unavailable`, not routed around by serving a future program early. This keeps channel state a pure function of `(schedule, wall-clock time, which files exist on disk)`, matching the scheduler's core principle that nothing is tracked as separate mutable state. `TestTuneIn_DoesNotJumpAheadToAFutureProgramWhenCurrentFileIsMissing` (`internal/playback/tunein_test.go`) locks in this behavior — the opposite of what §6's "Missing-file skip-forward" test below describes. If you're touching this code, follow the plan/test, not this section's original prose.

When the scheduled program's file doesn't exist on disk at tune-in time: `playback` walks forward through that channel's remaining schedule (same data `CurrentState`/`ListPrograms` already expose) looking for the next program whose file *does* exist, and serves that one instead, logging the skip (matching the core backend's existing "a single unavailable item never aborts the whole channel" principle, applied here to playback instead of scanning). If no program from now onward has a playable file, the endpoint returns `{"status": "unavailable"}` rather than hanging or erroring — a distinct, valid response the frontend can render as "nothing available on this channel right now," separate from `off_air`'s "nothing is scheduled at all."

This walk only needs to check file existence (a cheap `os.Stat`), not re-probe codecs — codec info is already cached from scan time on every candidate `MediaItem`.

## 5. Session lifecycle

**In-memory only** — a concurrency-safe map of session ID → session state (the running `ffmpeg` process, its temp directory, last-access timestamp), owned by a `SessionManager`. Nothing persisted: a server restart drops all active sessions, which is fine — there's no cross-restart viewing-state requirement anywhere in the PRD, and this mirrors the scheduler's own "nothing lost on restart, nothing ticking with no viewers" principle.

**Cleanup: idle-timeout only** (confirmed during brainstorming — no explicit client-side "stop watching" signal). A background sweep goroutine (interval e.g. 30s) checks every session's last-access timestamp; any session idle past a threshold (e.g. 60s — long enough to survive normal player buffering, short enough not to leak `ffmpeg` processes) gets its process killed and its temp directory removed. The idle threshold is a `SessionManager` constructor parameter, not a hardcoded constant, specifically so tests can use a short window instead of waiting out a real 60 seconds.

**Segment storage:** a per-session subdirectory under the OS temp directory (e.g. `os.TempDir()/personaltv-playback/{session-id}/`), removed on teardown. On startup, `playback` sweeps `os.TempDir()/personaltv-playback/` for leftover subdirectories from an unclean prior shutdown and removes them — the same "clean up orphaned state on startup" shape, applied to sessions instead of scanner state.

**`ffmpeg` invocation:** input-seeks to the computed offset (`-ss <offset> -i <path>`, before `-i` for fast approximate seeking — decoding from the true start of a multi-hour file just to reach the offset would be far slower and isn't needed for this product's accuracy requirements) and encodes to HLS with `-hls_playlist_type event` (an appending, non-expiring playlist — segments already produced stay in the playlist and are seekable, matching "seek forward within what's already streamed works normally," rather than a sliding live-window playlist that would drop earlier segments).

**Process failure:** `playback` watches the `ffmpeg` process. If it exits before producing a first segment, or exits/crashes later, the session is marked failed; the playlist/segment endpoint (§2.3) then returns an error for that session instead of letting the player buffer forever waiting for segments that will never arrive.

## 6. Testing strategy

- **Compatibility matrix** — pure function, no I/O, fully unit-tested directly (every codec/container combination named in §3, both directions).
- **`ffmpeg` subprocess behavior** — integration-tested against real short synthetic videos, reusing the `generateTestVideo`-style helper pattern already established in the media scanner's tests (`internal/mediastore`) — not mocked, per the original design spec's own testing note. Covers: a transcode session produces a valid playlist and at least one real segment within a bounded wait; a deliberately-broken/unreadable input surfaces as a failed session (per §5) rather than hanging.
- **Idle-timeout sweep** — since the threshold is a constructor parameter, tests use a short window (e.g. tens of milliseconds) and assert the process is actually gone and the temp directory actually removed afterward — no real-time waiting.
- **Missing-file skip-forward** — integration test: schedule two programs on a channel, delete the first's underlying file, tune in, assert the response is for the second program. A variant with *no* playable program in the remaining schedule asserts `{"status": "unavailable"}`.
- **Direct play** — integration test against a real generated video: request with a `Range` header, assert the correct byte range comes back (proving `http.ServeContent` is wired correctly against the resolved path, not reimplementing range-request logic).
- **Off-air** — reuses the existing off-air test setup pattern already established for `/api/channels/{id}/now` (a channel with no current program) — the same channel state, a new assertion against the `/watch` endpoint's response shape.

## 7. Open questions

None outstanding — all decisions in this spec were confirmed during the brainstorming session that produced it.
