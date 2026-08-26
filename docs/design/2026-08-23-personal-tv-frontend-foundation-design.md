# Personal TV — Frontend Foundation & Management Screens — Design Spec

**Status:** Approved by user in brainstorming session, ready for implementation planning.
**Supersedes:** nothing. Extends `docs/design/2026-08-21-personal-tv-design.md` (the backend/overall design spec) with a concrete frontend design.
**Depends on:** the core backend (`docs/plans/2026-08-21-personal-tv-core-backend.md`), fully implemented and merged to `main`.

## 1. Scope and decomposition

The PRD/overall design spec describe five frontend areas: **TV**, **Guide**, **Media Library**, **Channels**, **Settings**. This spec covers four of them — **Guide, Media Library, Channels, Settings** — plus the shared app shell (routing, layout, API client, build tooling). **TV** (the video player and channel-switching screen) is explicitly out of scope here.

**Why TV is deferred:** TV needs an actual playback endpoint (direct-play via HTTP range requests, or `ffmpeg`-transcode to HLS) to stream real video. That backend work ("Plan 2 — Playback," per the core-backend plan's own references) does not exist yet. The other four screens are pure CRUD/EPG-rendering against the REST API the backend already exposes (`/api/sources`, `/api/media`, `/api/channels`, `/api/channels/{id}/programs`, `/api/channels/{id}/now`) and have no dependency on playback. Building them now, and TV once playback lands, avoids blocking frontend progress on backend work that hasn't started, and avoids building a throwaway stub player now.

The app shell reserves a fifth navigation slot for TV so adding it later is additive, not a restructuring.

## 2. Architecture and tech stack

| Concern | Choice | Why |
|---|---|---|
| Framework | React + TypeScript | Matches the backend/overall design spec's existing stack decision. |
| Build tool | Vite | Fast dev server, straightforward static build for embedding. Confirmed by the overall design spec ("Vite dev server proxying `/api` to Go"). |
| Styling | Plain CSS (CSS modules or plain stylesheets per component), no UI framework | Matches the backend's minimal-dependency philosophy (pure-Go SQLite driver, no ORM). Full control over the Guide grid's layout, which doesn't map cleanly onto generic component-library primitives anyway. |
| Routing | React Router | Real URLs per screen (`/guide`, `/library`, `/channels`, `/settings`), working browser back/forward, bookmarkable/refreshable screens. |
| Server state / data fetching | TanStack Query (React Query) | Every screen here is "fetch from REST API, refresh periodically" — Query's polling, caching, request de-dup, and loading/error states remove a large amount of otherwise hand-rolled logic. |
| Testing | Vitest + React Testing Library, network mocked at the boundary (MSW) | Matches the backend's test-first discipline. Components/hooks are tested against a fake API, not against mocked internals. |

**Directory:** a new `web/` directory at the repo root (`.gitignore` already anticipates `/web/node_modules` and `/web/dist`).

**Dev workflow:** `vite` dev server (in `web/`) proxies `/api/*` requests to the Go backend (`go run ./cmd/personaltv`, port from the backend's existing config), so both run side by side during development with no CORS/proxy configuration needed in the browser.

**Production build:** `npm run build` in `web/` produces `web/dist`. A new Go package embeds `web/dist` via `go:embed` and serves it for any request that isn't `/api/*` or `/healthz`, so the shipped artifact stays one binary / one container, per the overall design spec's deployment model. `cmd/personaltv`'s router wiring gains this catch-all static-file route; no existing `/api/*` route changes.

## 3. App shell and navigation

Persistent left sidebar (chosen over a top nav bar during brainstorming — more TV-app-like, leaves full width for the Guide's wide timeline grid, and scales cleanly when TV is added later). Sidebar items: **Guide** (default/home route), **Library**, **Channels**, **Settings**. A reserved-but-unused slot exists in the sidebar's layout for **TV**, added later without restructuring.

Routes:
- `/guide` — Guide screen (default redirect from `/`)
- `/library` — Media Library screen
- `/channels` — Channels list
- `/channels/:id` — a channel's schedule editor
- `/settings` — Settings (media sources)

**API client:** a single typed module (e.g. `web/src/api/client.ts`) wrapping `fetch`, with one function per backend endpoint, typed request/response shapes matching the Go API's JSON contract. Every screen and every TanStack Query hook goes through this module — no raw `fetch` calls scattered through components. This is the frontend's enforcement of the architectural principle already stated in `CLAUDE.md`: the UI consumes the API, it never reaches into scheduling/media internals directly.

## 4. Screens

### 4.1 Guide

The default/home screen. A classic TV-guide timeline grid (chosen over a simpler "now/next list" during brainstorming, for the real cable-guide feel of seeing a whole time window across all channels at once):

- **Rows:** one per enabled channel (from `GET /api/channels`).
- **Columns:** a horizontally scrollable time axis. Default visible window: 1 hour before now to 5 hours after now (a UI parameter, not a product requirement — the implementation plan may adjust the exact hours without revisiting this spec).
- **Program blocks:** width proportional to duration, positioned by start time, labeled with the media's title.
- **"Now" indicator:** a vertical line at the current wall-clock position, recomputed client-side (no server round-trip needed to move the line).
- **Off-air gaps:** the backend's scheduler already supports non-contiguous schedules — `CurrentState.Current` is `nil` when "now" falls between two programs, a first-class "off air" state (see `internal/scheduler/scheduler.go`). The Guide grid renders an explicit **"Off Air"** block filling any gap on a channel's row within the visible time window: between two consecutive programs, before the first program (if the window starts before it), after the last program (if the window extends past it), and across the whole row for a channel with no programs at all in the window.

**Data:** `GET /api/channels` (channel list) + `GET /api/channels/{id}/programs` per channel (all programs, unbounded — no time-window API needed) + `GET /api/media` (once, cached) for titles. Joined client-side: each program's `end_time = start_time + media.duration_sec`, mirroring the PRD's "end time computed from duration" rule (already implemented server-side in `scheduler.ScheduledProgram.EndTime()`; the frontend performs the same trivial arithmetic, not a reimplementation of scheduling logic). The visible time window is a client-side filter over each channel's full program list — MVP schedules are small enough that fetching the whole list per channel and filtering client-side is simpler than adding a new time-windowed backend endpoint, and matches the "sequential playlist" MVP scope (not open-ended/infinite schedules).

**Polling:** programs/channels queries poll on an interval (default 30s) to catch schedule changes made elsewhere (e.g. in another browser tab on the Channels screen). The "now" line's position and each program's progress are computed purely from wall-clock time between polls — never fetched per-second.

### 4.2 Media Library

A sortable, searchable **text table** (title, duration, source, video codec, audio codec, container, invalid flag) over `GET /api/media`. Chosen over a thumbnail grid during brainstorming — the backend generates no thumbnails/poster art in MVP scope (ffprobe technical metadata only), and building thumbnail extraction is out of scope for this plan. The table's column design is deliberately left able to gain a leading thumbnail column later without restructuring, if/when thumbnail extraction becomes its own piece of work.

- **Search/filter:** client-side, over the full `GET /api/media` result set (title substring match; filter by source; filter to invalid-only).
- **Invalid items:** visually flagged (e.g. a badge/row style), matching the backend's `Invalid` field for files that failed `ffprobe`.
- No source-management actions here (add/rescan/remove source) — those live in Settings, which owns media sources. Media Library is purely for browsing/searching media *to use when building schedules*, matching the PRD's UI-area split (media source configuration → Settings; browsing discovered media → Media Library).

### 4.3 Channels

**List view** (`/channels`): every channel (`GET /api/channels`) with name, enabled/disabled toggle, and reorder controls (backed by the `position` field already in the `Channel` model); create a new channel (name only, minimal fields); rename and delete from the same list.

**Schedule editor** (`/channels/:id`): a channel's ordered list of programs (`GET /api/channels/{id}/programs`, joined with media titles as in the Guide), each row showing media title, start time, and computed end time.

- **Add a program:** a media picker (search/select from `GET /api/media`) plus a start-time input, `POST`ed to `/api/channels/{id}/programs`.
- **Remove a program:** `DELETE /api/programs/{id}`.
- **Edit a program's start time:** `PUT /api/programs/{id}`.
- **Reordering / gaps:** ordering is implicit in each program's `start_time`, not a separate position field — the list is simply sorted by start time (matching `ProgramRepository.ListByChannel`'s existing `ORDER BY start_time`). The start-time input is free-form: nothing forces or suggests contiguous back-to-back scheduling, so a gap (off-air period) is just "the next program's start time is later than the previous program's end time" — no special UI affordance is needed to create one, it falls out naturally from free-form start times.
- **Not built in this plan:** drag-and-drop reordering (chosen against during brainstorming in favor of the simpler add/remove/edit-start-time list — drag-drop could be added later as a pure UI enhancement without changing the underlying data flow, since ordering is already purely derived from `start_time`).
- **Superseded:** the assumption that drag-and-drop is "a pure UI enhancement without changing the underlying data flow" turned out to be wrong once recurring scheduling entered scope — see `docs/design/2026-08-26-recurring-slot-scheduling-design.md`, which replaces this whole schedule-editor section (`Program`/`start_time`-only model, dropdown-based add form) with a recurring slot-chain model and a drag-and-drop weekly timeline. This section is kept for history; follow the newer spec for anything scheduling-related.

### 4.4 Settings

Scoped to **media sources only** for this plan (confirmed during brainstorming — "playback" and "application/system" settings from the PRD's UI-area list have nothing real to configure yet: no playback backend, no auth, no other app-level config exists). Deliberately not building empty placeholder sections for those.

- List sources (`GET /api/sources`): name, path, item count — derived client-side by filtering the already-fetched `GET /api/media` result set by `source_id`, no backend change needed.
- Add a source (`POST /api/sources`): name + path form.
- Rescan a source (`POST /api/sources/{id}/scan`): triggers a scan, shows loading state, refreshes the Media Library's query on completion.
- Remove a source (`DELETE /api/sources/{id}`): with a confirmation step, since this cascades to delete the source's media items (and, per the schema's `ON DELETE CASCADE`, any programs referencing them) — the same class of destructive action the backend's own scanner bug (fixed in `5c3b027`) was about, so the frontend should not make this a single accidental click.

## 5. Data flow and state management

TanStack Query owns all server state. One query per endpoint+params (e.g. `useChannels()`, `useProgramsForChannel(id)`, `useMediaItems()`, `useSources()`). Mutations (`useMutation`) invalidate the queries they affect — e.g. adding a program invalidates that channel's programs query and the Guide's aggregate data; rescanning a source invalidates the media-items query.

No global client-side state store (Redux/Zustand/etc.) — TanStack Query's cache is the state layer for server data, and the small amount of local UI state (form inputs, selected time window on the Guide) stays in component state via `useState`/`useReducer` where it's used.

## 6. Error handling

Query errors surface as an inline error banner on the screen/component that hit them — not a global crash. A route-level error boundary is the last-resort safety net for unexpected render errors, not the primary error-handling path. Mutation failures (add/remove/rescan/create/delete) show an inline error message near the action that failed and leave form state intact (the user's typed input isn't lost on a failed submit). This mirrors the backend's own principle (PRD §11, design spec §6) that a single failure must not take down more than the thing that failed.

## 7. Testing

Vitest + React Testing Library, test-first, matching the backend's TDD discipline (`CLAUDE.md`'s Testing Conventions section). Network is mocked at the boundary with MSW rather than mocking components or the API client module — tests render real components against a fake API server and assert on rendered behavior (e.g. "given these programs and this media list, the Guide renders these blocks at these time positions, with an Off Air block in this gap"; "clicking Rescan calls `POST /api/sources/{id}/scan`, shows a loading state, then refreshes the table"). Colocated `*.test.tsx`/`*.test.ts` files next to the code they test, following the backend's colocated-test convention.

## 8. Out of scope (this plan)

- TV / video player screen (blocked on playback backend — see §1)
- Thumbnail/poster art for media items (no backend support; Media Library's table leaves room for it later)
- Drag-and-drop schedule reordering (deferred enhancement — see §4.3; now designed in `docs/design/2026-08-26-recurring-slot-scheduling-design.md`, which supersedes §4.3's schedule editor)
- Any playback/application/system settings (nothing to configure yet — see §4.4)
- Any new backend API endpoints (this plan is buildable entirely against the existing REST API)
- Authentication (explicit MVP non-goal per the PRD)

## 9. Open questions

None outstanding — all decisions in this spec were confirmed during the brainstorming session that produced it.
