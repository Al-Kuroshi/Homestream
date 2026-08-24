# Personal TV — Progress / Session Handoff

**Last updated:** 2026-08-24

This file exists so a new session (human or Claude Code) can pick up where the last one left off without re-reading the whole conversation history. It tracks *where we are*, not *what the product is* — for that, see the docs below.

## Where things live

- **Product requirements (canonical):** `docs/prd/HomeStreamer.md`
- **Technical design spec (backend):** `docs/design/2026-08-21-personal-tv-design.md`
- **Technical design spec (frontend):** `docs/design/2026-08-23-personal-tv-frontend-foundation-design.md`
- **Technical design spec (playback):** `docs/design/2026-08-23-personal-tv-playback-design.md` — approved, implementation plan being written now (see Status).
- **Implementation plan (Plan 1 — Core Backend):** `docs/plans/2026-08-21-personal-tv-core-backend.md` — merged.
- **Implementation plan (Plan 2 — Frontend Foundation):** `docs/plans/2026-08-23-personal-tv-frontend-foundation.md` — merged.
- **Implementation plan (Plan 3 — Playback Backend):** `docs/plans/2026-08-23-personal-tv-playback-backend.md` — written, ready to execute.
- **Repo guidance for Claude Code:** `CLAUDE.md`
- **You are here:** working directly on `main` (no active worktree). Plan 1 (backend), Plan 2 (frontend), and the mutation-error-handling follow-up are all merged. Plan 3 (playback backend) has an approved design spec and a written implementation plan (6 tasks); it's about to run through the same subagent-driven-development process as Plans 1 and 2.

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

## Next step

**Plan 3 (playback backend)** is ready to execute: design spec approved, implementation plan written (`docs/plans/2026-08-23-personal-tv-playback-backend.md`, 6 tasks) — a new `internal/playback` package (compatibility matrix, tune-in orchestration with missing-file exclusion, in-memory `SessionManager` with idle-timeout-only cleanup, `ffmpeg`-based HLS transcoding) plus three new REST endpoints (`POST /api/channels/{id}/watch`, `GET /api/media/{id}/stream`, `GET /api/playback/sessions/{id}/{file}`), added additively with zero changes to any existing route.

**TV/player screen** is still not built — it's a separate, smaller follow-up plan once the playback endpoints exist and are curl-able (mirroring how Plan 1 shipped before any UI existed). Docker packaging is also not started.

Minor, non-blocking items parked during the frontend's final review (fine to pick up opportunistically, not tracked further than this list): no column-sorting on the Media Library table (design spec says "sortable", never implemented); a reorder test in `ChannelsListScreen.test.tsx` that doesn't check *which* channel got which position; a white-box test asserting Guide's polling interval via the query cache rather than observed behavior; no `AppRoutes.test.tsx` coverage for the `/channels/:id` route; a couple of cache-invalidation completeness gaps (deleting a source doesn't invalidate the media query key; deleting a channel doesn't invalidate its programs key) mitigated today by TanStack Query's default refetch-on-mount; an unused `channelId` field on `UpdateProgramInput`/`DeleteProgramInput`; one un-memoized lookup in `ChannelScheduleScreen.tsx` (Guide's equivalent is memoized); no overlap warning in the schedule editor (the Guide now clips overlaps gracefully, but nothing tells the user they created one); missing assets return `200 text/html` (SPA fallback) instead of `404`; no delete-confirmation on channels (Settings has this pattern for sources, Channels doesn't); and a `/// <reference types="node" />` in `web/src/test/setup.ts` that leaks `@types/node`'s ambient globals across the whole `tsconfig.app.json` compilation unit rather than staying scoped to that one file (low practical risk today — nothing else uses `process`/`Buffer` — but worth knowing if a future browser-code file accidentally references a Node global and type-checks successfully before crashing at runtime).

## Key decisions made (see the design specs for full reasoning)

- **Backend:** Go, single static binary, `modernc.org/sqlite` (pure-Go, no CGO).
- **Frontend:** React + TypeScript SPA (Vite), embedded into the Go binary via `go:embed`. Plain CSS (no UI framework), React Router, TanStack Query, Vitest/RTL/MSW for testing. See the frontend design spec for the full rationale (persistent sidebar nav, timeline-grid Guide over a simpler now/next list, no drag-and-drop scheduling, Settings scoped to media sources only).
- **Database:** SQLite behind repository interfaces, swappable to PostgreSQL later without touching business logic.
- **Media source (MVP):** local filesystem only, including NAS/network shares via a Docker bind mount.
- **Metadata/subtitles:** no internet enrichment in MVP — filename + `ffprobe` technical metadata only.
- **Scheduling:** pure function of `(schedule, wall clock)`, recomputed on demand — no background ticking process, nothing lost on restart. Off-air gaps between programs are a first-class state, not an error, on both backend and frontend.
- **Media scanning:** manual rescan only for MVP, no filesystem watcher.
