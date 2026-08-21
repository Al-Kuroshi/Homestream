# Personal TV — Progress / Session Handoff

**Last updated:** 2026-08-21

This file exists so a new session (human or Claude Code) can pick up where the last one left off without re-reading the whole conversation history. It tracks *where we are*, not *what the product is* — for that, see the docs below.

## Where things live

- **Product requirements (canonical):** `docs/prd/HomeStreamer.md`
- **Older PRD drafts (superseded, kept for history):** `docs/prd/archive/`
- **Technical design spec:** `docs/design/2026-08-21-personal-tv-design.md`
- **Repo guidance for Claude Code:** `CLAUDE.md`

## Status

No source code exists yet. Work so far has been requirements and architecture, done via the `superpowers:brainstorming` process (architectural path).

1. ✅ PRD written and consolidated into a single canonical, human-readable document (`docs/prd/HomeStreamer.md`).
2. ✅ Technical design walked through interactively and written to `docs/design/2026-08-21-personal-tv-design.md`. Self-review checklist passed (no placeholders, internally consistent, appropriately scoped, no ambiguity found).
3. ⏳ **Waiting on user review of the design spec** before proceeding.
4. ⬜ Not started: implementation plan (via the `superpowers:writing-plans` skill — this is the next step once the spec is approved).
5. ⬜ Not started: any actual code.

## Key decisions already made (see the design spec for full reasoning)

- **Backend:** Go. Chosen over Node/TS once the deployment model became "self-hosted, one instance per user" rather than multi-tenant — single static binary and small Docker image matter more than dev-velocity/ecosystem when cost-per-instance is a non-issue.
- **Frontend:** React + TypeScript SPA, embedded into the Go binary via `go:embed`. Ships as one binary/one container; dev workflow (Vite + Go running side by side) is unaffected by embedding.
- **Database:** SQLite, accessed only through repository interfaces, specifically so it can be swapped to PostgreSQL later without touching business logic.
- **Media source (MVP):** local filesystem only, including NAS/network shares reached via a Docker bind mount — the app never knows or cares what's backing the mounted path. Cloud storage is deferred but the `Source` interface is the seam for adding it later.
- **Metadata/subtitles:** no internet enrichment in MVP (filename + `ffprobe` technical metadata only). Deferred via a `MetadataProvider` interface (no-op default) to avoid the real scope increase (async jobs, API keys, fuzzy matching, caching) and to keep the MVP fully local-first.
- **Playback:** conditional — direct-play via HTTP range requests when the source codec is already browser-compatible, `ffmpeg`-transcode to HLS (seeking to the right offset) when it isn't. This maximizes compatibility without paying transcode CPU cost for files that don't need it.
- **Streaming model:** fully lazy/on-demand. Nothing streams or transcodes for a channel with no viewers; "what's on now" is a pure function of `(schedule, wall clock)`, recomputed on demand rather than tracked by a running background process. Multi-viewer session sharing on the same channel is deferred.
- **Live UI updates:** client-side polling. Frontend computes progress locally from wall-clock time and periodically re-fetches the schedule — no WebSocket/SSE, since it's one-way, low-frequency data.
- **Media scanning:** manual rescan only for MVP. No filesystem watcher (unreliable over SMB/NFS) and no periodic background scan.
- **Docs style:** rejected using ASD-STE100 (Simplified Technical English) for the PRD or design specs — it's built for step-by-step procedural manuals and fights against the rationale/tradeoff-heavy prose those docs need. Worth revisiting only for a future install/runbook-style doc.

## Open items / not yet decided

- Nothing is currently blocking except the user's review pass on the design spec.
- Data Flow / Error Handling / Testing sections of the design were walked through in chat and are captured in the spec — no outstanding gaps identified.

## Next step

Once the user confirms the design spec is good: invoke the `superpowers:writing-plans` skill to turn it into a concrete implementation plan (no code should be written before that plan exists and is reviewed).
