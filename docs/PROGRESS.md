# Personal TV — Progress / Session Handoff

**Last updated:** 2026-08-24 (Plan 3 complete)

This file exists so a new session (human or Claude Code) can pick up where the last one left off without re-reading the whole conversation history. It tracks *where we are*, not *what the product is* — for that, see the docs below.

## Where things live

- **Product requirements (canonical):** `docs/prd/HomeStreamer.md`
- **Technical design spec (backend):** `docs/design/2026-08-21-personal-tv-design.md`
- **Technical design spec (frontend):** `docs/design/2026-08-23-personal-tv-frontend-foundation-design.md`
- **Technical design spec (playback):** `docs/design/2026-08-23-personal-tv-playback-design.md` — approved, implementation plan being written now (see Status).
- **Implementation plan (Plan 1 — Core Backend):** `docs/plans/2026-08-21-personal-tv-core-backend.md` — merged.
- **Implementation plan (Plan 2 — Frontend Foundation):** `docs/plans/2026-08-23-personal-tv-frontend-foundation.md` — merged.
- **Implementation plan (Plan 3 — Playback Backend):** `docs/plans/2026-08-23-personal-tv-playback-backend.md` — implemented, all 6 tasks complete, awaiting merge decision (see Status).
- **Repo guidance for Claude Code:** `CLAUDE.md`
- **You are here:** work happened in git worktree `.claude/worktrees/playback-backend` on branch `worktree-playback-backend` (based on `main` @ `86d0946`), not yet merged. Plan 1 (backend), Plan 2 (frontend), and the mutation-error-handling follow-up are all merged to `main`. Plan 3 (playback backend) ran through subagent-driven-development end to end — all 6 tasks implemented and individually reviewed, plus a final whole-branch review that found real bugs (fixed) — and is ready for `superpowers:finishing-a-development-branch` to decide how it lands.

## Status

**Backend (Plan 1):** merged to `main`. Go, SQLite, media scanner, scheduler, channels service, REST API — all working, tested, `go test ./... -race` clean.

**Frontend (Plan 2):** merged to `main`. React + TypeScript SPA (Vite/Vitest/MSW) covering Guide (EPG timeline grid with off-air-gap rendering), Media Library, Channels (list + schedule editor), and Settings (media sources) — all against the existing backend REST API, no backend changes except the additive `go:embed` static-serving wiring. `go run ./cmd/personaltv` serves the built SPA end-to-end.

**Mutation error handling (the one gap the frontend's final review deferred):** done, as a small bounded follow-up (brainstormed and approved, no separate plan doc). A shared `MutationError` component (`web/src/components/MutationError.tsx`) now backs an inline error banner on every mutation across all three screens that were missing it — Channels (create/rename/toggle/reorder/delete), Channel Schedule (add/edit/remove program), and Settings (rescan/delete source, plus the pre-existing add-source one refactored to use the same component). Design spec §6 is now fully implemented. `go build/vet/test ./...` and `cd web && npx tsc -b && npm run lint && npm test` (82 tests) all clean.

During implementation, several real bugs were caught and fixed by implementers/reviewers rather than shipped silently — worth knowing about if you're touching this code:
- A missing app-wide `QueryClientProvider` (the real app would have crashed on first render of any data-fetching screen) — fixed in the Task 5 fix round.
- A scheduling-math bug where the Guide's off-air-gap logic didn't clip overlapping programs, so two overlapping programs would render as two overlapping blocks — fixed in Task 10.
- A tautology bug in the Guide's "now" indicator: the visible time window was recomputed every render from the same clock used to test whether the line should show, which is mathematically always true — the line could never actually move. Fixed in Task 11 by anchoring the window at mount.
- A `go test ./...` failure on a fresh clone (the embed tests asserted on real built content that doesn't exist before `npm run build` has run) — fixed in the final review's fix wave with a skip guard.
- Unmatched `/api/*` requests (typos, wrong methods) were silently falling through to the SPA and returning `200 text/html`, which broke the frontend's own error handling (`apiGet` would resolve with `undefined` instead of throwing) — fixed with an explicit `/api/` 404 registered before the SPA catch-all.
- A UTC/local time display mismatch: users typed a local start time but saw it echoed back in UTC with no indication — fixed by making display match input (local time throughout), with tests pinned to `TZ=UTC` for determinism instead.

**Playback backend (Plan 3):** implemented on branch `worktree-playback-backend`, not yet merged to `main`. New `internal/playback` package — direct-play compatibility matrix (Task 1), media path resolution + direct-play streaming endpoint (Task 2), `ffmpeg`-based HLS `SessionManager` with idle-timeout cleanup and orphan cleanup on startup (Task 3), HLS session-serving endpoint with a path-traversal guard (Task 4), tune-in orchestration as a pure function of `(schedule, now, file existence)` that never advances to a future program to route around a missing file (Task 5), and full wiring into `cmd/personaltv/main.go` plus an end-to-end proof test (Task 6) — added additively with zero changes to any of the 15 pre-existing routes/call sites. `go build/vet/test ./... -race` clean; `gofmt -l .` empty.

All 6 tasks passed individual spec+quality review. The final whole-branch review (dispatched on the most capable model) caught a real, load-bearing bug the individual task reviews had missed — worth knowing about if you're touching this code:
- **`-hls_time 2` was silently inert.** HLS can only cut segments at keyframes, and libx264's default GOP (~250 frames, ~10s) meant `-hls_time 2` had no effect — segments came out at ~10s, not 2s. This cascaded into three more problems the same bug was masking: the hardcoded 5s startup-detection deadline was actually racing a ~10s encode (would intermittently fail on real hardware even though it passed in CI with a tiny synthetic fixture), the plan's own "growing playlist" Definition-of-Done item was untested (a 6s test fixture produced exactly one segment, so the "wait for a `.ts` file" test was vacuous), and 4 test files' missing `sessions.Close()` calls were only harmless by accident (ffmpeg finished before the test ended, so nothing was still running to race `t.TempDir()`'s cleanup). Fixed by adding `-force_key_frames "expr:gte(t,n_forced*2)"` to force keyframes at the interval `-hls_time` expects, `-preset veryfast`, a configurable `startupTimeout` (default 15s, up from the hardcoded 5s), a rewritten segmentation test that proves real growth (12s fixture, polls for ≥2 segments), and the 4 missing cleanup calls.
- Also fixed in the same pass: transcode session directory was a fixed, unconfigurable path under `os.TempDir()` — now `PERSONALTV_SESSIONS_DIR`-configurable, matching the existing `PERSONALTV_DB_PATH`/`PERSONALTV_PORT` convention (a prerequisite the not-yet-written Docker packaging plan would otherwise have had to retrofit). Two production symbols (`PlaybackServiceForTest`, `StartTestSession`) that existed solely for test access were removed in favor of the test helpers returning the real `*playback.SessionManager` they already construct.
- A design decision worth flagging forward: in `hls` mode, `offset_sec` in the watch response has *already* been applied via ffmpeg's `-ss` seek when the session started — a client must not seek by it again, or it'll double-seek. Documented on `TuneInResult`/`watchResponse`, but there's no client yet to get this wrong (see below) — whoever writes the TV/player UI should read that doc comment first.
- `docs/design/2026-08-23-personal-tv-playback-design.md` §4/§6 are now stale: they still describe a "walk forward and serve the next existing program" behavior that the plan's Global Constraints deliberately overrode with "never skip ahead" (Task 5's `TestTuneIn_DoesNotJumpAheadToAFutureProgramWhenCurrentFileIsMissing` locks in the *opposite* of what the spec's §6 describes testing). The code is correct per the plan; the spec doc itself needs a note added so a future reader doesn't "fix" the code back to match stale prose.

## Next step

**Decide how the playback-backend branch lands** — run `superpowers:finishing-a-development-branch` from the `worktree-playback-backend` branch (or merge it, per your normal workflow, then continue from `main`).

**TV/player screen** is still not built — it's a separate, smaller follow-up plan once the playback endpoints exist and are curl-able (mirroring how Plan 1 shipped before any UI existed; this plan's own end-to-end test already proves the endpoints work via curl-equivalent HTTP calls). Docker packaging is also not started, though `PERSONALTV_SESSIONS_DIR` (added in this plan) removes one blocker for it.

Minor, non-blocking items parked during the frontend's final review (fine to pick up opportunistically, not tracked further than this list): no column-sorting on the Media Library table (design spec says "sortable", never implemented); a reorder test in `ChannelsListScreen.test.tsx` that doesn't check *which* channel got which position; a white-box test asserting Guide's polling interval via the query cache rather than observed behavior; no `AppRoutes.test.tsx` coverage for the `/channels/:id` route; a couple of cache-invalidation completeness gaps (deleting a source doesn't invalidate the media query key; deleting a channel doesn't invalidate its programs key) mitigated today by TanStack Query's default refetch-on-mount; an unused `channelId` field on `UpdateProgramInput`/`DeleteProgramInput`; one un-memoized lookup in `ChannelScheduleScreen.tsx` (Guide's equivalent is memoized); no overlap warning in the schedule editor (the Guide now clips overlaps gracefully, but nothing tells the user they created one); missing assets return `200 text/html` (SPA fallback) instead of `404`; no delete-confirmation on channels (Settings has this pattern for sources, Channels doesn't); and a `/// <reference types="node" />` in `web/src/test/setup.ts` that leaks `@types/node`'s ambient globals across the whole `tsconfig.app.json` compilation unit rather than staying scoped to that one file (low practical risk today — nothing else uses `process`/`Buffer` — but worth knowing if a future browser-code file accidentally references a Node global and type-checks successfully before crashing at runtime).

## Key decisions made (see the design specs for full reasoning)

- **Backend:** Go, single static binary, `modernc.org/sqlite` (pure-Go, no CGO).
- **Frontend:** React + TypeScript SPA (Vite), embedded into the Go binary via `go:embed`. Plain CSS (no UI framework), React Router, TanStack Query, Vitest/RTL/MSW for testing. See the frontend design spec for the full rationale (persistent sidebar nav, timeline-grid Guide over a simpler now/next list, no drag-and-drop scheduling, Settings scoped to media sources only).
- **Database:** SQLite behind repository interfaces, swappable to PostgreSQL later without touching business logic.
- **Media source (MVP):** local filesystem only, including NAS/network shares via a Docker bind mount.
- **Metadata/subtitles:** no internet enrichment in MVP — filename + `ffprobe` technical metadata only.
- **Scheduling:** pure function of `(schedule, wall clock)`, recomputed on demand — no background ticking process, nothing lost on restart. Off-air gaps between programs are a first-class state, not an error, on both backend and frontend.
- **Media scanning:** manual rescan only for MVP, no filesystem watcher.
