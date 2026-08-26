# Personal TV — Recurring Slot-Chain Scheduling & Drag-and-Drop Timeline — Design Spec

**Status:** Approved by user in brainstorming session, ready for implementation planning.
**Supersedes:** the scheduling portions of `docs/design/2026-08-23-personal-tv-frontend-foundation-design.md` §4.3 (Channels screen's dropdown-based schedule editor) and its §8 note deferring "drag-and-drop schedule reordering." That earlier note was about reordering an absolute-start-time list; this spec replaces the underlying model, not just the UI, so it supersedes rather than extends that section.
**Depends on:** the existing core backend and frontend foundation, both merged to `main`.
**Relationship to the PRD:** `docs/prd/HomeStreamer.md` §7 lists "recurring programs" as an explicit future capability, not required for the MVP. This spec is that future capability, now explicitly requested and scoped by the project owner. It does not touch any of the PRD's other MVP-non-goals.

## 1. Scope and decomposition

The project owner asked for a drag-and-drop weekly scheduling timeline. Discussion surfaced that the actual desired behavior is a deeper model change: schedules should be **recurring by default** (a slot repeats every week unless marked one-off), and each channel's day should be a **sequential chain of duration-sized slots** rather than a list of manually-chosen absolute start times. That's the real scope of this spec.

Two related ideas came up and are **explicitly deferred to their own future specs**, not built here:

- **TV series/episode auto-advance** (a recurring slot bound to a series plays the next episode each time it airs). Deferred because there is currently no series/season/episode concept anywhere in the media model — every scanned file is an independent, flat `MediaItem`. Building this now would be speculative without real series content to design against.
- **YouTube-sourced slots** (trailers, etc., played via the official YouTube IFrame embed player). Deferred because it requires a new media-source-type abstraction and an entirely separate playback engine from the existing direct-play/HLS pipeline — unrelated in kind to the scheduling model change this spec covers.

Also explicitly **not** part of this spec (decided during brainstorming, see §6 and §8):

- Per-occurrence exceptions to a recurring slot (e.g., "skip just this one Monday"). Editing or deleting a recurring slot always affects every future occurrence.
- A configurable "broadcast day" anchor. Each day is midnight-to-midnight for this spec; a custom anchor (e.g., 6 AM–6 AM) is a possible future refinement, not required now.
- Cover-art/poster fetching. The frontend gains an optional image field in its slot display model now (see §7), but nothing populates it yet — it always falls back to text until a future metadata-fetching spec exists.

## 2. Domain model: the Slot

`Program` is replaced by a broader concept, **Slot**, covering everything that can occupy time on a channel:

- **Media slot** — references a `MediaItem`; duration is the file's actual runtime (unchanged from today).
- **Gap slot** — no media reference; a user-chosen duration and a free-text label (e.g., "Ad Break," "Intermission"). This is new: today, gaps are only ever an *implicit absence* between programs. Gap slots make a deliberate pause a first-class, schedulable thing.

Independently, every slot is either:

- **Recurring** (the default, matching "unless we specify... it should not recur, it should always be recurring"). Addressed by `(channel, day_of_week, position)` — never an absolute timestamp. Its clock time is *computed*, not stored: the sum of the durations of every recurring slot at a lower `position` on the same channel and day, starting from midnight. Repeats every week, indefinitely, until deleted. Editing or deleting always affects every future week — there is no per-occurrence override (§6 confirmed this explicitly, to keep the data model to one row per recurring slot with no separate exceptions table).
- **One-off**. Addressed by an absolute `(channel, start_time)` — structurally identical to how `Program.start_time` works today. Exists for exactly one occurrence and never repeats. Because recurring slots can't be overridden per-occurrence, a one-off slot **may only be placed into a genuinely empty gap** on its date (never overlapping a recurring slot's resolved occupancy for that day) — if you want to permanently change what airs in a recurring slot, you edit the recurring slot itself, not layer a one-off on top of it.

This dual addressing is deliberate: it lets one-off slots reuse today's existing absolute-time model and validation almost unchanged, while recurring slots are the one genuinely new addressing scheme. It also resolves the tension between "always recurring by default" and "no per-occurrence exceptions" — one-off is for *new, additional, single-occurrence* content in an open gap, not for editing what a recurring slot plays on one particular week.

**Day boundary rule** (confirmed in brainstorming): a day is a hard midnight-to-midnight window. A slot — recurring or one-off — is **rejected at creation/insertion time** if it (or the reflow it causes; see §4) would push any slot's end past midnight. Days do not spill into each other. Trailing time after the last slot of a day, up to midnight, stays implicit off-air, same as today — you are never forced to fill a whole day.

## 3. Data model (backend)

`programs` table → `slots` table, replacing `internal/db/migrations/0001_initial_schema.sql`'s `programs` definition (or added as a new migration, per the implementation plan's judgment call — this spec doesn't mandate in-place vs. new-migration, just the resulting shape):

```sql
CREATE TABLE slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,                 -- 'media' | 'gap'
    media_item_id INTEGER REFERENCES media_items(id) ON DELETE CASCADE,  -- required iff kind='media'
    gap_duration_sec REAL,              -- required iff kind='gap'
    gap_label TEXT NOT NULL DEFAULT '', -- optional, only meaningful iff kind='gap'
    recurring INTEGER NOT NULL DEFAULT 1,
    day_of_week INTEGER,                -- 0-6, required iff recurring=1
    position INTEGER,                   -- ordering key within (channel_id, day_of_week), required iff recurring=1
    start_time TEXT,                    -- absolute timestamp, required iff recurring=0
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_slots_channel_id ON slots(channel_id);
CREATE INDEX idx_slots_channel_recurring_day ON slots(channel_id, day_of_week) WHERE recurring = 1;
```

`model.Program` → `model.Slot` with the fields above (Go's `database/sql` nullable-field handling — `sql.NullInt64`/pointers — for the fields that are conditionally required, matching how this codebase already handles optional columns elsewhere). `ProgramRepository` → `SlotRepository`, gaining query methods scoped to `(channel_id, day_of_week)` for recurring slots in addition to the existing `ListByChannel`.

**Existing data:** the 6 test programs created earlier in this session map directly onto one-off slots (`recurring=0`, existing `start_time` carried over unchanged, `kind='media'`) — no data loss, no special migration logic beyond the schema/column rename.

**Position values:** stored as sparse integers (e.g., increments of 1000) so most inserts-between-existing-slots don't require renumbering every later slot — an implementation detail for the plan stage, not a decision this spec needs to pin down further.

## 4. Scheduler: resolution, not a rewrite

This is the reassuring part: **`internal/scheduler.Evaluate` does not change.** It's already a pure function over `[]ScheduledProgram` (concrete `{StartTime, Duration}` pairs) — it doesn't know or care whether those came from one absolute-time row or a resolved recurring rule.

What's new is the step that builds its input. Today, `channels.Service.CurrentState` does this directly (`internal/channels/service.go`): list every `Program` row for a channel, unbounded, map each to a `ScheduledProgram`, call `Evaluate`. That direct mapping is replaced by a resolution function:

```go
// ResolveDate returns the concrete ScheduledPrograms that occupy the given
// channel on the given calendar date: every recurring slot whose day_of_week
// matches, walked in position order with times computed by cumulative
// duration from midnight, plus every one-off slot dated to that day.
func ResolveDate(slots []model.Slot, mediaByID map[int64]*model.MediaItem, date time.Time) []scheduler.ScheduledProgram
```

- `CurrentState(channelID, now)`: calls `ResolveDate` for **today through some lookahead window** (e.g., today + the next 7 days), concatenates the results, and feeds the whole set into the unchanged `Evaluate`. The lookahead is necessary so "what plays next" can see into tomorrow once today's chain is exhausted — `Evaluate` already handles an arbitrary mix of dates correctly, it just needs to be given them.
- The **Guide screen** and the **new weekly timeline UI** both need "what's resolved between date A and date B" — this is the same `ResolveDate` function called once per day in the range. This replaces the Guide's current approach of fetching *all* programs unboundedly and filtering client-side (which assumed a finite list — recurring slots make the naive "list" infinite, so resolution must happen for a bounded window instead, and that's naturally a backend concern, not a client-side filter).

Because `ResolveDate` is a pure function over data already in memory (no I/O), it's straightforward to unit-test exhaustively — empty day, one recurring slot, several slots filling a day, a one-off slot in a gap, a day with only one-offs, etc. — independent of the HTTP/DB layers, matching this repo's existing test conventions.

## 5. Validation

Enforced server-side (not just in the UI, so the API itself can never reach a contradictory state — matching this codebase's general philosophy of not trusting the client alone):

- **Recurring slot insert/update:** resolve its day's recurring chain with the new/moved slot in place; reject if any slot's computed end time exceeds midnight.
- **One-off slot insert/update:** call `ResolveDate` for its target date (which already includes that day's recurring occupancy), and reject if the requested `start_time`..`end_time` window overlaps any resolved slot, or spills past midnight.
- Both cases return a clear error the frontend surfaces inline (matching this repo's existing mutation-error-handling pattern, `MutationError`) — "doesn't fit here" rather than a generic failure.

## 6. API changes

- `GET/POST /api/channels/{id}/programs`, `PUT/DELETE /api/programs/{id}` → renamed to `.../slots` (mirroring the model rename), with the new fields (`kind`, `recurring`, `day_of_week`, `position`, `gap_duration_sec`, `gap_label`) and validation from §5.
- New: `GET /api/channels/{id}/slots/resolved?from=DATE&to=DATE` — returns the resolved (concrete-time) occurrences for that window, for the Guide and the new timeline UI to render directly. This is additive; it doesn't replace the raw slot CRUD endpoints above, which manage the underlying rules.
- `GET /api/channels/{id}/now` (existing endpoint) is unaffected in its contract — only its internal implementation changes to use `ResolveDate` per §4.

## 7. Frontend

**Channel Schedule screen** (`/channels/:id`): the dropdown-based add-program form is fully replaced by a 7-day drag-and-drop grid (one column per day, navigable to other weeks per the earlier "real calendar week" framing — though note recurring slots render identically every week since their time is computed from the rule, not a specific date; only one-off slots are week-specific).

- A media-library side panel lists available `MediaItem`s to drag onto a day column. Each entry (and each placed block on the grid) is represented primarily by a **cover-art icon**; since no backend image data exists yet, this always falls back to showing the title as text — the display model has an optional image field now so wiring in real cover art later (a future spec) is a data change, not a component rewrite.
- A "+ Gap" affordance creates a gap slot with a user-entered duration and optional label.
- Dropping a slot means "insert before/after this existing slot" (or "at the start/end of this day" if empty) — never picking a raw clock time. The grid shows the computed resulting times live and rejects (with an inline message, per §5) a placement that would spill past midnight.
- A recurring/one-off toggle appears when placing a slot, defaulting to recurring.

**Guide screen**: changes its data source from "fetch all programs per channel, filter client-side to a visible window" to calling the new resolved-window endpoint (§6) for whatever window it displays — the rendering logic (off-air-gap blocks, "now" line, etc.) is unaffected, only where the concrete program list comes from.

## 8. Testing

Following this repo's existing conventions (black-box, colocated, test-first):

- **Backend:** `ResolveDate` gets thorough pure-function unit tests in `internal/scheduler` (or a new package if the implementation plan judges the resolution logic belongs closer to `internal/channels` — a call for the plan, not this spec). Repository and API handler tests follow the existing patterns in `internal/repository/sqlite` and `internal/api`, extended for the new fields and the `/slots/resolved` endpoint. Validation rules (§5) get explicit test cases for each rejection reason.
- **Frontend:** the new timeline component is tested with Vitest/RTL/MSW per existing convention — rendering a resolved week's slots at correct positions/widths, drag-and-drop insertion triggering the right mutation, and inline rejection messages on invalid placements (mocked at the network boundary, consistent with how this repo already tests mutation error states).

## 9. Out of scope (this spec)

- TV series/episode auto-advance (§1)
- YouTube-sourced slots (§1)
- Per-occurrence exceptions to recurring slots (§1, §2)
- Configurable broadcast-day anchor other than midnight (§1)
- Cover-art/poster image fetching (§1, §7) — the display model supports an image slot, nothing populates it yet
