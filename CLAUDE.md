# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

The core backend is implemented: Go module `personaltv` (Go 1.22+), SQLite data layer, local-filesystem media scanner, pure scheduling logic, channels service, and a REST API (`internal/db`, `internal/model`, `internal/repository`, `internal/mediastore`, `internal/scheduler`, `internal/channels`, `internal/api`, wired in `cmd/personaltv`).

The frontend also now exists: a React + TypeScript SPA in `web/` (Vite-scaffolded) with all 5 MVP screens complete (Sources, Library, Channels, Channel Schedule, Guide), talking to the backend exclusively through `web/src/api/*`. It's embedded into the Go binary at build time via `go:embed` (`web/embed.go`) and served for any non-`/api`, non-`/healthz` path (`internal/api/router.go`), so a single Go binary serves both the API and the UI in production.

The playback backend is also implemented: `internal/playback` (direct-play compatibility matrix, media path resolution, an `ffmpeg`-based HLS `SessionManager` with idle-timeout cleanup, tune-in orchestration) plus three REST endpoints — `POST /api/channels/{id}/watch`, `GET /api/media/{id}/stream`, `GET /api/playback/sessions/{id}/{file}` — wired into `cmd/personaltv/main.go`. `PERSONALTV_SESSIONS_DIR` (default: a subdirectory under the OS temp dir) configures where transcode sessions write their HLS segments.

The fifth and final MVP frontend screen, **TV** (`web/src/screens/TVScreen.tsx` at `/tv/:channelId`, `TVIndexScreen.tsx` at `/tv`), is also implemented — the app's five screens (TV, Guide, Library, Channels, Settings) are all now built. It's an event-driven player (no polling loop — a single self-scheduled `setTimeout` per tune-in) consuming the playback endpoints above: `useTuneIn` (`web/src/api/playback.ts`) drives `VideoPlayer` (direct `<video>` playback, or `hls.js` for transcoded sessions), `Interstitial` (off-air/unavailable with a next-up countdown, never silently skipping ahead), `NowPlayingOverlay` (auto-hiding), and `ChannelSwitcher` (prev/next + a channel-list overlay). **No `<video>` element in this codebase has ever been exercised in a real browser** — the implementation environment had none available, so verification stopped at type-checking, jsdom-mocked component tests, and an API-level contract smoke test against the real Go binary. A manual browser pass is still owed before this is considered fully done (see `docs/PROGRESS.md`'s Next step).

**Recurring weekly slot-chain scheduling is implemented** (design spec: `docs/design/2026-08-26-recurring-slot-scheduling-design.md`). The `Program` model is gone, replaced end to end by `Slot` — see "Core domain model" below for the mental model you need before touching scheduling code. On the frontend, `ChannelScheduleScreen` (`/channels/:id`) is a drag-and-drop weekly grid (drag media in from the library panel, drag existing slots to move them, drop a Gap/Break entry, toggle recurring vs one-off per placement, delete via each block's × button). **None of that drag-and-drop has been exercised in a real browser either** — same caveat as the TV screen below; a manual pass is owed (see `docs/PROGRESS.md`).

**Docker packaging is implemented** (design spec: `docs/design/2026-08-27-docker-packaging-design.md`, plan: `docs/superpowers/plans/2026-08-27-docker-packaging.md`). A 3-stage `Dockerfile` builds the frontend (`npm ci && npm run build`), cross-compiles the Go binary (`CGO_ENABLED=0`, `TARGETOS`/`TARGETARCH`-aware for multi-arch builds), and assembles a `debian:bookworm-slim` runtime image with `ffmpeg`/`ffprobe`/`curl` installed and a non-root user. `docker-compose.yml` wires a read-only bind mount for the user's media library, a named volume for the SQLite DB, and a configurable host port via `PERSONALTV_PORT` (see `.env.example`). A top-level `README.md` documents the Docker quick-start. Only `linux/amd64` has actually been run and verified; ARM64 is a documented, accepted out-of-scope item (see the design spec) rather than a known bug.

`ffmpeg`/`ffprobe` must be installed and on `PATH` to build the mental model of and to run this repo's tests (several tests generate short synthetic videos with `ffmpeg` and probe them with `ffprobe`).

```bash
go build ./...                  # build
go vet ./...                    # lint
gofmt -l .                      # format check (empty output = clean)
go test ./...                   # test
go test ./... -race             # test with race detector (use before merging/finishing a branch)
go test ./internal/mediastore/... -run TestScanner -v   # run a single package/test pattern
```

Frontend commands (run from `web/`):

```bash
cd web && npm install           # install dependencies
npm test                        # test (Vitest)
npm run build                   # type-check + production build into web/dist
npm run lint                    # lint (oxlint)
npm run dev                     # dev server; proxies /api/* to a Go backend on :8080 (run `go run ./cmd/personaltv` alongside it)
```

**Build order matters for a real production binary.** `web/embed.go` embeds
whatever is in `web/dist` at Go build time. On a fresh clone, `web/dist`
holds only tracked placeholder files (`.gitkeep`/`.gitignore`), so `go build`
still succeeds but embeds a placeholder, not the real UI. Run `npm run build`
in `web/` first, *then* `go build`/`go run` at the repo root, to get a binary
that serves the real SPA. `web/embed_test.go`'s tests skip automatically
(with a clear message) when `web/dist` isn't built, rather than failing or
passing vacuously.

A `Dockerfile`/`docker-compose.yml` setup now exists at the repo root (see "Project status" above) and is the intended deployment method (see below); it still expects the same `npm run build` → `go build` order internally (the Dockerfile's frontend stage runs `npm run build` before the Go build stage compiles the binary), so the build-order note above applies inside the image too, not just to a local `go build`.

## Testing conventions

Tests live next to the code they test, as `*_test.go` files in the same
package directory — standard Go convention, no separate `/tests` directory.
One exception: `internal/integration/end_to_end_test.go` is a dedicated
package holding a single full-stack test that drives the system through
real HTTP calls, rather than testing one layer in isolation.

- **Black-box by default.** Test files use an external test package
  (`db_test`, `sqlite_test`, `api_test`, `channels_test`, `scheduler_test`,
  `integration_test`) and only exercise the exported API, like a real
  caller would. The one exception is `internal/mediastore`, whose two test
  files use the internal `mediastore` package so `probe_test.go` and
  `scan_test.go` can share the unexported `generateTestVideo` helper.
- **Shared setup helpers within a package, not copy-pasted boilerplate.**
  `internal/db/testhelper.go` exports `db.OpenTest(t)` for a fresh,
  migrated, auto-cleaned-up SQLite DB — every other package's tests call
  it rather than reimplementing DB setup. `internal/api`'s handler tests
  similarly share `newTestServer`/`newTestServerWithConn` across files.
- **`generateTestVideo` is intentionally duplicated** (in `internal/mediastore`
  and `internal/integration`) rather than shared, because Go doesn't let
  test helpers export across packages via `_test.go` files. Not an
  oversight — do not "fix" this by trying to factor it into a shared
  non-test package.
- **Real dependencies, not mocks.** Tests hit a real (temp-file) SQLite DB
  and shell out to real `ffmpeg`/`ffprobe` rather than mocking either —
  closer to integration tests than strict unit tests, a deliberate
  tradeoff that catches real driver/schema/subprocess issues at the cost
  of needing `ffmpeg` on `PATH` to run the suite.
- **Tests are written test-first.** New work in this repo should keep
  following that: write the failing test, confirm it fails, then
  implement.

The above is specifically about the Go backend's tests. The frontend
(`web/`) follows the equivalent spirit in its own idiom: tests are colocated
with the code they test (`Foo.tsx` / `Foo.test.tsx`), black-box by default
(rendering components and asserting on what a user would see), and mock the
API at the network boundary with MSW rather than mocking `web/src/api/*`
calls directly.

## What this product is

Personal TV (a.k.a. "HomeStreamer") is an open-source, self-hosted platform that turns a user's local media library into configurable, scheduled virtual TV channels with an electronic program guide (EPG) — the experience of "pick a channel and watch what's on" rather than browsing a file library.

Full requirements live in `docs/prd/HomeStreamer.md`. Read it before starting implementation work — it is the source of truth for scope. Key points future work must respect:

## Core domain model

The scheduling engine is the product's core differentiator and must stay independent of the UI:

```
Channel → Slot (recurring or one-off) → resolution → Media
```

- **Media** — a playable item (movie, episode, video) sourced from the user's local filesystem. Represented abstractly (`title`, `duration`, `type`, `source`, `metadata`) so the scheduler never depends on where media came from.
- **Channel** — a virtual TV station with its own independent schedule; users create/rename/delete/enable/disable channels.
- **Slot** (`model.Slot`, table `slots`) — the single scheduling primitive. There is no `Program` model any more; recurring weekly scheduling is **implemented**, not just designed (`docs/design/2026-08-26-recurring-slot-scheduling-design.md`). A slot is either:
  - **recurring** (`recurring = true`): addressed by `day_of_week` (0 = Sunday, matching `time.Weekday()`/`Date.getUTCDay()`) plus a sparse integer `position`. It has no start time of its own — the day's recurring slots are walked in `position` order and their start times are computed by cumulative duration from that day's UTC midnight. Inserting or moving one therefore *reflows* every later slot on that weekday.
  - **one-off** (`recurring = false`): addressed by an absolute `start_time`, as the old `Program` was.

  Orthogonally, a slot's `kind` is either `media` (references a `media_item_id`; its duration is the item's duration) or `gap` (references no media at all; `gap_duration_sec` + `gap_label` describe a deliberate scheduled break). A gap resolves with `media_item_id` 0, so every consumer must branch on `kind` rather than trying to look the media up — see `resolvedSlotResponse`/`programStateJSON` and `joinResolvedSlotsWithMedia`.
- **Resolution** — turning slots into concrete occurrences for a date or window. `channels.ResolveDate` resolves one UTC calendar day; `channels.Service.ResolvedWindow` walks a range of days and backs `GET /api/channels/{id}/slots/resolved`, which the Guide and the weekly grid both consume. Resolution feeds the unchanged `internal/scheduler`, which still evaluates a flat list of `scheduler.ScheduledProgram` against a wall-clock instant — so "what's on now" remains a pure function of `(schedule, now)` with nothing cached or ticking. Playback's `TuneIn` (`internal/playback/tunein.go`) resolves through `ResolvedWindow` too; anything that bypasses it silently cannot play recurring-slot content.
- **All day/midnight/weekday arithmetic is UTC**, deliberately (an approved global constraint). The Guide and TV screens render clock *times* in the viewer's local timezone, so for a non-UTC viewer a slot can appear under a different day/time there than in the weekly grid. The grid labels itself "(all times UTC)"; see `docs/PROGRESS.md` for the accepted-limitation note.

REST surface: `GET/POST /api/channels/{id}/slots`, `GET /api/channels/{id}/slots/resolved?from&to`, `GET/PUT/DELETE /api/slots/{id}`. `PUT` is a **full replace** — a partial body silently drops the fields it omits.

At any point the system must be able to answer, per channel: what's playing now, when did it start/end, and what plays next (and after that).

## Architectural layering

Maintain clear separation between these layers; the UI must consume the API rather than reach into scheduling/media internals directly:

```
Media Management → Scheduling → Channel State → Playback → API → Clients
```

- Media source access should go through an abstraction (local filesystem is the only MVP source) so future sources (Plex, Jellyfin, network streams, etc.) can be added as adapters without touching the scheduler.
- The API is the contract for the browser-based MVP client and any future clients (mobile, smart TV, desktop) — design endpoints for media sources, media items, channels, slots, resolved schedules, and current playback/EPG state.
- Docker Compose is the intended local deployment method from the start.

## MVP scope discipline

This PRD explicitly restricts scope — do not add these unless a future spec explicitly requests them: streaming-service integrations (Netflix/Hulu/YouTube/etc.), cloud media storage, torrent/piracy-site functionality, DRM circumvention, media downloading, social/sharing features, mobile/smart-TV apps, recommendation systems, AI-generated schedules, advanced auth, payments/billing, or cloud infrastructure. Authentication is optional for the local-only MVP but the architecture shouldn't preclude adding it later.

When implementation details are ambiguous: check `docs/prd/HomeStreamer.md` first, prefer the simplest implementation consistent with it, don't expand scope without approval, and surface architecturally-significant decisions before implementing them rather than guessing.
