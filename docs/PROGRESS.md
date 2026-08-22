# Personal TV — Progress / Session Handoff

**Last updated:** 2026-08-22

This file exists so a new session (human or Claude Code) can pick up where the last one left off without re-reading the whole conversation history. It tracks *where we are*, not *what the product is* — for that, see the docs below.

## Where things live

- **Product requirements (canonical):** `docs/prd/HomeStreamer.md`
- **Technical design spec:** `docs/design/2026-08-21-personal-tv-design.md`
- **Implementation plan (Plan 1 — Core Backend):** `docs/plans/2026-08-21-personal-tv-core-backend.md`
- **Repo guidance for Claude Code:** `CLAUDE.md`
- **You are here:** git worktree `personal-tv-core-backend`, branch `worktree-personal-tv-core-backend`, 19 commits ahead of `main`. Ready to integrate — the `superpowers:finishing-a-development-branch` skill is running now to decide how this branch reaches `main`.

## Status

PRD and design spec are done and approved. Plan 1 (core backend: DB, media scanner, scheduler, channels service, REST API) is **fully implemented, tested, and reviewed** — all 12 tasks complete, a final whole-branch review, its fix wave, and one additional emergent Critical fix (see below) all landed and independently re-reviewed clean. Build/vet/gofmt/`go test ./... -race` all pass. The SDD execution ledger for this plan has been deleted (its job was done) — the record now lives in git history, starting at commit `545ab4f`.

The final fix wave's own re-review had caught a bug created by the interaction of two of its own fixes: making the media scanner tolerant of directory-walk errors, combined with now-correctly-enforced foreign keys, meant an unreachable root media directory (e.g. a dropped NAS mount) got misread as "the user deleted everything" — the scanner would prune the entire media library, and `ON DELETE CASCADE` would take every scheduled program with it. Silent, permanent data loss from a routine failure. This was surfaced to the user before merging (per the SDD skill's own rule for load-bearing findings found outside its normal fix-wave budget), fixed in commit `5c3b027` (skip pruning whenever the walk observed any traversal error; regression test `TestScanner_RootWalkErrorDoesNotPruneExistingItems`), and independently re-reviewed clean — no new breakage, existing prune/scan tests unaffected.

## Next step

Backend (Plan 1) is done. Next up per the plan's stated 4-plan breakdown: playback (direct-play/transcode), the frontend SPA, and Docker packaging — none of those plans have been written yet. Three Minor findings from the final review are parked as deferred, not blocking: transient-DB-error mapping to 404 in a couple of handlers, an undocumented `DurationSec<=0` skip condition, and a test-only stale-pooled-connection landmine.

## Key decisions made (see the design spec for full reasoning)

- **Backend:** Go, single static binary, `modernc.org/sqlite` (pure-Go, no CGO).
- **Frontend:** not built yet — Plan 1 was backend-only by design (see the plan's intro for the full 4-plan breakdown: core backend → playback → frontend SPA → Docker packaging).
- **Database:** SQLite behind repository interfaces, swappable to PostgreSQL later without touching business logic.
- **Media source (MVP):** local filesystem only, including NAS/network shares via a Docker bind mount.
- **Metadata/subtitles:** no internet enrichment in MVP — filename + `ffprobe` technical metadata only.
- **Scheduling:** pure function of `(schedule, wall clock)`, recomputed on demand — no background ticking process, nothing lost on restart.
- **Media scanning:** manual rescan only for MVP, no filesystem watcher.
