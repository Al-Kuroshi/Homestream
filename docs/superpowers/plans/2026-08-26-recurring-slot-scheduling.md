# Recurring Slot-Chain Scheduling & Drag-and-Drop Timeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `Program`/absolute-`start_time`-only scheduling model with a `Slot` model (recurring-by-default, day-of-week + position addressed; one-off slots keep absolute-`start_time` addressing) and a drag-and-drop weekly timeline UI, per the approved design spec.

**Architecture:** Backend: `Program` → `Slot` throughout (model, repository, service, API, routes). A new pure resolver (`ResolveDate`) expands a channel's slots into concrete `scheduler.ScheduledProgram` occurrences for a given calendar date; `scheduler.Evaluate` itself is untouched. Frontend: the Channels schedule editor becomes a 7-day drag-and-drop grid fed by a new resolved-window API endpoint; the Guide screen switches to the same endpoint instead of unbounded per-channel program lists.

**Tech Stack:** Go 1.22+ (existing `net/http` ServeMux, `modernc.org/sqlite`), React + TypeScript (existing Vite/TanStack Query/Vitest/RTL/MSW stack). Drag-and-drop uses the native HTML5 Drag and Drop API (`draggable`, `onDragStart`/`onDragOver`/`onDrop`) — no new dependency, consistent with this repo's minimal-dependency philosophy.

**Spec:** `docs/design/2026-08-26-recurring-slot-scheduling-design.md`

## Global Constraints

- Day-of-week integers are `0`–`6` with `0 = Sunday`, matching both Go's `time.Weekday()` and JavaScript's `Date.getDay()`/`Date.getUTCDay()` natively — no offset conversion anywhere.
- All day-boundary and day-of-week computation (backend and frontend) is done in **UTC**, matching this codebase's existing convention of storing/computing all timestamps in UTC (`time.Now().UTC()` throughout the backend; only `web/src/scheduling/time.ts`'s local-time functions are the deliberate, documented exception, and this plan does not touch those).
- Recurring slot positions are stored as sparse integers (increments of 1000) so most inserts don't require renumbering later slots.
- A day is a hard midnight-to-midnight (UTC) window. Both recurring and one-off slot validation reject a placement whose resolved occupancy would exceed 24 hours from that day's start (spec §2, §5).
- One-off slots may only be placed into time not already occupied by a resolved recurring slot on that date (spec §2) — enforced server-side in validation, not just the UI.
- Tasks 1-6 are one atomic backend migration arc (`Program` → `Slot` cascades through `model` → `repository` → `channels` → `api`): `go build ./...` is deliberately broken between Tasks 1 and 6 (each intermediate task's own "run tests" steps scope to the specific package(s) that task touches, which do pass in isolation) and is only required to be fully clean, repo-wide, at Task 6's end. From Task 6 onward, `go build ./...`, `go vet ./...`, `gofmt -l .`, and `go test ./... -race` must stay clean after every backend task. `cd web && npx tsc -b && npm run lint && npm test` must stay clean after every frontend task (Tasks 7-11).

---

## Task 1: `Slot` data model — migration + Go struct

**Files:**
- Create: `internal/db/migrations/0002_slots.sql`
- Modify: `internal/model/model.go`

**Interfaces:**
- Produces: `model.Slot` struct, `model.SlotKindMedia`/`model.SlotKindGap` constants — every later backend task depends on these exact field names and types.

- [ ] **Step 1: Write the migration**

```sql
-- internal/db/migrations/0002_slots.sql
DROP TABLE programs;

CREATE TABLE slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    media_item_id INTEGER REFERENCES media_items(id) ON DELETE CASCADE,
    gap_duration_sec REAL,
    gap_label TEXT NOT NULL DEFAULT '',
    recurring INTEGER NOT NULL DEFAULT 1,
    day_of_week INTEGER,
    position INTEGER,
    start_time TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_slots_channel_id ON slots(channel_id);
CREATE INDEX idx_slots_channel_recurring_day ON slots(channel_id, day_of_week) WHERE recurring = 1;
```

- [ ] **Step 2: Replace `Program` with `Slot` in the model**

In `internal/model/model.go`, replace the `Program` struct (currently the last struct in the file) with:

```go
const (
	SlotKindMedia = "media"
	SlotKindGap   = "gap"
)

type Slot struct {
	ID             int64      `json:"id"`
	ChannelID      int64      `json:"channel_id"`
	Kind           string     `json:"kind"`
	MediaItemID    *int64     `json:"media_item_id,omitempty"`
	GapDurationSec *float64   `json:"gap_duration_sec,omitempty"`
	GapLabel       string     `json:"gap_label"`
	Recurring      bool       `json:"recurring"`
	DayOfWeek      *int       `json:"day_of_week,omitempty"`
	Position       *int       `json:"position,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
```

- [ ] **Step 3: Verify the build fails everywhere `model.Program` was used (expected — later tasks fix each site)**

Run: `go build ./... 2>&1 | head -30`
Expected: FAIL with `undefined: model.Program` in `internal/repository/repository.go`, `internal/repository/sqlite/program_repository.go`, `internal/api/programs_handlers.go`, `internal/channels/service.go` — this is the expected, temporary state until Tasks 2, 4, and 6 fix each site. Do not fix them in this task.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/0002_slots.sql internal/model/model.go
git commit -m "feat: replace Program model with Slot (recurring + gap support)"
```

---

## Task 2: `SlotRepository` — interface + SQLite implementation

**Files:**
- Modify: `internal/repository/repository.go`
- Create: `internal/repository/sqlite/slot_repository.go`
- Create: `internal/repository/sqlite/slot_repository_test.go`
- Delete: `internal/repository/sqlite/program_repository.go`, `internal/repository/sqlite/program_repository_test.go`

**Interfaces:**
- Consumes: `model.Slot` (Task 1).
- Produces: `repository.SlotRepository` interface; `sqlite.NewSlotRepository(conn *sql.DB) *sqlite.SlotRepository` — Tasks 4/5/6 depend on this exact constructor name and the interface's method signatures below.

- [ ] **Step 1: Replace `ProgramRepository` with `SlotRepository` in the repository interface**

In `internal/repository/repository.go`, replace the `ProgramRepository` interface with:

```go
type SlotRepository interface {
	Create(ctx context.Context, s *model.Slot) error
	Get(ctx context.Context, id int64) (*model.Slot, error)
	ListByChannel(ctx context.Context, channelID int64) ([]*model.Slot, error)
	Update(ctx context.Context, s *model.Slot) error
	Delete(ctx context.Context, id int64) error
}
```

(`ResolveDate`, written in Task 3, operates in-memory over the full `ListByChannel` result — the same "fetch everything, compute in memory" pattern `scheduler.Evaluate` already uses today — so no separate day-scoped query method is needed.)

- [ ] **Step 2: Delete the old program repository files**

```bash
rm internal/repository/sqlite/program_repository.go internal/repository/sqlite/program_repository_test.go
```

- [ ] **Step 3: Write the failing test**

Create `internal/repository/sqlite/slot_repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func setupChannel(t *testing.T, ctx context.Context, conn *sql.DB) *model.Channel {
	t.Helper()
	channelRepo := sqlite.NewChannelRepository(conn)
	channel := &model.Channel{Name: "Movies"}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}
	return channel
}

func setupMediaItem(t *testing.T, ctx context.Context, conn *sql.DB) *model.MediaItem {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("creating media item: %v", err)
	}
	return item
}

func TestSlotRepository_CreateAndGet_RecurringMediaSlot(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel := setupChannel(t, ctx, conn)
	item := setupMediaItem(t, ctx, conn)
	repo := sqlite.NewSlotRepository(conn)

	dayOfWeek := 1
	position := 1000
	slot := &model.Slot{
		ChannelID:   channel.ID,
		Kind:        model.SlotKindMedia,
		MediaItemID: &item.ID,
		Recurring:   true,
		DayOfWeek:   &dayOfWeek,
		Position:    &position,
	}
	if err := repo.Create(ctx, slot); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if slot.ID == 0 {
		t.Fatal("expected a non-zero ID after Create")
	}

	got, err := repo.Get(ctx, slot.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != model.SlotKindMedia || got.MediaItemID == nil || *got.MediaItemID != item.ID {
		t.Fatalf("Get returned %+v, want media slot referencing item %d", got, item.ID)
	}
	if !got.Recurring || got.DayOfWeek == nil || *got.DayOfWeek != dayOfWeek {
		t.Fatalf("Get returned %+v, want recurring=true day_of_week=%d", got, dayOfWeek)
	}
	if got.StartTime != nil {
		t.Fatalf("expected nil StartTime for a recurring slot, got %v", got.StartTime)
	}
}

func TestSlotRepository_CreateAndGet_OneOffGapSlot(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel := setupChannel(t, ctx, conn)
	repo := sqlite.NewSlotRepository(conn)

	start := time.Date(2026, 8, 31, 21, 0, 0, 0, time.UTC)
	duration := 300.0
	slot := &model.Slot{
		ChannelID:      channel.ID,
		Kind:           model.SlotKindGap,
		GapDurationSec: &duration,
		GapLabel:       "Ad Break",
		Recurring:      false,
		StartTime:      &start,
	}
	if err := repo.Create(ctx, slot); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, slot.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != model.SlotKindGap || got.GapDurationSec == nil || *got.GapDurationSec != duration {
		t.Fatalf("Get returned %+v, want a gap slot with duration %v", got, duration)
	}
	if got.MediaItemID != nil {
		t.Fatalf("expected nil MediaItemID for a gap slot, got %v", got.MediaItemID)
	}
	if got.Recurring {
		t.Fatal("expected Recurring=false")
	}
	if got.StartTime == nil || !got.StartTime.Equal(start) {
		t.Fatalf("got StartTime %v, want %v", got.StartTime, start)
	}
	if got.DayOfWeek != nil || got.Position != nil {
		t.Fatalf("expected nil DayOfWeek/Position for a one-off slot, got %+v", got)
	}
}

func TestSlotRepository_ListByChannel_OrderedAndScoped(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelA := setupChannel(t, ctx, conn)
	channelB := setupChannel(t, ctx, conn)
	item := setupMediaItem(t, ctx, conn)
	repo := sqlite.NewSlotRepository(conn)

	dayOfWeek, posA, posB := 2, 2000, 1000
	slotA := &model.Slot{ChannelID: channelA.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &posA}
	slotB := &model.Slot{ChannelID: channelA.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &posB}
	other := &model.Slot{ChannelID: channelB.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &posA}
	for _, s := range []*model.Slot{slotA, slotB, other} {
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	got, err := repo.ListByChannel(ctx, channelA.ID)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2 (scoped to channelA)", len(got))
	}
}

func TestSlotRepository_Update(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel := setupChannel(t, ctx, conn)
	item := setupMediaItem(t, ctx, conn)
	repo := sqlite.NewSlotRepository(conn)

	dayOfWeek, position := 3, 1000
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := repo.Create(ctx, slot); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newPosition := 2000
	slot.Position = &newPosition
	if err := repo.Update(ctx, slot); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(ctx, slot.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Position == nil || *got.Position != newPosition {
		t.Fatalf("got Position %v, want %d", got.Position, newPosition)
	}
}

func TestSlotRepository_Delete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel := setupChannel(t, ctx, conn)
	item := setupMediaItem(t, ctx, conn)
	repo := sqlite.NewSlotRepository(conn)

	dayOfWeek, position := 4, 1000
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := repo.Create(ctx, slot); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, slot.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, slot.ID); err == nil {
		t.Fatal("expected an error getting a deleted slot")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/repository/sqlite/... -run TestSlotRepository -v`
Expected: FAIL to compile — `sqlite.NewSlotRepository` undefined.

- [ ] **Step 5: Implement `SlotRepository`**

Create `internal/repository/sqlite/slot_repository.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type SlotRepository struct {
	db *sql.DB
}

func NewSlotRepository(conn *sql.DB) *SlotRepository {
	return &SlotRepository{db: conn}
}

func (r *SlotRepository) Create(ctx context.Context, s *model.Slot) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO slots (channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ChannelID, s.Kind, nullableInt64(s.MediaItemID), nullableFloat64(s.GapDurationSec), s.GapLabel,
		s.Recurring, nullableIntAsInt64(s.DayOfWeek), nullableIntAsInt64(s.Position), nullableTime(s.StartTime),
		db.FormatTime(now), db.FormatTime(now))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (r *SlotRepository) Get(ctx context.Context, id int64) (*model.Slot, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at
		 FROM slots WHERE id = ?`, id)
	return scanSlot(row.Scan)
}

func (r *SlotRepository) ListByChannel(ctx context.Context, channelID int64) ([]*model.Slot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at
		 FROM slots WHERE channel_id = ?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]*model.Slot, 0)
	for rows.Next() {
		s, err := scanSlot(rows.Scan)
		if err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

func (r *SlotRepository) Update(ctx context.Context, s *model.Slot) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE slots SET channel_id = ?, kind = ?, media_item_id = ?, gap_duration_sec = ?, gap_label = ?,
		 recurring = ?, day_of_week = ?, position = ?, start_time = ?, updated_at = ? WHERE id = ?`,
		s.ChannelID, s.Kind, nullableInt64(s.MediaItemID), nullableFloat64(s.GapDurationSec), s.GapLabel,
		s.Recurring, nullableIntAsInt64(s.DayOfWeek), nullableIntAsInt64(s.Position), nullableTime(s.StartTime),
		db.FormatTime(now), s.ID)
	if err != nil {
		return err
	}
	s.UpdatedAt = now
	return nil
}

func (r *SlotRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM slots WHERE id = ?`, id)
	return err
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableIntAsInt64(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

func nullableFloat64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return db.FormatTime(*v)
}

func scanSlot(scan scanRow) (*model.Slot, error) {
	var s model.Slot
	var mediaItemID sql.NullInt64
	var gapDurationSec sql.NullFloat64
	var dayOfWeek sql.NullInt64
	var position sql.NullInt64
	var startTime sql.NullString
	var createdAt, updatedAt string

	err := scan(&s.ID, &s.ChannelID, &s.Kind, &mediaItemID, &gapDurationSec, &s.GapLabel,
		&s.Recurring, &dayOfWeek, &position, &startTime, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if mediaItemID.Valid {
		v := mediaItemID.Int64
		s.MediaItemID = &v
	}
	if gapDurationSec.Valid {
		v := gapDurationSec.Float64
		s.GapDurationSec = &v
	}
	if dayOfWeek.Valid {
		v := int(dayOfWeek.Int64)
		s.DayOfWeek = &v
	}
	if position.Valid {
		v := int(position.Int64)
		s.Position = &v
	}
	if startTime.Valid {
		t, err := db.ParseTime(startTime.String)
		if err != nil {
			return nil, err
		}
		s.StartTime = &t
	}
	if s.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}
```

`scanRow` already exists in this package (shared with the other repositories, per the codebase's existing convention — do not redefine it).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/repository/sqlite/... -v`
Expected: PASS (all `SlotRepository` tests, plus every other repository's existing tests still passing).

- [ ] **Step 7: Commit**

```bash
git add internal/repository/repository.go internal/repository/sqlite/slot_repository.go internal/repository/sqlite/slot_repository_test.go
git rm internal/repository/sqlite/program_repository.go internal/repository/sqlite/program_repository_test.go
git commit -m "feat: replace ProgramRepository with SlotRepository"
```

---

## Task 3: `ResolveDate` — recurring/one-off resolution

**Files:**
- Create: `internal/channels/resolve.go`
- Create: `internal/channels/resolve_test.go`

**Interfaces:**
- Consumes: `model.Slot` (Task 1), `scheduler.ScheduledProgram`/`scheduler.Evaluate` (existing, unchanged — `internal/scheduler/scheduler.go`).
- Produces: `channels.ResolveDate(slots []*model.Slot, mediaByID map[int64]*model.MediaItem, date time.Time) []scheduler.ScheduledProgram` and `channels.SlotDuration(s *model.Slot, mediaByID map[int64]*model.MediaItem) (time.Duration, bool)` — Tasks 4 and 5 depend on both exact signatures.

This lives in `internal/channels`, not `internal/scheduler`: `internal/scheduler/scheduler.go` currently has zero dependency on `internal/model` (it defines its own decoupled `ScheduledProgram` type), and resolving slots inherently needs `model.Slot`/`model.MediaItem`. Keeping `internal/scheduler` model-free preserves its existing purity; `internal/channels` already does the equivalent job today (`Service.CurrentState` turns persisted rows into `[]scheduler.ScheduledProgram`), so this is the same responsibility, not a new one.

- [ ] **Step 1: Write the failing tests**

Create `internal/channels/resolve_test.go`:

```go
package channels_test

import (
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/model"
)

func mediaItem(id int64, durationSec float64) *model.MediaItem {
	return &model.MediaItem{ID: id, DurationSec: durationSec}
}

func recurringMediaSlot(id int64, mediaItemID int64, dayOfWeek, position int) *model.Slot {
	return &model.Slot{ID: id, Kind: model.SlotKindMedia, MediaItemID: &mediaItemID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
}

func oneOffMediaSlot(id int64, mediaItemID int64, start time.Time) *model.Slot {
	return &model.Slot{ID: id, Kind: model.SlotKindMedia, MediaItemID: &mediaItemID, Recurring: false, StartTime: &start}
}

func TestResolveDate_EmptyDay(t *testing.T) {
	date := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // a Monday
	got := channels.ResolveDate(nil, nil, date)
	if len(got) != 0 {
		t.Fatalf("got %d resolved slots, want 0", len(got))
	}
}

func TestResolveDate_OneRecurringSlot_StartsAtMidnight(t *testing.T) {
	date := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) // a Monday, day_of_week=1
	slots := []*model.Slot{recurringMediaSlot(1, 100, 1, 1000)}
	mediaByID := map[int64]*model.MediaItem{100: mediaItem(100, 3600)}

	got := channels.ResolveDate(slots, mediaByID, date)
	if len(got) != 1 {
		t.Fatalf("got %d resolved slots, want 1", len(got))
	}
	wantStart := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if !got[0].StartTime.Equal(wantStart) {
		t.Fatalf("got StartTime %v, want %v", got[0].StartTime, wantStart)
	}
	if got[0].Duration != time.Hour {
		t.Fatalf("got Duration %v, want 1h", got[0].Duration)
	}
}

func TestResolveDate_TwoRecurringSlots_SecondStartsAfterFirstEnds(t *testing.T) {
	date := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	slots := []*model.Slot{
		recurringMediaSlot(1, 100, 1, 1000),
		recurringMediaSlot(2, 200, 1, 2000),
	}
	mediaByID := map[int64]*model.MediaItem{
		100: mediaItem(100, 3600),
		200: mediaItem(200, 1800),
	}

	got := channels.ResolveDate(slots, mediaByID, date)
	if len(got) != 2 {
		t.Fatalf("got %d resolved slots, want 2", len(got))
	}
	wantSecondStart := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	if !got[1].StartTime.Equal(wantSecondStart) {
		t.Fatalf("second slot StartTime %v, want %v", got[1].StartTime, wantSecondStart)
	}
}

func TestResolveDate_RecurringSlot_OnlyAppliesOnItsOwnWeekday(t *testing.T) {
	tuesday := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	slots := []*model.Slot{recurringMediaSlot(1, 100, 1 /* Monday */, 1000)}
	mediaByID := map[int64]*model.MediaItem{100: mediaItem(100, 3600)}

	got := channels.ResolveDate(slots, mediaByID, tuesday)
	if len(got) != 0 {
		t.Fatalf("got %d resolved slots on a non-matching weekday, want 0", len(got))
	}
}

func TestResolveDate_OneOffSlot_OnlyAppliesOnItsOwnDate(t *testing.T) {
	targetDate := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	oneOffStart := time.Date(2026, 8, 31, 21, 0, 0, 0, time.UTC)
	slots := []*model.Slot{oneOffMediaSlot(1, 100, oneOffStart)}
	mediaByID := map[int64]*model.MediaItem{100: mediaItem(100, 1800)}

	got := channels.ResolveDate(slots, mediaByID, targetDate)
	if len(got) != 1 || !got[0].StartTime.Equal(oneOffStart) {
		t.Fatalf("got %+v, want one resolved slot starting at %v", got, oneOffStart)
	}

	nextDay := targetDate.AddDate(0, 0, 1)
	got = channels.ResolveDate(slots, mediaByID, nextDay)
	if len(got) != 0 {
		t.Fatalf("got %d resolved slots on a different date, want 0", len(got))
	}
}

func TestResolveDate_SkipsSlotWithNoUsableDuration(t *testing.T) {
	date := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	slots := []*model.Slot{recurringMediaSlot(1, 999 /* missing from mediaByID */, 1, 1000)}

	got := channels.ResolveDate(slots, map[int64]*model.MediaItem{}, date)
	if len(got) != 0 {
		t.Fatalf("got %d resolved slots for missing media, want 0", len(got))
	}
}

func TestResolveDate_GapSlot_UsesGapDuration(t *testing.T) {
	date := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	gapDuration := 300.0
	dayOfWeek, position := 1, 1000
	slots := []*model.Slot{{ID: 1, Kind: model.SlotKindGap, GapDurationSec: &gapDuration, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}}

	got := channels.ResolveDate(slots, nil, date)
	if len(got) != 1 || got[0].Duration != 5*time.Minute {
		t.Fatalf("got %+v, want one 5-minute gap slot", got)
	}
}

func TestSlotDuration_GapWithoutDuration_IsUnusable(t *testing.T) {
	slot := &model.Slot{Kind: model.SlotKindGap}
	if _, ok := channels.SlotDuration(slot, nil); ok {
		t.Fatal("expected a gap slot with no gap_duration_sec to be unusable")
	}
}

func TestSlotDuration_MediaWithInvalidItem_IsUnusable(t *testing.T) {
	mediaItemID := int64(100)
	slot := &model.Slot{Kind: model.SlotKindMedia, MediaItemID: &mediaItemID}
	mediaByID := map[int64]*model.MediaItem{100: {ID: 100, DurationSec: 3600, Invalid: true}}
	if _, ok := channels.SlotDuration(slot, mediaByID); ok {
		t.Fatal("expected an invalid media item to be unusable")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/channels/... -run "TestResolveDate|TestSlotDuration" -v`
Expected: FAIL to compile — `channels.ResolveDate`/`channels.SlotDuration` undefined.

- [ ] **Step 3: Implement `ResolveDate` and `SlotDuration`**

Create `internal/channels/resolve.go`:

```go
package channels

import (
	"sort"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/scheduler"
)

// SlotDuration returns how long a slot occupies and whether that duration is
// usable at all. A gap slot is unusable if it has no GapDurationSec. A media
// slot is unusable if its MediaItemID isn't in mediaByID, or the item is
// Invalid, or has no positive DurationSec — mirroring the same
// missing/invalid-media handling channels.Service.CurrentState already did
// for Program before this type existed.
func SlotDuration(s *model.Slot, mediaByID map[int64]*model.MediaItem) (time.Duration, bool) {
	if s.Kind == model.SlotKindGap {
		if s.GapDurationSec == nil || *s.GapDurationSec <= 0 {
			return 0, false
		}
		return time.Duration(*s.GapDurationSec * float64(time.Second)), true
	}
	if s.MediaItemID == nil {
		return 0, false
	}
	item, ok := mediaByID[*s.MediaItemID]
	if !ok || item.Invalid || item.DurationSec <= 0 {
		return 0, false
	}
	return time.Duration(item.DurationSec * float64(time.Second)), true
}

// ResolveDate returns the concrete occurrences that occupy a channel's slots
// on the given calendar date (evaluated in UTC — see Global Constraints):
// every recurring slot whose DayOfWeek matches date's weekday, walked in
// Position order with times computed by cumulative duration from midnight,
// plus every one-off slot whose StartTime falls on that same calendar date.
// Slots with no usable duration are skipped (see SlotDuration).
func ResolveDate(slots []*model.Slot, mediaByID map[int64]*model.MediaItem, date time.Time) []scheduler.ScheduledProgram {
	date = date.UTC()
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	weekday := int(date.Weekday())

	var recurring []*model.Slot
	for _, s := range slots {
		if s.Recurring && s.DayOfWeek != nil && *s.DayOfWeek == weekday {
			recurring = append(recurring, s)
		}
	}
	sort.Slice(recurring, func(i, j int) bool { return positionOf(recurring[i]) < positionOf(recurring[j]) })

	var resolved []scheduler.ScheduledProgram
	cursor := dayStart
	for _, s := range recurring {
		dur, ok := SlotDuration(s, mediaByID)
		if !ok {
			continue
		}
		resolved = append(resolved, toScheduledProgram(s, cursor, dur))
		cursor = cursor.Add(dur)
	}

	for _, s := range slots {
		if s.Recurring || s.StartTime == nil {
			continue
		}
		start := s.StartTime.UTC()
		if start.Year() != date.Year() || start.YearDay() != date.YearDay() {
			continue
		}
		dur, ok := SlotDuration(s, mediaByID)
		if !ok {
			continue
		}
		resolved = append(resolved, toScheduledProgram(s, start, dur))
	}

	return resolved
}

func positionOf(s *model.Slot) int {
	if s.Position == nil {
		return 0
	}
	return *s.Position
}

func toScheduledProgram(s *model.Slot, start time.Time, dur time.Duration) scheduler.ScheduledProgram {
	var mediaItemID int64
	if s.MediaItemID != nil {
		mediaItemID = *s.MediaItemID
	}
	return scheduler.ScheduledProgram{
		ProgramID:   s.ID,
		MediaItemID: mediaItemID,
		StartTime:   start,
		Duration:    dur,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/channels/... -run "TestResolveDate|TestSlotDuration" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/channels/resolve.go internal/channels/resolve_test.go
git commit -m "feat: add ResolveDate slot resolution (recurring + one-off)"
```

---

## Task 4: `channels.Service` — Slot CRUD + placement validation

**Files:**
- Modify: `internal/channels/service.go`
- Modify: `internal/channels/service_test.go`

**Interfaces:**
- Consumes: `repository.SlotRepository` (Task 2), `channels.ResolveDate`/`channels.SlotDuration` (Task 3).
- Produces: `Service.AddSlot`, `Service.GetSlot`, `Service.UpdateSlot`, `Service.RemoveSlot`, `Service.ListSlots` (replacing the `*Program` methods), `channels.ValidationError` — Task 6 (API handlers) depends on this exact error type to map to HTTP 400.

- [ ] **Step 1: Write the failing tests**

In `internal/channels/service_test.go`, add (adjust existing `Program`-based test helpers in this file to build `*model.Slot` instead, following the same pattern as the tests below):

```go
func TestService_AddSlot_RejectsWhenRecurringDayWouldSpillPastMidnight(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 23*3600) // 23h-long "movie"

	dayOfWeek, position := 1, 1000
	first := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, first); err != nil {
		t.Fatalf("AddSlot (first): %v", err)
	}

	secondPosition := 2000
	second := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &secondPosition}
	err := svc.AddSlot(ctx, second)
	var verr *channels.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("AddSlot (second, should overflow the day): got err %v, want a *channels.ValidationError", err)
	}
}

func TestService_AddSlot_RejectsOneOffOverlappingARecurringSlot(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600) // 1h-long

	dayOfWeek, position := 1, 1000
	recurring := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, recurring); err != nil {
		t.Fatalf("AddSlot (recurring): %v", err)
	}

	// A Monday: 2026-08-31. The recurring slot resolves to 00:00-01:00 that
	// day; a one-off dropped at 00:30 overlaps it.
	overlapStart := time.Date(2026, 8, 31, 0, 30, 0, 0, time.UTC)
	oneOff := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: false, StartTime: &overlapStart}
	err := svc.AddSlot(ctx, oneOff)
	var verr *channels.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("AddSlot (overlapping one-off): got err %v, want a *channels.ValidationError", err)
	}
}

func TestService_AddSlot_AcceptsOneOffInAGenuineGap(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600)

	dayOfWeek, position := 1, 1000
	recurring := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, recurring); err != nil {
		t.Fatalf("AddSlot (recurring): %v", err)
	}

	// Recurring slot occupies 00:00-01:00 that Monday; 02:00 is a genuine gap.
	gapStart := time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC)
	oneOff := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: false, StartTime: &gapStart}
	if err := svc.AddSlot(ctx, oneOff); err != nil {
		t.Fatalf("AddSlot (one-off in a real gap): %v", err)
	}
}

func TestService_UpdateSlot_ExcludesItsOwnPriorStateFromValidation(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600)

	dayOfWeek, position := 1, 1000
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, slot); err != nil {
		t.Fatalf("AddSlot: %v", err)
	}

	// Moving the slot to a new position on the same (otherwise empty) day
	// must not be rejected as if it collided with its own prior placement.
	newPosition := 5000
	slot.Position = &newPosition
	if err := svc.UpdateSlot(ctx, slot); err != nil {
		t.Fatalf("UpdateSlot (moving the only slot on its day): %v", err)
	}
}
```

Add the shared test helper (place near the top of `internal/channels/service_test.go`, alongside any existing helpers in that file):

```go
func newServiceWithChannelAndMediaItem(t *testing.T, durationSec float64) (*channels.Service, *model.Channel, *model.MediaItem) {
	t.Helper()
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: durationSec}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("creating media item: %v", err)
	}
	channel := &model.Channel{Name: "Movies"}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}

	svc := channels.NewService(channelRepo, slotRepo, itemRepo)
	return svc, channel, item
}
```

This requires `"errors"`, `"personaltv/internal/db"`, and `"personaltv/internal/repository/sqlite"` imported in `internal/channels/service_test.go` if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/channels/... -v`
Expected: FAIL to compile — `channels.NewService`'s second parameter type mismatch (still expects `repository.ProgramRepository`), `svc.AddSlot`/`channels.ValidationError` undefined.

- [ ] **Step 3: Implement the service changes**

In `internal/channels/service.go`:
- Rename the `programs repository.ProgramRepository` field to `slots repository.SlotRepository`, and update `NewService`'s parameter accordingly.
- Replace `AddProgram`/`GetProgram`/`UpdateProgram`/`RemoveProgram`/`ListPrograms` with the methods below (same bodies, renamed, plus validation on write):

```go
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

const dayLength = 24 * time.Hour

func (s *Service) AddSlot(ctx context.Context, slot *model.Slot) error {
	if err := s.validateSlot(ctx, slot, 0); err != nil {
		return err
	}
	return s.slots.Create(ctx, slot)
}

func (s *Service) GetSlot(ctx context.Context, id int64) (*model.Slot, error) {
	return s.slots.Get(ctx, id)
}

func (s *Service) UpdateSlot(ctx context.Context, slot *model.Slot) error {
	if err := s.validateSlot(ctx, slot, slot.ID); err != nil {
		return err
	}
	return s.slots.Update(ctx, slot)
}

func (s *Service) RemoveSlot(ctx context.Context, id int64) error {
	return s.slots.Delete(ctx, id)
}

func (s *Service) ListSlots(ctx context.Context, channelID int64) ([]*model.Slot, error) {
	return s.slots.ListByChannel(ctx, channelID)
}

// validateSlot enforces spec §5: a recurring slot's day must not exceed 24h
// once it's placed, and a one-off slot must land in a genuine gap (not
// overlapping any resolved slot on its date) and must not spill past
// midnight. excludeID lets an update validate against every OTHER existing
// slot without rejecting itself as a false collision with its own prior
// placement.
func (s *Service) validateSlot(ctx context.Context, candidate *model.Slot, excludeID int64) error {
	existing, err := s.slots.ListByChannel(ctx, candidate.ChannelID)
	if err != nil {
		return err
	}
	others := make([]*model.Slot, 0, len(existing))
	for _, e := range existing {
		if e.ID != excludeID {
			others = append(others, e)
		}
	}

	mediaByID, err := s.mediaByIDForSlots(ctx, append(append([]*model.Slot{}, others...), candidate))
	if err != nil {
		return err
	}

	if candidate.Recurring {
		if candidate.DayOfWeek == nil || candidate.Position == nil {
			return &ValidationError{Msg: "recurring slots require day_of_week and position"}
		}
		if _, ok := SlotDuration(candidate, mediaByID); !ok {
			return &ValidationError{Msg: "slot has no usable duration"}
		}
		var total time.Duration
		for _, o := range others {
			if o.Recurring && o.DayOfWeek != nil && *o.DayOfWeek == *candidate.DayOfWeek {
				if d, ok := SlotDuration(o, mediaByID); ok {
					total += d
				}
			}
		}
		if d, ok := SlotDuration(candidate, mediaByID); ok {
			total += d
		}
		if total > dayLength {
			return &ValidationError{Msg: "doesn't fit: this day is already full"}
		}
		return nil
	}

	if candidate.StartTime == nil {
		return &ValidationError{Msg: "one-off slots require start_time"}
	}
	dur, ok := SlotDuration(candidate, mediaByID)
	if !ok {
		return &ValidationError{Msg: "slot has no usable duration"}
	}
	start := candidate.StartTime.UTC()
	end := start.Add(dur)
	dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	if end.After(dayStart.Add(dayLength)) {
		return &ValidationError{Msg: "doesn't fit: would spill past midnight"}
	}
	for _, r := range ResolveDate(others, mediaByID, start) {
		if start.Before(r.EndTime()) && r.StartTime.Before(end) {
			return &ValidationError{Msg: "doesn't fit: overlaps another slot"}
		}
	}
	return nil
}

func (s *Service) mediaByIDForSlots(ctx context.Context, slots []*model.Slot) (map[int64]*model.MediaItem, error) {
	mediaByID := make(map[int64]*model.MediaItem)
	for _, slot := range slots {
		if slot.MediaItemID == nil {
			continue
		}
		if _, ok := mediaByID[*slot.MediaItemID]; ok {
			continue
		}
		item, err := s.items.Get(ctx, *slot.MediaItemID)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		mediaByID[*slot.MediaItemID] = item
	}
	return mediaByID, nil
}
```

`errors` must be imported in `internal/channels/service.go` (it already is, per the existing `CurrentState` method's `errors.Is` call).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/channels/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/channels/service.go internal/channels/service_test.go
git commit -m "feat: add Slot CRUD with recurring/one-off placement validation"
```

---

## Task 5: `channels.Service` — `CurrentState` via resolution + `ResolvedWindow`

**Files:**
- Modify: `internal/channels/service.go`
- Modify: `internal/channels/service_test.go`

**Interfaces:**
- Consumes: `channels.ResolveDate` (Task 3), `Service.mediaByIDForSlots` (Task 4).
- Produces: `Service.ResolvedWindow(ctx, channelID int64, from, to time.Time) ([]scheduler.ScheduledProgram, error)` — Task 6 depends on this exact signature.

- [ ] **Step 1: Write the failing tests**

Add to `internal/channels/service_test.go`:

```go
func TestService_CurrentState_ResolvesFromRecurringSlots(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600)

	monday := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	dayOfWeek, position := int(monday.Weekday()), 1000
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, slot); err != nil {
		t.Fatalf("AddSlot: %v", err)
	}

	now := monday.Add(30 * time.Minute)
	state, err := svc.CurrentState(ctx, channel.ID, now)
	if err != nil {
		t.Fatalf("CurrentState: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != slot.ID {
		t.Fatalf("got %+v, want the recurring slot playing at %v", state, now)
	}
	if state.Offset != 30*time.Minute {
		t.Fatalf("got Offset %v, want 30m", state.Offset)
	}
}

func TestService_CurrentState_NextLooksIntoTomorrowWhenTodayIsExhausted(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600)

	monday := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	mondayDay, tuesdayDay, position := int(monday.Weekday()), int(tuesday.Weekday()), 1000
	mondaySlot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &mondayDay, Position: &position}
	tuesdaySlot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &tuesdayDay, Position: &position}
	if err := svc.AddSlot(ctx, mondaySlot); err != nil {
		t.Fatalf("AddSlot (monday): %v", err)
	}
	if err := svc.AddSlot(ctx, tuesdaySlot); err != nil {
		t.Fatalf("AddSlot (tuesday): %v", err)
	}

	// Monday's only slot (00:00-01:00) has already ended by 23:00 Monday;
	// "next" must find Tuesday's slot at 00:00, not report nothing.
	now := monday.Add(23 * time.Hour)
	state, err := svc.CurrentState(ctx, channel.ID, now)
	if err != nil {
		t.Fatalf("CurrentState: %v", err)
	}
	if state.Next == nil || state.Next.ProgramID != tuesdaySlot.ID {
		t.Fatalf("got %+v, want Next to be Tuesday's slot", state)
	}
}

func TestService_ResolvedWindow_ReturnsOccurrencesAcrossMultipleDays(t *testing.T) {
	ctx := context.Background()
	svc, channel, item := newServiceWithChannelAndMediaItem(t, 3600)

	monday := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	dayOfWeek, position := int(monday.Weekday()), 1000
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: true, DayOfWeek: &dayOfWeek, Position: &position}
	if err := svc.AddSlot(ctx, slot); err != nil {
		t.Fatalf("AddSlot: %v", err)
	}

	from := monday
	to := monday.AddDate(0, 0, 14) // two weeks: the recurring slot should appear twice
	resolved, err := svc.ResolvedWindow(ctx, channel.ID, from, to)
	if err != nil {
		t.Fatalf("ResolvedWindow: %v", err)
	}
	count := 0
	for _, r := range resolved {
		if r.ProgramID == slot.ID {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("got %d occurrences of the weekly recurring slot across 2 weeks, want 2", count)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/channels/... -run "TestService_CurrentState_ResolvesFromRecurringSlots|TestService_CurrentState_NextLooksIntoTomorrowWhenTodayIsExhausted|TestService_ResolvedWindow" -v`
Expected: FAIL — `CurrentState` still builds `ScheduledProgram` directly from `Program`-shaped data (won't compile once Task 4 lands; if it does compile at this point, the "next looks into tomorrow" case fails because today's list doesn't include tomorrow), `svc.ResolvedWindow` undefined.

- [ ] **Step 3: Implement**

In `internal/channels/service.go`, replace the existing `CurrentState` method body with:

```go
const lookaheadDays = 7

func (s *Service) CurrentState(ctx context.Context, channelID int64, now time.Time) (scheduler.CurrentState, error) {
	slots, err := s.slots.ListByChannel(ctx, channelID)
	if err != nil {
		return scheduler.CurrentState{}, err
	}
	mediaByID, err := s.mediaByIDForSlots(ctx, slots)
	if err != nil {
		return scheduler.CurrentState{}, err
	}

	now = now.UTC()
	var resolved []scheduler.ScheduledProgram
	for day := 0; day <= lookaheadDays; day++ {
		resolved = append(resolved, ResolveDate(slots, mediaByID, now.AddDate(0, 0, day))...)
	}
	return scheduler.Evaluate(resolved, now), nil
}

// ResolvedWindow returns every slot occurrence between from (inclusive) and
// to (exclusive), resolving one calendar day at a time. Used by the Guide
// screen and the weekly schedule timeline, both of which need concrete
// occurrences over a bounded window rather than the raw (and, for a
// recurring slot, effectively infinite) slot list.
func (s *Service) ResolvedWindow(ctx context.Context, channelID int64, from, to time.Time) ([]scheduler.ScheduledProgram, error) {
	slots, err := s.slots.ListByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	mediaByID, err := s.mediaByIDForSlots(ctx, slots)
	if err != nil {
		return nil, err
	}

	from, to = from.UTC(), to.UTC()
	var resolved []scheduler.ScheduledProgram
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		resolved = append(resolved, ResolveDate(slots, mediaByID, d)...)
	}
	return resolved, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/channels/... -v`
Expected: PASS.

- [ ] **Step 5: Run the whole backend suite to check for fallout in other packages**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: FAIL only in `internal/api` (Task 6 fixes it) — every other package should PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/channels/service.go internal/channels/service_test.go
git commit -m "feat: resolve CurrentState from slots; add ResolvedWindow"
```

---

## Task 6: API — slot handlers, resolved-window endpoint, routes

**Files:**
- Create: `internal/api/slots_handlers.go`
- Create: `internal/api/slots_handlers_test.go`
- Delete: `internal/api/programs_handlers.go`, `internal/api/programs_handlers_test.go` (if the latter exists)
- Modify: `internal/api/router.go`

**Interfaces:**
- Consumes: `Service.AddSlot`/`GetSlot`/`UpdateSlot`/`RemoveSlot`/`ListSlots`/`ResolvedWindow` (Tasks 4-5), `channels.ValidationError` (Task 4).
- Produces: `GET/POST /api/channels/{id}/slots`, `PUT/DELETE /api/slots/{id}`, `GET /api/channels/{id}/slots/resolved` — Task 7 (frontend API client) depends on these exact paths and JSON shapes.

Confirmed: `internal/api/programs_handlers_test.go` does not exist (only `internal/api/programs_handlers.go` needs deleting). This package's tests share `newTestServer(t) (*api.Server, *playback.SessionManager)` and `newTestServerWithConn(t, conn) (*api.Server, *playback.SessionManager)`, both defined in `internal/api/sources_handlers_test.go` (per this codebase's documented convention of sharing handler-test setup within a package). Both currently build their `channels.Service` from `sqlite.NewProgramRepository(conn)` — Step 1 below fixes that in place.

- [ ] **Step 1: Fix the shared test helpers in `sources_handlers_test.go`**

In `internal/api/sources_handlers_test.go`, in both `newTestServer` and `newTestServerWithConn`, replace:

```go
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
```

with:

```go
	slotRepo := sqlite.NewSlotRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, slotRepo, itemRepo)
```

(This appears twice in the file — once per function. Both occurrences need the change.)

- [ ] **Step 2: Delete the old programs handler file**

```bash
rm -f internal/api/programs_handlers.go
```

- [ ] **Step 3: Write the failing tests**

Create `internal/api/slots_handlers_test.go`, using the now-fixed `newTestServerWithConn` and this package's existing real-HTTP-client testing style (`httptest.NewServer` + `http.Post`/`http.Get`, as `sources_handlers_test.go`'s `TestSourcesAPI_CreateListDelete` already does):

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func seedChannelAndMediaItem(t *testing.T, conn *sql.DB) (*model.Channel, *model.MediaItem) {
	t.Helper()
	ctx := context.Background()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("creating media item: %v", err)
	}
	channel := &model.Channel{Name: "Movies"}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}
	return channel, item
}

func TestHandleAddSlot_Recurring(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": true, "day_of_week": 1, "position": 1000,
	})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d", resp.StatusCode)
	}
}

func TestHandleAddSlot_RejectsWhenValidationFails(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": false,
		// 30m before midnight; a 1h-long item spills over.
		"start_time": time.Date(2026, 8, 31, 23, 30, 0, 0, time.UTC).Format(time.RFC3339),
	})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400 (spills past midnight)", resp.StatusCode)
	}
}

func TestHandleResolvedSlots(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	addBody, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": true, "day_of_week": 1, "position": 1000,
	})
	addResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(addBody))
	if err != nil {
		t.Fatalf("setup POST failed: %v", err)
	}
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: got status %d", addResp.StatusCode)
	}

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	url := ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/slots/resolved?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d", resp.StatusCode)
	}
	var resolved []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("got %d resolved slots, want 1", len(resolved))
	}
}
```

`seedChannelAndMediaItem`'s `conn` parameter needs `"database/sql"` imported for the `*sql.DB` type — add `"database/sql"` to the import block above.

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestHandleAddSlot|TestHandleResolvedSlots" -v`
Expected: FAIL to compile — handlers/routes don't exist yet.

- [ ] **Step 5: Implement the handlers**

Create `internal/api/slots_handlers.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/model"
)

type slotRequest struct {
	Kind           string     `json:"kind"`
	MediaItemID    *int64     `json:"media_item_id,omitempty"`
	GapDurationSec *float64   `json:"gap_duration_sec,omitempty"`
	GapLabel       string     `json:"gap_label"`
	Recurring      bool       `json:"recurring"`
	DayOfWeek      *int       `json:"day_of_week,omitempty"`
	Position       *int       `json:"position,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
}

func (req slotRequest) toSlot(channelID int64) (*model.Slot, error) {
	if req.Kind != model.SlotKindMedia && req.Kind != model.SlotKindGap {
		return nil, errRequiredFields("kind")
	}
	if req.Kind == model.SlotKindMedia && req.MediaItemID == nil {
		return nil, errRequiredFields("media_item_id")
	}
	if req.Kind == model.SlotKindGap && (req.GapDurationSec == nil || *req.GapDurationSec <= 0) {
		return nil, errRequiredFields("gap_duration_sec")
	}
	if req.Recurring && (req.DayOfWeek == nil || req.Position == nil) {
		return nil, errRequiredFields("day_of_week", "position")
	}
	if !req.Recurring && req.StartTime == nil {
		return nil, errRequiredFields("start_time")
	}
	return &model.Slot{
		ChannelID:      channelID,
		Kind:           req.Kind,
		MediaItemID:    req.MediaItemID,
		GapDurationSec: req.GapDurationSec,
		GapLabel:       req.GapLabel,
		Recurring:      req.Recurring,
		DayOfWeek:      req.DayOfWeek,
		Position:       req.Position,
		StartTime:      req.StartTime,
	}, nil
}

func (s *Server) handleListSlots(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), channelID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	slots, err := s.channels.ListSlots(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

func (s *Server) handleAddSlot(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot, err := req.toSlot(channelID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.AddSlot(r.Context(), slot); err != nil {
		writeSlotError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, slot)
}

func (s *Server) handleUpdateSlot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := s.channels.GetSlot(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot, err := req.toSlot(existing.ChannelID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot.ID = id
	if err := s.channels.UpdateSlot(r.Context(), slot); err != nil {
		writeSlotError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, slot)
}

func (s *Server) handleDeleteSlot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.RemoveSlot(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSlotError(w http.ResponseWriter, err error) {
	var verr *channels.ValidationError
	if errors.As(err, &verr) {
		writeError(w, http.StatusBadRequest, verr)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

type resolvedSlotResponse struct {
	ProgramID   int64     `json:"program_id"`
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

func (s *Server) handleResolvedSlots(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errRequiredFields("from"))
		return
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errRequiredFields("to"))
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), channelID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	resolved, err := s.channels.ResolvedWindow(r.Context(), channelID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]resolvedSlotResponse, 0, len(resolved))
	for _, p := range resolved {
		out = append(out, resolvedSlotResponse{ProgramID: p.ProgramID, MediaItemID: p.MediaItemID, StartTime: p.StartTime, EndTime: p.EndTime()})
	}
	writeJSON(w, http.StatusOK, out)
}
```

In `internal/api/router.go`, replace the four `/programs`-related route lines with:

```go
mux.HandleFunc("GET /api/channels/{id}/slots", s.handleListSlots)
mux.HandleFunc("POST /api/channels/{id}/slots", s.handleAddSlot)
mux.HandleFunc("GET /api/channels/{id}/slots/resolved", s.handleResolvedSlots)
mux.HandleFunc("PUT /api/slots/{id}", s.handleUpdateSlot)
mux.HandleFunc("DELETE /api/slots/{id}", s.handleDeleteSlot)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 7: Fix `internal/integration/end_to_end_test.go`**

This file (confirmed) builds its own `channels.Service` from `sqlite.NewProgramRepository` twice (once per test function, around what are currently lines 51 and 154) and POSTs to `/programs` twice (currently lines 114 and 210) with a bare `{"media_item_id": ..., "start_time": ...}` body. Fix both occurrences of each:

Replace both:
```go
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
```
with:
```go
	slotRepo := sqlite.NewSlotRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, slotRepo, itemRepo)
```

Replace both:
```go
	progBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(progBody))
```
with:
```go
	progBody, _ := json.Marshal(map[string]any{"kind": "media", "media_item_id": item.ID, "recurring": false, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(progBody))
```
(The surrounding `if progResp.StatusCode != http.StatusCreated { ... }`/`add program failed` error-message text can stay as-is — only the endpoint path, request body, and repository/variable names above need to change.)

- [ ] **Step 8: Run the full backend verification**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: PASS, `gofmt -l .` prints nothing, no failures anywhere in the backend.

- [ ] **Step 9: Commit**

```bash
git add internal/api/slots_handlers.go internal/api/slots_handlers_test.go internal/api/router.go internal/api/sources_handlers_test.go internal/integration/end_to_end_test.go
git rm -f internal/api/programs_handlers.go
git commit -m "feat: add slot API endpoints (CRUD + resolved-window)"
```

---

## Task 7: Frontend — `Slot`/`ResolvedSlot` types + API client

**Files:**
- Modify: `web/src/api/types.ts`
- Create: `web/src/api/slots.ts`
- Create: `web/src/api/slots.test.ts`
- Delete: `web/src/api/programs.ts` (confirmed: no `programs.test.ts` exists alongside it)

**Interfaces:**
- Produces: `Slot`, `ResolvedSlot` types; `listSlots`, `listResolvedSlots`, `addSlot`, `updateSlot`, `deleteSlot`, `useSlotsForChannel`, `useResolvedSlots`, `useAddSlot`, `useUpdateSlot`, `useDeleteSlot` — Tasks 8-11 depend on these exact names.

- [ ] **Step 1: Replace `Program` with `Slot`/`ResolvedSlot` in types**

In `web/src/api/types.ts`, replace the `Program` interface with:

```ts
export interface Slot {
  id: number;
  channel_id: number;
  kind: "media" | "gap";
  media_item_id: number | null;
  gap_duration_sec: number | null;
  gap_label: string;
  recurring: boolean;
  day_of_week: number | null;
  position: number | null;
  start_time: string | null;
  created_at: string;
  updated_at: string;
}

export interface ResolvedSlot {
  program_id: number;
  media_item_id: number;
  start_time: string;
  end_time: string;
}
```

- [ ] **Step 2: Delete the old programs API file**

```bash
rm -f web/src/api/programs.ts
```

- [ ] **Step 3: Write the failing test**

Create `web/src/api/slots.test.ts`:

```ts
import { QueryClient } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { addSlot, listResolvedSlots, listSlots, useAddSlot, useResolvedSlots, useSlotsForChannel } from "./slots";

describe("slots API client", () => {
  it("listSlots fetches a channel's slots", async () => {
    server.use(http.get("/api/channels/1/slots", () => HttpResponse.json([{ id: 1 }])));
    const result = await listSlots(1);
    expect(result).toEqual([{ id: 1 }]);
  });

  it("listResolvedSlots fetches the resolved window with from/to query params", async () => {
    server.use(
      http.get("/api/channels/1/slots/resolved", ({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get("from")).toBe("2026-08-31T00:00:00.000Z");
        expect(url.searchParams.get("to")).toBe("2026-09-07T00:00:00.000Z");
        return HttpResponse.json([{ program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" }]);
      })
    );
    const result = await listResolvedSlots(1, "2026-08-31T00:00:00.000Z", "2026-09-07T00:00:00.000Z");
    expect(result).toHaveLength(1);
  });

  it("addSlot posts the slot body without the channelId wrapper field", async () => {
    server.use(
      http.post("/api/channels/1/slots", async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>;
        expect(body).not.toHaveProperty("channelId");
        expect(body).toMatchObject({ kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
        return HttpResponse.json({ id: 1 }, { status: 201 });
      })
    );
    await addSlot({ channelId: 1, kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
  });

  it("useSlotsForChannel exposes the fetched slots", async () => {
    server.use(http.get("/api/channels/1/slots", () => HttpResponse.json([{ id: 1, channel_id: 1 }])));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useSlotsForChannel(1), { wrapper: wrapWithQueryClient(client) });
    await waitFor(() => expect(result.current.data).toEqual([{ id: 1, channel_id: 1 }]));
  });

  it("useResolvedSlots exposes the fetched resolved window", async () => {
    server.use(http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([{ program_id: 1 }])));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useResolvedSlots(1, "2026-08-31T00:00:00.000Z", "2026-09-07T00:00:00.000Z"), {
      wrapper: wrapWithQueryClient(client),
    });
    await waitFor(() => expect(result.current.data).toEqual([{ program_id: 1 }]));
  });

  it("useAddSlot invalidates the channel's slots and resolved queries on success", async () => {
    server.use(http.post("/api/channels/1/slots", () => HttpResponse.json({ id: 1 }, { status: 201 })));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useAddSlot(1), { wrapper: wrapWithQueryClient(client) });
    result.current.mutate({ channelId: 1, kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalled();
  });
});
```

This requires `vi` imported from `vitest` (add it to the existing `import { describe, expect, it } from "vitest"` line). `wrapWithQueryClient`/`createTestQueryClient` live in `web/src/test/queryClient.tsx` (confirmed — note the `.tsx` extension, since it returns JSX) and are already used the same way by `TVScreen.test.tsx`/`ChannelScheduleScreen.test.tsx`.

- [ ] **Step 4: Run test to verify it fails**

Run: `cd web && npx vitest run src/api/slots.test.ts`
Expected: FAIL to compile — `./slots` module doesn't exist yet.

- [ ] **Step 5: Implement the API client**

Create `web/src/api/slots.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { ResolvedSlot, Slot } from "./types";

export function listSlots(channelId: number): Promise<Slot[]> {
  return apiGet<Slot[]>(`/channels/${channelId}/slots`);
}

export function listResolvedSlots(channelId: number, from: string, to: string): Promise<ResolvedSlot[]> {
  const params = new URLSearchParams({ from, to });
  return apiGet<ResolvedSlot[]>(`/channels/${channelId}/slots/resolved?${params}`);
}

export interface SlotInput {
  kind: "media" | "gap";
  media_item_id?: number;
  gap_duration_sec?: number;
  gap_label?: string;
  recurring: boolean;
  day_of_week?: number;
  position?: number;
  start_time?: string;
}

export interface AddSlotInput extends SlotInput {
  channelId: number;
}

export function addSlot(input: AddSlotInput): Promise<Slot> {
  const { channelId, ...body } = input;
  return apiPost<Slot>(`/channels/${channelId}/slots`, body);
}

export interface UpdateSlotInput extends SlotInput {
  id: number;
  channelId: number;
}

export function updateSlot(input: UpdateSlotInput): Promise<Slot> {
  const { id, channelId, ...body } = input;
  void channelId;
  return apiPut<Slot>(`/slots/${id}`, body);
}

export interface DeleteSlotInput {
  id: number;
  channelId: number;
}

export function deleteSlot(input: DeleteSlotInput): Promise<void> {
  return apiDelete(`/slots/${input.id}`);
}

function slotsKey(channelId: number) {
  return ["channels", channelId, "slots"] as const;
}

function resolvedSlotsBaseKey(channelId: number) {
  return ["channels", channelId, "slots", "resolved"] as const;
}

export function useSlotsForChannel(channelId: number) {
  return useQuery({ queryKey: slotsKey(channelId), queryFn: () => listSlots(channelId), enabled: channelId > 0 });
}

export function useResolvedSlots(channelId: number, from: string, to: string) {
  return useQuery({
    queryKey: [...resolvedSlotsBaseKey(channelId), from, to] as const,
    queryFn: () => listResolvedSlots(channelId, from, to),
    enabled: channelId > 0,
  });
}

function invalidateChannelSlots(queryClient: ReturnType<typeof useQueryClient>, channelId: number) {
  queryClient.invalidateQueries({ queryKey: slotsKey(channelId) });
  queryClient.invalidateQueries({ queryKey: resolvedSlotsBaseKey(channelId) });
}

export function useAddSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: addSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}

export function useUpdateSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}

export function useDeleteSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd web && npx vitest run src/api/slots.test.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/api/types.ts web/src/api/slots.ts web/src/api/slots.test.ts
git rm -f web/src/api/programs.ts
git commit -m "feat: add Slot/ResolvedSlot types and API client"
```

---

## Task 8: Frontend — Guide screen switches to the resolved-window endpoint

**Files:**
- Modify: `web/src/scheduling/guide.ts`
- Modify: `web/src/scheduling/guide.test.ts`
- Modify: `web/src/screens/GuideScreen.tsx`
- Modify: `web/src/screens/GuideScreen.test.tsx` (adjust its MSW mocks from `/api/channels/:id/programs` to `/api/channels/:id/slots/resolved`, following the same pattern as the other test changes in this task)

**Interfaces:**
- Consumes: `useResolvedSlots` (Task 7).
- Produces: `joinResolvedSlotsWithMedia` (replacing `joinProgramsWithMedia`) — no other task depends on this, it's Guide-screen-internal.

- [ ] **Step 1: Update `scheduling/guide.ts`'s test for the new join function**

In `web/src/scheduling/guide.test.ts`, replace the `describe("joinProgramsWithMedia", ...)` block with:

```ts
describe("joinResolvedSlotsWithMedia", () => {
  const mediaItem: MediaItem = {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  };

  it("joins each resolved slot with its media title, sorted by start time", () => {
    const media = new Map([[1, mediaItem]]);
    const resolved: ResolvedSlot[] = [
      { program_id: 2, media_item_id: 1, start_time: "2026-01-01T20:00:00Z", end_time: "2026-01-01T21:00:00Z" },
      { program_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z", end_time: "2026-01-01T19:00:00Z" },
    ];
    const joined = joinResolvedSlotsWithMedia(resolved, media);
    expect(joined.map((p) => p.programId)).toEqual([1, 2]);
    expect(joined[0]).toMatchObject({
      title: "Movie A",
      start: new Date("2026-01-01T18:00:00Z"),
      end: new Date("2026-01-01T19:00:00Z"),
    });
  });

  it("falls back to a placeholder title when the media item is missing", () => {
    const resolved: ResolvedSlot[] = [
      { program_id: 1, media_item_id: 99, start_time: "2026-01-01T18:00:00Z", end_time: "2026-01-01T19:00:00Z" },
    ];
    const joined = joinResolvedSlotsWithMedia(resolved, new Map());
    expect(joined[0].title).toBe("Media #99");
  });
});
```

Update this file's imports: replace `import type { MediaItem, Program } from "../api/types";` with `import type { MediaItem, ResolvedSlot } from "../api/types";`, and `joinProgramsWithMedia` with `joinResolvedSlotsWithMedia` in the `import ... from "./guide"` line.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/scheduling/guide.test.ts`
Expected: FAIL — `joinResolvedSlotsWithMedia` undefined.

- [ ] **Step 3: Implement**

In `web/src/scheduling/guide.ts`, replace the `import` line and `joinProgramsWithMedia` function:

```ts
import type { MediaItem, ResolvedSlot } from "../api/types";
```

```ts
export function joinResolvedSlotsWithMedia(resolved: ResolvedSlot[], mediaById: Map<number, MediaItem>): GuideProgram[] {
  return resolved
    .map((r) => {
      const item = mediaById.get(r.media_item_id);
      return {
        programId: r.program_id,
        mediaItemId: r.media_item_id,
        title: item?.title ?? `Media #${r.media_item_id}`,
        start: new Date(r.start_time),
        end: new Date(r.end_time),
      };
    })
    .sort((a, b) => a.start.getTime() - b.start.getTime());
}
```

Remove the now-unused `computeEndTime` import from `web/src/scheduling/guide.ts` if nothing else in the file uses it (`buildTimeline` doesn't call it — only the old `joinProgramsWithMedia` did).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/scheduling/guide.test.ts`
Expected: PASS.

- [ ] **Step 5: Update `GuideScreen.tsx` to use the resolved-window endpoint**

Replace this import in `web/src/screens/GuideScreen.tsx`:

```ts
import { listPrograms } from "../api/programs";
import { buildTimeline, defaultGuideWindow, joinProgramsWithMedia, type TimelineBlock } from "../scheduling/guide";
```

with:

```ts
import { listResolvedSlots } from "../api/slots";
import { buildTimeline, defaultGuideWindow, joinResolvedSlotsWithMedia, type TimelineBlock } from "../scheduling/guide";
```

Replace the `programQueries` block:

```ts
  const programQueries = useQueries({
    queries: (channels ?? []).map((channel) => ({
      queryKey: ["channels", channel.id, "slots", "resolved", windowStart.toISOString(), windowEnd.toISOString()] as const,
      queryFn: () => listResolvedSlots(channel.id, windowStart.toISOString(), windowEnd.toISOString()),
      refetchInterval: POLL_INTERVAL_MS,
    })),
  });
```

And the render body's `buildTimeline(joinProgramsWithMedia(programs, mediaById), windowStart, windowEnd)` call becomes `buildTimeline(joinResolvedSlotsWithMedia(programs, mediaById), windowStart, windowEnd)` (the `rows` mapping already names each query's `.data` as `programs` — no other change needed there since `ResolvedSlot[]` is what's now fetched).

- [ ] **Step 6: Update `GuideScreen.test.tsx`'s MSW mocks**

Read `web/src/screens/GuideScreen.test.tsx` first. Every `http.get("/api/channels/:id/programs", ...)` mock becomes `http.get("/api/channels/:id/slots/resolved", ...)`, and each mocked response's fixtures change from `Program`-shaped (`{ id, channel_id, media_item_id, start_time }`) to `ResolvedSlot`-shaped (`{ program_id, media_item_id, start_time, end_time }`) — compute each fixture's `end_time` explicitly (e.g. `start_time` + the fixture media item's `duration_sec`) rather than relying on client-side computation, since the resolved endpoint always returns a concrete `end_time` now.

- [ ] **Step 7: Run the full frontend test suite**

Run: `cd web && npx tsc -b && npm test`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/scheduling/guide.ts web/src/scheduling/guide.test.ts web/src/screens/GuideScreen.tsx web/src/screens/GuideScreen.test.tsx
git commit -m "feat: switch Guide screen to the resolved-window slots endpoint"
```

---

## Task 9: Frontend — weekly grid pure logic + read-only rendering

**Files:**
- Create: `web/src/scheduling/week.ts`
- Create: `web/src/scheduling/week.test.ts`
- Modify: `web/src/screens/ChannelScheduleScreen.tsx` (full rewrite of the render/data layer; drag-and-drop interactivity is Task 10)
- Modify: `web/src/screens/ChannelScheduleScreen.css`
- Modify: `web/src/screens/ChannelScheduleScreen.test.tsx` (full rewrite)

**Interfaces:**
- Consumes: `useSlotsForChannel`, `useResolvedSlots` (Task 7).
- Produces: `startOfWeekUTC`, `weekDates`, `addWeeks`, `positionForInsert`, `slotsForDate` (all in `web/src/scheduling/week.ts`) — Task 10 depends on `positionForInsert` and `slotsForDate`'s exact signatures.

- [ ] **Step 1: Write the failing tests for the pure week/position logic**

Create `web/src/scheduling/week.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import type { ResolvedSlot, Slot } from "../api/types";
import { addWeeks, positionForInsert, slotsForDate, startOfWeekUTC, weekDates } from "./week";

describe("startOfWeekUTC", () => {
  it("returns the Sunday 00:00 UTC on or before the given date", () => {
    // 2026-09-02 is a Wednesday.
    const got = startOfWeekUTC(new Date("2026-09-02T15:30:00Z"));
    expect(got.toISOString()).toBe("2026-08-30T00:00:00.000Z");
  });

  it("returns the date itself when it's already a Sunday at midnight", () => {
    const got = startOfWeekUTC(new Date("2026-08-30T00:00:00Z"));
    expect(got.toISOString()).toBe("2026-08-30T00:00:00.000Z");
  });
});

describe("weekDates", () => {
  it("returns 7 consecutive UTC midnights starting at weekStart", () => {
    const weekStart = new Date("2026-08-30T00:00:00Z");
    const dates = weekDates(weekStart);
    expect(dates).toHaveLength(7);
    expect(dates[0].toISOString()).toBe("2026-08-30T00:00:00.000Z");
    expect(dates[6].toISOString()).toBe("2026-09-05T00:00:00.000Z");
  });
});

describe("addWeeks", () => {
  it("shifts a date forward by the given number of weeks", () => {
    const got = addWeeks(new Date("2026-08-30T00:00:00Z"), 1);
    expect(got.toISOString()).toBe("2026-09-06T00:00:00.000Z");
  });

  it("shifts a date backward for a negative count", () => {
    const got = addWeeks(new Date("2026-08-30T00:00:00Z"), -1);
    expect(got.toISOString()).toBe("2026-08-23T00:00:00.000Z");
  });
});

describe("positionForInsert", () => {
  it("returns 1000 when the day is empty", () => {
    expect(positionForInsert([], 0)).toBe(1000);
  });

  it("returns 1000 less than the first slot when inserting at the start", () => {
    expect(positionForInsert([2000, 3000], 0)).toBe(1000);
  });

  it("returns 1000 more than the last slot when inserting at the end", () => {
    expect(positionForInsert([1000, 2000], 2)).toBe(3000);
  });

  it("returns the midpoint when inserting between two slots", () => {
    expect(positionForInsert([1000, 3000], 1)).toBe(2000);
  });
});

describe("slotsForDate", () => {
  const slot: Slot = {
    id: 1, channel_id: 1, kind: "media", media_item_id: 5, gap_duration_sec: null, gap_label: "",
    recurring: true, day_of_week: 1, position: 1000, start_time: null,
    created_at: "", updated_at: "",
  };
  const resolved: ResolvedSlot[] = [
    { program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" },
    { program_id: 2, media_item_id: 5, start_time: "2026-09-01T00:00:00Z", end_time: "2026-09-01T01:00:00Z" },
  ];

  it("returns only the occurrences whose start falls on the given UTC date, sorted by start time", () => {
    const slotsById = new Map([[1, slot]]);
    const got = slotsForDate(resolved, slotsById, new Date("2026-08-31T00:00:00Z"));
    expect(got).toHaveLength(1);
    expect(got[0].resolved.program_id).toBe(1);
    expect(got[0].slot).toBe(slot);
  });

  it("omits an occurrence whose originating slot isn't in slotsById", () => {
    const got = slotsForDate(resolved, new Map(), new Date("2026-08-31T00:00:00Z"));
    expect(got).toHaveLength(0);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/scheduling/week.test.ts`
Expected: FAIL — `./week` module doesn't exist yet.

- [ ] **Step 3: Implement `web/src/scheduling/week.ts`**

```ts
import type { ResolvedSlot, Slot } from "../api/types";

export const MS_PER_DAY = 24 * 60 * 60 * 1000;

// Sunday 00:00 UTC on or before `date` — day_of_week 0 = Sunday throughout
// this codebase (see the implementation plan's Global Constraints),
// matching Date.getUTCDay() directly.
export function startOfWeekUTC(date: Date): Date {
  const midnight = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
  midnight.setUTCDate(midnight.getUTCDate() - midnight.getUTCDay());
  return midnight;
}

export function weekDates(weekStart: Date): Date[] {
  return Array.from({ length: 7 }, (_, i) => new Date(weekStart.getTime() + i * MS_PER_DAY));
}

export function addWeeks(date: Date, weeks: number): Date {
  return new Date(date.getTime() + weeks * 7 * MS_PER_DAY);
}

// Sparse-integer position for a new slot inserted at insertBeforeIndex
// among existingPositions (already known to be sorted the same way the
// caller will render them). Mirrors the backend's own sparse-position
// convention (increments of 1000) so most inserts never need to renumber
// sibling slots.
export function positionForInsert(existingPositions: number[], insertBeforeIndex: number): number {
  const sorted = [...existingPositions].sort((a, b) => a - b);
  if (sorted.length === 0) return 1000;
  if (insertBeforeIndex <= 0) return sorted[0] - 1000;
  if (insertBeforeIndex >= sorted.length) return sorted[sorted.length - 1] + 1000;
  return (sorted[insertBeforeIndex - 1] + sorted[insertBeforeIndex]) / 2;
}

export interface DaySlotBlock {
  slot: Slot;
  resolved: ResolvedSlot;
}

// Picks the resolved occurrences whose start falls on the UTC calendar date
// of `date`, joins each back to its originating Slot (for kind/gap_label/
// recurring, which ResolvedSlot alone doesn't carry), sorted by start time.
export function slotsForDate(resolved: ResolvedSlot[], slotsById: Map<number, Slot>, date: Date): DaySlotBlock[] {
  const dayStart = Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate());
  const dayEnd = dayStart + MS_PER_DAY;
  return resolved
    .filter((r) => {
      const t = new Date(r.start_time).getTime();
      return t >= dayStart && t < dayEnd;
    })
    .map((r) => ({ slot: slotsById.get(r.program_id), resolved: r }))
    .filter((b): b is DaySlotBlock => b.slot !== undefined)
    .sort((a, b) => new Date(a.resolved.start_time).getTime() - new Date(b.resolved.start_time).getTime());
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/scheduling/week.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing test for the read-only screen rendering**

Create (overwriting) `web/src/screens/ChannelScheduleScreen.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelScheduleScreen } from "./ChannelScheduleScreen";

const CHANNEL = { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "", updated_at: "" };
const MEDIA = [
  { id: 5, source_id: 1, rel_path: "a.mp4", title: "Fury", duration_sec: 3600, video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1, mod_time: "", invalid: false, created_at: "", updated_at: "" },
];
const SLOTS = [
  { id: 1, channel_id: 1, kind: "media", media_item_id: 5, gap_duration_sec: null, gap_label: "", recurring: true, day_of_week: 1, position: 1000, start_time: null, created_at: "", updated_at: "" },
];

function renderScreen() {
  server.use(
    http.get("/api/channels/1", () => HttpResponse.json(CHANNEL)),
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/channels/1/slots", () => HttpResponse.json(SLOTS)),
    http.get("/api/channels/1/slots/resolved", () =>
      HttpResponse.json([{ program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" }])
    )
  );
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={["/channels/1"]}>
      <Routes>
        <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("ChannelScheduleScreen", () => {
  it("renders the channel name, 7 day columns, and a media-library panel entry per media item", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    for (const day of ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]) {
      expect(screen.getByText(day, { exact: false })).toBeInTheDocument();
    }
    expect(await screen.findByText("Fury", { selector: ".media-library-item *, .media-library-item" })).toBeTruthy();
  });

  it("renders a resolved slot as a block on its day, falling back to the media title (no cover art wired up yet)", async () => {
    renderScreen();
    const block = await screen.findByTestId("slot-block-1");
    expect(block).toHaveTextContent("Fury");
    expect(block.querySelector("img")).toBeNull();
  });

  it("renders a gap slot's label instead of a media title", async () => {
    server.use(
      http.get("/api/channels/1/slots", () =>
        HttpResponse.json([
          { id: 2, channel_id: 1, kind: "gap", media_item_id: null, gap_duration_sec: 300, gap_label: "Ad Break", recurring: true, day_of_week: 1, position: 1000, start_time: null, created_at: "", updated_at: "" },
        ])
      ),
      http.get("/api/channels/1/slots/resolved", () =>
        HttpResponse.json([{ program_id: 2, media_item_id: 0, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T00:05:00Z" }])
      )
    );
    renderScreen();
    const block = await screen.findByTestId("slot-block-2");
    expect(block).toHaveTextContent("Ad Break");
  });
});
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: FAIL — the existing dropdown-based screen doesn't render day columns, a media panel, or `slot-block-*` test IDs.

- [ ] **Step 7: Implement the read-only screen**

Replace the contents of `web/src/screens/ChannelScheduleScreen.tsx` with:

```tsx
import { useState } from "react";
import { useParams } from "react-router-dom";
import { useChannel } from "../api/channels";
import { useMediaItems } from "../api/media";
import { useResolvedSlots, useSlotsForChannel } from "../api/slots";
import type { MediaItem } from "../api/types";
import { addWeeks, slotsForDate, startOfWeekUTC, weekDates } from "../scheduling/week";
import "./ChannelScheduleScreen.css";

const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MS_PER_HOUR = 60 * 60 * 1000;

// No cover-art metadata exists in the backend yet (deferred future work,
// per the design spec) — always undefined, so every slot block below falls
// back to its text title/label. Isolated here so wiring in a real field
// later is a one-line change, not a render-logic rewrite.
function mediaPosterUrl(_item: MediaItem | undefined): string | undefined {
  return undefined;
}

export function ChannelScheduleScreen() {
  const params = useParams<{ id: string }>();
  const channelId = Number(params.id);
  const { data: channel, isLoading: channelLoading, isError: channelError } = useChannel(channelId);
  const { data: media } = useMediaItems();
  const { data: slots } = useSlotsForChannel(channelId);

  const [weekAnchor] = useState(() => startOfWeekUTC(new Date()));
  const [weekOffset, setWeekOffset] = useState(0);
  const weekStart = addWeeks(weekAnchor, weekOffset);
  const weekEnd = addWeeks(weekStart, 1);
  const { data: resolved, isLoading: resolvedLoading, isError: resolvedError } = useResolvedSlots(
    channelId,
    weekStart.toISOString(),
    weekEnd.toISOString()
  );

  const mediaById = new Map((media ?? []).map((m) => [m.id, m]));
  const slotsById = new Map((slots ?? []).map((s) => [s.id, s]));
  const days = weekDates(weekStart);

  if (channelLoading || resolvedLoading) return <p>Loading schedule…</p>;
  if (channelError || resolvedError || !channel) return <p role="alert">Failed to load this channel's schedule.</p>;

  return (
    <section>
      <h1>{channel.name}</h1>
      <div className="week-nav">
        <button onClick={() => setWeekOffset((o) => o - 1)}>&lsaquo; Previous week</button>
        <span>{weekStart.toDateString()} – {new Date(weekEnd.getTime() - 1).toDateString()}</span>
        <button onClick={() => setWeekOffset((o) => o + 1)}>Next week &rsaquo;</button>
      </div>
      <div className="schedule-layout">
        <aside className="media-library-panel">
          <h2>Media library</h2>
          <ul>
            {(media ?? []).map((item) => (
              <li key={item.id} className="media-library-item" draggable>
                <span>{item.title}</span>
              </li>
            ))}
          </ul>
        </aside>
        <div className="week-grid">
          {days.map((day, i) => (
            <div className="day-column" key={day.toISOString()}>
              <div className="day-column-header">
                {DAY_LABELS[i]} {day.getUTCDate()}
              </div>
              {slotsForDate(resolved ?? [], slotsById, day).map(({ slot, resolved: r }) => {
                const heightPercent = ((new Date(r.end_time).getTime() - new Date(r.start_time).getTime()) / (24 * MS_PER_HOUR)) * 100;
                const mediaItem = slot.kind === "media" ? mediaById.get(slot.media_item_id ?? -1) : undefined;
                const label = slot.kind === "gap" ? slot.gap_label || "Gap" : mediaItem?.title ?? `Media #${slot.media_item_id}`;
                const posterUrl = mediaPosterUrl(mediaItem);
                return (
                  <div
                    key={r.program_id}
                    data-testid={`slot-block-${r.program_id}`}
                    className={`slot-block slot-block-${slot.kind}`}
                    style={{ height: `${heightPercent}%` }}
                  >
                    {posterUrl ? <img className="slot-block-poster" src={posterUrl} alt={label} /> : <span>{label}</span>}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: PASS.

- [ ] **Step 9: Add matching CSS**

Replace the contents of `web/src/screens/ChannelScheduleScreen.css` with:

```css
.week-nav {
  display: flex;
  align-items: center;
  gap: 16px;
  margin: 12px 0;
}

.schedule-layout {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.media-library-panel {
  width: 200px;
  flex-shrink: 0;
}

.media-library-panel ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.media-library-item {
  padding: 8px;
  border: 1px solid #333;
  border-radius: 4px;
  cursor: grab;
}

.week-grid {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 8px;
}

.day-column {
  display: flex;
  flex-direction: column;
  min-height: 600px;
  border: 1px solid #333;
  border-radius: 4px;
  overflow: hidden;
}

.day-column-header {
  padding: 6px;
  font-weight: 600;
  text-align: center;
  border-bottom: 1px solid #333;
}

.slot-block {
  padding: 4px 6px;
  font-size: 0.85rem;
  overflow: hidden;
  border-bottom: 1px solid rgba(255, 255, 255, 0.15);
}

.slot-block-media {
  background: #2a4d69;
}

.slot-block-gap {
  background: #4d3d2a;
  font-style: italic;
}

.slot-block-poster {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
```

- [ ] **Step 10: Run the full frontend verification**

Run: `cd web && npx tsc -b && npm run lint && npm test`
Expected: PASS (lint may still show the same 3 pre-existing benign warnings noted elsewhere in this codebase's docs — no new ones).

- [ ] **Step 11: Commit**

```bash
git add web/src/scheduling/week.ts web/src/scheduling/week.test.ts web/src/screens/ChannelScheduleScreen.tsx web/src/screens/ChannelScheduleScreen.css web/src/screens/ChannelScheduleScreen.test.tsx
git commit -m "feat: render the weekly slot-chain grid (read-only)"
```

---

## Task 10: Frontend — drag-and-drop insert/move with inline rejection errors

**Files:**
- Modify: `web/src/screens/ChannelScheduleScreen.tsx`
- Modify: `web/src/screens/ChannelScheduleScreen.test.tsx`

**Interfaces:**
- Consumes: `useAddSlot`, `useUpdateSlot` (Task 7), `positionForInsert` (Task 9), `MutationError` (existing, `web/src/components/MutationError.tsx`).

- [ ] **Step 1: Write the failing tests**

Add to `web/src/screens/ChannelScheduleScreen.test.tsx`:

```tsx
import { fireEvent } from "@testing-library/react";

function dragEventWithData(data: unknown) {
  return { dataTransfer: { getData: () => JSON.stringify(data), setData: () => {} } };
}

it("drops a media item from the library panel onto an empty day, adding a recurring slot at position 1000", async () => {
  server.use(
    http.get("/api/channels/1/slots", () => HttpResponse.json([])),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([])),
    http.post("/api/channels/1/slots", async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      expect(body).toMatchObject({ kind: "media", media_item_id: 5, recurring: true, position: 1000 });
      return HttpResponse.json({ id: 9, ...body }, { status: 201 });
    })
  );
  renderScreen();

  const mediaItem = await screen.findByText("Fury");
  const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
  fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
  fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));
});

it("shows an inline error when the backend rejects a placement", async () => {
  server.use(
    http.get("/api/channels/1/slots", () => HttpResponse.json([])),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([])),
    http.post("/api/channels/1/slots", () => HttpResponse.json({ error: "doesn't fit: this day is already full" }, { status: 400 }))
  );
  renderScreen();

  const mediaItem = await screen.findByText("Fury");
  const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
  fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
  fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));

  expect(await screen.findByRole("alert")).toHaveTextContent("doesn't fit: this day is already full");
});

it("moves an existing slot when it's dragged to a different drop zone", async () => {
  server.use(
    http.get("/api/channels/1/slots", () => HttpResponse.json(SLOTS)),
    http.get("/api/channels/1/slots/resolved", () =>
      HttpResponse.json([{ program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" }])
    ),
    http.put("/api/slots/1", async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      expect(body).toMatchObject({ recurring: true, position: 2000 });
      return HttpResponse.json({ id: 1, ...body });
    })
  );
  renderScreen();

  const block = await screen.findByTestId("slot-block-1");
  const endZone = await screen.findByTestId("day-drop-zone-1-end");
  fireEvent.dragStart(block, dragEventWithData({ existingSlotId: 1 }));
  fireEvent.drop(endZone, dragEventWithData({ existingSlotId: 1 }));
});
```

Add `import { fireEvent } from "@testing-library/react";` (or fold it into the existing `render, screen` import line) at the top of the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: FAIL — no drop zones or drag handlers exist yet.

- [ ] **Step 3: Implement drag-and-drop**

In `web/src/screens/ChannelScheduleScreen.tsx`:

Add imports:

```ts
import type { DragEvent } from "react";
import { MutationError } from "../components/MutationError";
import { useAddSlot, useUpdateSlot } from "../api/slots";
```

Add, inside the component, after the existing hooks:

```ts
  const addSlotMutation = useAddSlot(channelId);
  const updateSlotMutation = useUpdateSlot(channelId);

  function handleDragStartMedia(e: DragEvent, mediaItemId: number) {
    e.dataTransfer.setData("application/json", JSON.stringify({ mediaItemId }));
  }

  function handleDragStartSlot(e: DragEvent, existingSlotId: number) {
    e.dataTransfer.setData("application/json", JSON.stringify({ existingSlotId }));
  }

  function handleDrop(e: DragEvent, dayOfWeek: number, insertBeforeIndex: number) {
    e.preventDefault();
    const payload = JSON.parse(e.dataTransfer.getData("application/json")) as
      | { mediaItemId: number }
      | { existingSlotId: number };

    const daySlotIds = new Set(
      (slots ?? []).filter((s) => s.recurring && s.day_of_week === dayOfWeek).map((s) => s.id)
    );
    const existingPositions = (slots ?? [])
      .filter((s) => daySlotIds.has(s.id) && ("existingSlotId" in payload ? s.id !== payload.existingSlotId : true))
      .map((s) => s.position ?? 0);
    const position = positionForInsert(existingPositions, insertBeforeIndex);

    if ("mediaItemId" in payload) {
      addSlotMutation.mutate({
        channelId,
        kind: "media",
        media_item_id: payload.mediaItemId,
        recurring: true,
        day_of_week: dayOfWeek,
        position,
      });
      return;
    }

    const existing = slotsById.get(payload.existingSlotId);
    if (!existing) return;
    updateSlotMutation.mutate({
      id: existing.id,
      channelId,
      kind: existing.kind,
      media_item_id: existing.media_item_id ?? undefined,
      gap_duration_sec: existing.gap_duration_sec ?? undefined,
      gap_label: existing.gap_label,
      recurring: true,
      day_of_week: dayOfWeek,
      position,
    });
  }
```

Update the `positionForInsert` import at the top to include it alongside the existing `week` imports:

```ts
import { addWeeks, positionForInsert, slotsForDate, startOfWeekUTC, weekDates } from "../scheduling/week";
```

In the JSX, make each media-library item draggable with a real handler, and add drop zones + `draggable` to rendered slot blocks, and render `MutationError` once for the two mutations:

```tsx
        <aside className="media-library-panel">
          <h2>Media library</h2>
          <ul>
            {(media ?? []).map((item) => (
              <li
                key={item.id}
                className="media-library-item"
                draggable
                onDragStart={(e) => handleDragStartMedia(e, item.id)}
              >
                <span>{item.title}</span>
              </li>
            ))}
          </ul>
        </aside>
        <div className="week-grid">
          <MutationError isError={addSlotMutation.isError} error={addSlotMutation.error} />
          <MutationError isError={updateSlotMutation.isError} error={updateSlotMutation.error} />
          {days.map((day, i) => {
            const dayOfWeek = day.getUTCDay();
            const daySlots = slotsForDate(resolved ?? [], slotsById, day);
            return (
              <div className="day-column" key={day.toISOString()}>
                <div className="day-column-header">
                  {DAY_LABELS[i]} {day.getUTCDate()}
                </div>
                <div
                  className="day-drop-zone"
                  data-testid={`day-drop-zone-${dayOfWeek}-start`}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => handleDrop(e, dayOfWeek, 0)}
                />
                {daySlots.map(({ slot, resolved: r }, index) => {
                  const heightPercent = ((new Date(r.end_time).getTime() - new Date(r.start_time).getTime()) / (24 * MS_PER_HOUR)) * 100;
                  const mediaItem = slot.kind === "media" ? mediaById.get(slot.media_item_id ?? -1) : undefined;
                  const label = slot.kind === "gap" ? slot.gap_label || "Gap" : mediaItem?.title ?? `Media #${slot.media_item_id}`;
                  const posterUrl = mediaPosterUrl(mediaItem);
                  return (
                    <div key={r.program_id}>
                      <div
                        data-testid={`slot-block-${r.program_id}`}
                        className={`slot-block slot-block-${slot.kind}`}
                        style={{ height: `${heightPercent}%` }}
                        draggable
                        onDragStart={(e) => handleDragStartSlot(e, slot.id)}
                      >
                        {posterUrl ? <img className="slot-block-poster" src={posterUrl} alt={label} /> : <span>{label}</span>}
                      </div>
                      <div
                        className="day-drop-zone"
                        data-testid={`day-drop-zone-${dayOfWeek}-${index === daySlots.length - 1 ? "end" : index}`}
                        onDragOver={(e) => e.preventDefault()}
                        onDrop={(e) => handleDrop(e, dayOfWeek, index + 1)}
                      />
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: PASS.

- [ ] **Step 5: Add drop-zone CSS**

Add to `web/src/screens/ChannelScheduleScreen.css`:

```css
.day-drop-zone {
  min-height: 8px;
}

.day-drop-zone:hover {
  background: rgba(255, 255, 255, 0.08);
}
```

- [ ] **Step 6: Run the full frontend verification**

Run: `cd web && npx tsc -b && npm run lint && npm test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/screens/ChannelScheduleScreen.tsx web/src/screens/ChannelScheduleScreen.css web/src/screens/ChannelScheduleScreen.test.tsx
git commit -m "feat: add drag-and-drop insert/move to the weekly schedule grid"
```

---

## Task 11: Frontend — gap-slot creation + recurring/one-off toggle

**Files:**
- Modify: `web/src/screens/ChannelScheduleScreen.tsx`
- Modify: `web/src/screens/ChannelScheduleScreen.test.tsx`

**Interfaces:**
- Consumes: `useAddSlot` (Task 7/10).

This task adds the two remaining pieces from the design spec: a way to place a gap/break slot (with a user-chosen duration), and a way to place either kind of slot as one-off (bound to the exact date of the column dropped on) instead of recurring.

- [ ] **Step 1: Write the failing tests**

Add to `web/src/screens/ChannelScheduleScreen.test.tsx`:

```tsx
it("dropping the Gap entry opens a duration prompt, and confirming adds a recurring gap slot", async () => {
  server.use(
    http.get("/api/channels/1/slots", () => HttpResponse.json([])),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([])),
    http.post("/api/channels/1/slots", async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      expect(body).toMatchObject({ kind: "gap", gap_duration_sec: 300, recurring: true, position: 1000 });
      return HttpResponse.json({ id: 9, ...body }, { status: 201 });
    })
  );
  renderScreen();

  const gapEntry = await screen.findByText("Gap / Break");
  const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
  fireEvent.dragStart(gapEntry, dragEventWithData({ gap: true }));
  fireEvent.drop(dropZone, dragEventWithData({ gap: true }));

  fireEvent.change(await screen.findByLabelText("Gap duration (minutes)"), { target: { value: "5" } });
  fireEvent.click(screen.getByText("Add gap"));
});

it("unchecking Repeats weekly places a one-off slot with the dropped column's exact date instead of day_of_week/position", async () => {
  server.use(
    http.get("/api/channels/1/slots", () => HttpResponse.json([])),
    http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([])),
    http.post("/api/channels/1/slots", async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      expect(body).toMatchObject({ kind: "media", media_item_id: 5, recurring: false });
      expect(body.start_time).toBeTruthy();
      expect(body.day_of_week).toBeUndefined();
      expect(body.position).toBeUndefined();
      return HttpResponse.json({ id: 9, ...body }, { status: 201 });
    })
  );
  renderScreen();

  const mediaItem = await screen.findByText("Fury");
  const dropZone = (await screen.findAllByTestId(/day-drop-zone-/))[0];
  fireEvent.dragStart(mediaItem, dragEventWithData({ mediaItemId: 5 }));
  fireEvent.drop(dropZone, dragEventWithData({ mediaItemId: 5 }));

  fireEvent.click(await screen.findByLabelText("Repeats weekly"));
  fireEvent.click(screen.getByText("Add"));
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: FAIL — no Gap entry, no placement confirmation form exists yet.

- [ ] **Step 3: Implement**

In `web/src/screens/ChannelScheduleScreen.tsx`, replace `handleDrop` and the drop-triggered mutation calls with a two-step flow: a drop stages a **pending placement** (what was dropped, and where), rendered as a small inline confirmation form; confirming it fires the actual mutation with the chosen recurring/one-off + (for gap) duration.

Add a type and state near the other hooks:

```ts
type PendingPlacement =
  | { kind: "media"; mediaItemId?: number; dayOfWeek: number; date: Date; insertBeforeIndex: number; existingSlotId?: number }
  | { kind: "gap"; dayOfWeek: number; date: Date; insertBeforeIndex: number; existingSlotId?: number };

const [pending, setPending] = useState<PendingPlacement | null>(null);
const [pendingRecurring, setPendingRecurring] = useState(true);
const [pendingGapMinutes, setPendingGapMinutes] = useState("5");
```

Replace `handleDrop` with a version that only stages `pending` (no mutation yet):

```ts
  function handleDrop(e: DragEvent, dayOfWeek: number, date: Date, insertBeforeIndex: number) {
    e.preventDefault();
    const payload = JSON.parse(e.dataTransfer.getData("application/json")) as
      | { mediaItemId: number }
      | { existingSlotId: number }
      | { gap: true };

    setPendingRecurring(true);
    if ("gap" in payload) {
      setPending({ kind: "gap", dayOfWeek, date, insertBeforeIndex });
      return;
    }
    if ("mediaItemId" in payload) {
      setPending({ kind: "media", mediaItemId: payload.mediaItemId, dayOfWeek, date, insertBeforeIndex });
      return;
    }
    const existing = slotsById.get(payload.existingSlotId);
    if (!existing) return;
    if (existing.kind === "gap") {
      setPending({ kind: "gap", dayOfWeek, date, insertBeforeIndex, existingSlotId: existing.id });
    } else {
      setPending({
        kind: "media",
        mediaItemId: existing.media_item_id ?? undefined,
        dayOfWeek,
        date,
        insertBeforeIndex,
        existingSlotId: existing.id,
      });
    }
  }

  function computePosition(dayOfWeek: number, insertBeforeIndex: number, excludeSlotId?: number) {
    const existingPositions = (slots ?? [])
      .filter((s) => s.recurring && s.day_of_week === dayOfWeek && s.id !== excludeSlotId)
      .map((s) => s.position ?? 0);
    return positionForInsert(existingPositions, insertBeforeIndex);
  }

  function computeOneOffStartTime(date: Date, insertBeforeIndex: number): string {
    const dayBlocks = slotsForDate(resolved ?? [], slotsById, date);
    if (insertBeforeIndex <= 0) return date.toISOString();
    const previous = dayBlocks[insertBeforeIndex - 1];
    return previous ? previous.resolved.end_time : date.toISOString();
  }

  function confirmPending() {
    if (!pending) return;
    const base = {
      channelId,
      recurring: pendingRecurring,
      ...(pendingRecurring
        ? { day_of_week: pending.dayOfWeek, position: computePosition(pending.dayOfWeek, pending.insertBeforeIndex, pending.existingSlotId) }
        : { start_time: computeOneOffStartTime(pending.date, pending.insertBeforeIndex) }),
    };
    const body =
      pending.kind === "gap"
        ? { ...base, kind: "gap" as const, gap_duration_sec: Number(pendingGapMinutes) * 60, gap_label: "Gap" }
        : { ...base, kind: "media" as const, media_item_id: pending.mediaItemId! };

    if (pending.existingSlotId !== undefined) {
      const existing = slotsById.get(pending.existingSlotId)!;
      updateSlotMutation.mutate({ id: existing.id, ...body });
    } else {
      addSlotMutation.mutate(body);
    }
    setPending(null);
  }
```

Update `handleDrop`'s call sites in the JSX to pass `date` (the day column's own `Date`) alongside `dayOfWeek`, e.g. `onDrop={(e) => handleDrop(e, dayOfWeek, day, 0)}` and `onDrop={(e) => handleDrop(e, dayOfWeek, day, index + 1)}`.

Add the Gap entry to the media-library panel (above the media items list):

```tsx
            <li
              className="media-library-item media-library-gap-item"
              draggable
              onDragStart={(e) => e.dataTransfer.setData("application/json", JSON.stringify({ gap: true }))}
            >
              <span>Gap / Break</span>
            </li>
```

Render the pending-placement confirmation form once, near the top of `.week-grid` (alongside the two `MutationError`s):

```tsx
          {pending && (
            <div className="pending-placement-form" role="dialog" aria-label="Confirm placement">
              <label>
                <input
                  type="checkbox"
                  checked={pendingRecurring}
                  onChange={(e) => setPendingRecurring(e.target.checked)}
                  aria-label="Repeats weekly"
                />
                Repeats weekly
              </label>
              {pending.kind === "gap" && (
                <label>
                  Gap duration (minutes)
                  <input
                    type="number"
                    min={1}
                    value={pendingGapMinutes}
                    onChange={(e) => setPendingGapMinutes(e.target.value)}
                    aria-label="Gap duration (minutes)"
                  />
                </label>
              )}
              <button onClick={confirmPending}>{pending.kind === "gap" ? "Add gap" : "Add"}</button>
              <button onClick={() => setPending(null)}>Cancel</button>
            </div>
          )}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/screens/ChannelScheduleScreen.test.tsx`
Expected: PASS.

- [ ] **Step 5: Add matching CSS**

Add to `web/src/screens/ChannelScheduleScreen.css`:

```css
.media-library-gap-item {
  border-style: dashed;
}

.pending-placement-form {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px;
  border: 1px solid #333;
  border-radius: 4px;
  margin-bottom: 8px;
}
```

- [ ] **Step 6: Run the full frontend verification**

Run: `cd web && npx tsc -b && npm run lint && npm test && npm run build`
Expected: PASS (same 3 pre-existing benign lint warnings, nothing new; production build succeeds).

- [ ] **Step 7: Run the full backend verification one more time (nothing in this task should have touched it, but confirm no drift)**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/screens/ChannelScheduleScreen.tsx web/src/screens/ChannelScheduleScreen.css web/src/screens/ChannelScheduleScreen.test.tsx
git commit -m "feat: add gap-slot creation and recurring/one-off placement toggle"
```

---

## Task 12: Update `docs/PROGRESS.md` and close out the design spec's status

**Files:**
- Modify: `docs/PROGRESS.md`

- [ ] **Step 1: Update the status**

Update the "Recurring slot-chain scheduling design is approved, no implementation plan yet" line in `docs/PROGRESS.md`'s "Next step" section to reflect that the plan has been implemented (move it out of "Next step" into the "Status" section, following this file's existing pattern for a completed plan — see how Plans 1-3 are described there). Note the new `Slot` model/API/routes replacing `Program`, and that the Channels schedule editor is now the drag-and-drop weekly grid (no more dropdown-based add-program form).

- [ ] **Step 2: Commit**

```bash
git add docs/PROGRESS.md
git commit -m "docs: mark recurring slot-chain scheduling plan implemented"
```
