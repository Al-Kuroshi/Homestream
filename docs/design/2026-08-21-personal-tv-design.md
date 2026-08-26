# Personal TV — Technical Design

**Status:** Draft
**Date:** 2026-08-21
**Scope:** MVP, per `docs/prd/HomeStreamer.md`

## 1. Purpose

This document translates the product requirements in `docs/prd/HomeStreamer.md` into a concrete technical design for the MVP: a single-binary, self-hosted application that turns a local media library into scheduled virtual TV channels with an EPG. It covers the stack, architecture, components, data flow, error handling, and testing strategy. It does not cover implementation steps — that comes next, via an implementation plan.

## 2. Deployment Model

Self-hosted, one instance per user/household (not multi-tenant). Deployed as a single Docker container built from a single Go binary. Media lives on storage the user already controls — the same machine, or a NAS/network share mounted into the container via a Docker bind mount (`/host/media/path:/media`). This covers all combinations called out during design: Docker on Linux with local storage, Docker on Windows with storage on the Windows host, and Docker on any host with storage on a NAS or another machine on the network — in every case the app just reads from a configured path inside the container; it has no awareness of what's backing that path.

Cloud storage (S3-compatible, etc.) is an explicit non-goal for the MVP (`docs/prd/HomeStreamer.md` §8, §14), but the media-source layer (§4.1 below) is built as an interface specifically so a cloud adapter can be added later without touching the scheduler or playback layers.

## 3. Stack

| Layer | Choice | Why |
|---|---|---|
| Backend | Go | Single static binary, minimal runtime footprint, good fit for self-hosted deployment on modest hardware (NAS, small VPS, Raspberry Pi). ffmpeg is an external subprocess either way, so language choice doesn't affect transcoding cost — the deciding factor is deployment simplicity and per-instance resource footprint. |
| Frontend | React + TypeScript, built as a static SPA and embedded into the Go binary via `go:embed` | Rich component/state tooling for the genuinely interactive parts (EPG grid, drag-drop schedule editing, live progress), while still shipping as one binary/one container. Dev workflow (Vite dev server proxying `/api` to Go) is unaffected by embedding — it's a release-time step, not a dev-time one. |
| Database | SQLite, accessed only through repository interfaces | Zero-configuration, single-file, fits the single-container story with no extra service. Designed to be swappable to PostgreSQL later (see §4.4) without touching business logic. |
| Media transcoding | ffmpeg (external subprocess), invoked conditionally | See §4.3. |
| API style | REST, JSON | Matches PRD §18: the API is the contract for the web client and any future client (mobile, smart TV, desktop). |

## 4. Components

### 4.1 `mediastore`

Owns media discovery and access. Defines a `Source` interface (`List`, `Open`, `Stat`) so the rest of the system never assumes "local file" directly — for the MVP, the only implementation is a local-filesystem source that walks configured directories. On scan, each discovered file is probed with `ffprobe` for technical metadata (duration, video codec, audio codec, container) and persisted as a `MediaItem`. Duration from this probe is required — it's what the scheduler uses to compute program end times (PRD §6, §11).

Title/descriptive metadata beyond what can be derived from the filename is out of scope for the MVP (deferred per design discussion — see §9). This is handled through a separate `MetadataProvider` interface with a no-op/filename-based default implementation, so richer metadata (posters, descriptions, subtitles) can be added later as an alternate implementation without changing `mediastore`'s core scan/discovery logic.

### 4.2 `channels`

CRUD for channels, schedules, and programs. A channel has a name, description, optional artwork, an enabled/disabled flag, and an ordered position. A schedule is an ordered sequence of programs, each referencing a `MediaItem` with an explicit start time; end time is derived, not stored, since it's always computable from start time + media duration. **This "explicit start time" model is being superseded** by `docs/design/2026-08-26-recurring-slot-scheduling-design.md` (approved, not yet implemented) — recurring slots are addressed by day-of-week + position instead, with clock time computed rather than stored; one-off slots keep an explicit start time as described here.

### 4.3 `scheduler`

Pure domain logic with no I/O: given a channel's schedule and a point in time, computes the current program, its start/end time, the playback offset into it, and what plays next. Because this is a pure function of `(schedule, now)`, it needs no persistent runtime state — nothing is lost on an application restart, and nothing needs to be "ticking" for a channel that has no viewers (PRD §20 reliability; design decision to keep streaming fully lazy, see §4.4/§5).

### 4.4 `playback`

Manages tune-in sessions. When a client requests to watch a channel:

1. Ask `scheduler` for the current program and offset.
2. Probe the underlying file's codec/container (cached from the scan-time `ffprobe` result) against a known browser-compatibility matrix.
3. If compatible, serve the file directly via HTTP range requests, seeking to the computed offset — no CPU cost beyond disk I/O.
4. If not compatible (or in doubt), start an `ffmpeg` process transcoding to HLS beginning at that offset, and stream the resulting segments to the client.

Sessions are per-viewer and lazy: nothing streams or transcodes until someone actually tunes in, and the session is torn down when they leave. Sharing one transcode session across multiple simultaneous viewers of the same channel is a known future optimization, deliberately deferred — the PRD's future-scheduling and non-goals sections leave room for it without requiring it now.

### 4.5 `db`

SQLite behind repository interfaces (one per aggregate: channels, schedules, programs, media items). Business logic depends only on these interfaces, never on `database/sql` or SQL directly, so a PostgreSQL implementation can be added later by implementing the same interfaces against a different driver. Schema and migrations are kept portable (standard column types, no SQLite- or Postgres-only features) for the same reason.

### 4.6 `api`

REST/JSON HTTP handlers wrapping the packages above: media sources, media items, channels, programs, schedules, current playback/EPG state (PRD §18). The frontend consumes this API exclusively — it never reaches into the database or scheduler directly (PRD's architectural layering principle).

### 4.7 `web`

The embedded React/TS SPA, covering the five UI areas from PRD §12: **TV** (player, current channel/program, progress, next-up, channel switching), **Guide** (EPG grid), **Media Library** (browse/search/filter), **Channels** (create/edit/schedule), **Settings** (media sources, playback, application config). Live "now playing" state is computed client-side from the fetched schedule and wall-clock time (a pure countdown-style calculation), with the schedule re-fetched periodically to catch changes — no persistent connection (WebSocket/SSE) is used, since this is one-way, low-frequency status data.

## 5. Data Flow

**Scan.** User triggers a rescan (manual only for MVP — no filesystem watcher, no periodic background scan; PRD's MVP scope lists "rescan" as a user action) → `mediastore` walks the configured director(y/ies) → for each new or changed file (by mtime/size), `ffprobe` extracts duration/codec/container → persisted as a `MediaItem`. Unchanged files are skipped to keep rescans cheap on large libraries (PRD §11 performance).

**Tune-in.** Client requests to watch channel X → API asks `scheduler` for `(channel, now) → program, offset, next` → `playback` determines direct-play vs. transcode and starts streaming from that offset → client player begins mid-program, as if tuning into a live broadcast.

**Guide/EPG.** Client fetches each visible channel's schedule for a time window → frontend renders the grid and computes each program's progress locally from wall-clock time, re-fetching on an interval to pick up schedule changes.

## 6. Error Handling

- **Missing/deleted file at scheduled playback time:** `playback` skips to the next program in the schedule rather than failing the channel; the gap is logged. (PRD §11: "a single unavailable media item should not cause an entire channel to stop functioning.")
- **Unreadable/corrupt file at scan time:** `ffprobe` failure marks the item invalid and excludes it from scheduling; the rest of the scan continues.
- **ffmpeg failure mid-stream:** surfaced as an error to that viewer's session only; does not affect other viewers or the server process.
- **Application restart:** no recovery needed by design — current-program state is always recomputed from `(persisted schedule, wall clock)`, never held in memory (PRD §11, §20).

## 7. Testing Strategy

- **`scheduler`:** pure functions, no I/O — unit tests over fixed schedules against arbitrary timestamps (mid-program, exact start/end boundaries, gaps between programs, back-to-back programs).
- **`db` repositories:** tested against real SQLite (temp file or in-memory) through the repository interface, so the same suite can run against PostgreSQL later without rewriting tests.
- **`playback`:** the direct-play-vs-transcode decision is a pure function of probed codec info and is unit-tested directly; the actual `ffmpeg` subprocess invocation is integration-tested separately, not mocked.
- **`api`:** standard Go `httptest` handler tests.

## 8. Explicit Non-Goals (this design)

Carried forward from `docs/prd/HomeStreamer.md` §14 and reaffirmed during design: no cloud storage backend, no streaming-service integrations, no automatic metadata/subtitle fetching from the internet, no multi-viewer session sharing, no filesystem watching/automatic rescanning, no authentication, and no multi-tenant hosting. None of these are precluded by the architecture — each has a clear seam to extend into later (media source interface, metadata provider interface, etc.) — but none are built now.

## 9. Decisions Deferred Past MVP (with reasoning)

- **Metadata/subtitle enrichment from the internet:** deferred to keep the MVP local-first and avoid the real scope increase of async enrichment jobs, API key config, fuzzy title matching, and caching. A `MetadataProvider` interface exists so this can be added later as a new implementation, not a rearchitecture.
- **Cloud storage as a media source:** deferred per PRD §8; the `Source` interface in `mediastore` (§4.1) is the seam for adding it.
- **PostgreSQL:** deferred in favor of SQLite for MVP simplicity; the repository pattern (§4.5) is the seam for swapping it in.
- **Multi-viewer session sharing:** deferred; each viewer gets an independent playback session for MVP.
- **Automatic media discovery** (filesystem watcher or periodic rescan): deferred in favor of manual rescan only, to avoid the complexity and NAS/network-mount reliability issues (`inotify` often doesn't propagate over SMB/NFS) that automatic detection would introduce.
