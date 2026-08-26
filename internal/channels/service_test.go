package channels_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

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

func TestService_CurrentState(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600, ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("failed to upsert item: %v", err)
	}

	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: false, StartTime: &start}
	if err := slotRepo.Create(ctx, slot); err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	svc := channels.NewService(channelRepo, slotRepo, itemRepo)

	state, err := svc.CurrentState(ctx, channel.ID, start.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != slot.ID {
		t.Fatalf("expected slot %d to be current, got %+v", slot.ID, state.Current)
	}
	if state.Offset != 30*time.Minute {
		t.Errorf("expected offset 30m, got %v", state.Offset)
	}
}

func TestService_CurrentState_NoPrograms(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	svc := channels.NewService(channelRepo, slotRepo, itemRepo)
	state, err := svc.CurrentState(ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current != nil || state.Next != nil {
		t.Fatalf("expected empty state for a channel with no slots, got %+v", state)
	}
}

// TestService_CurrentState_SkipsMissingAndInvalidMediaItems covers the
// binding constraint that a single unavailable/invalid media item must never
// abort a whole channel's schedule computation. A channel carries three
// one-off slots — one whose media item was deleted out from under it, one
// whose media item is marked Invalid, and one healthy one — and the healthy
// one must still be reported correctly.
func TestService_CurrentState_SkipsMissingAndInvalidMediaItems(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	ghost := &model.MediaItem{SourceID: source.ID, RelPath: "ghost.mp4", Title: "Ghost", DurationSec: 3600, ModTime: time.Now().UTC()}
	broken := &model.MediaItem{SourceID: source.ID, RelPath: "broken.mp4", Title: "Broken", DurationSec: 3600, Invalid: true, ModTime: time.Now().UTC()}
	good := &model.MediaItem{SourceID: source.ID, RelPath: "good.mp4", Title: "Good", DurationSec: 3600, ModTime: time.Now().UTC()}
	for _, item := range []*model.MediaItem{ghost, broken, good} {
		if err := itemRepo.Upsert(ctx, item); err != nil {
			t.Fatalf("failed to upsert %s: %v", item.RelPath, err)
		}
	}

	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	base := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	ghostStart := base
	brokenStart := base.Add(time.Hour)
	goodStart := base.Add(2 * time.Hour)
	ghostSlot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &ghost.ID, Recurring: false, StartTime: &ghostStart}
	brokenSlot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &broken.ID, Recurring: false, StartTime: &brokenStart}
	goodSlot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &good.ID, Recurring: false, StartTime: &goodStart}
	for _, sl := range []*model.Slot{ghostSlot, brokenSlot, goodSlot} {
		if err := slotRepo.Create(ctx, sl); err != nil {
			t.Fatalf("failed to create slot: %v", err)
		}
	}

	// Orphan ghostSlot by deleting its media item with foreign keys
	// disabled on one pinned connection. Normally ON DELETE CASCADE would
	// take the slot with it; this reproduces the state a database written
	// before foreign keys were enforced can still be in.
	pinned, err := conn.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to pin a connection: %v", err)
	}
	if _, err := pinned.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("failed to disable foreign keys: %v", err)
	}
	if _, err := pinned.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, ghost.ID); err != nil {
		t.Fatalf("failed to delete ghost media item: %v", err)
	}
	if err := pinned.Close(); err != nil {
		t.Fatalf("failed to release pinned connection: %v", err)
	}

	svc := channels.NewService(channelRepo, slotRepo, itemRepo)

	// During the orphaned slot's window the channel is simply off-air, and
	// the next healthy slot is still reported.
	state, err := svc.CurrentState(ctx, channel.ID, base.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error despite a missing media item: %v", err)
	}
	if state.Current != nil {
		t.Errorf("expected no current slot while the orphaned slot's window is active, got %+v", state.Current)
	}
	if state.Next == nil || state.Next.ProgramID != goodSlot.ID {
		t.Fatalf("expected the healthy slot %d to be next, got %+v", goodSlot.ID, state.Next)
	}

	// And the healthy slot still computes correctly during its own window.
	state, err = svc.CurrentState(ctx, channel.ID, base.Add(2*time.Hour+15*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != goodSlot.ID {
		t.Fatalf("expected slot %d to be current, got %+v", goodSlot.ID, state.Current)
	}
	if state.Offset != 15*time.Minute {
		t.Errorf("expected offset 15m, got %v", state.Offset)
	}
}

func TestService_SlotCRUD(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	sourceRepo.Create(ctx, source)
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "A", DurationSec: 60, ModTime: time.Now().UTC()}
	itemRepo.Upsert(ctx, item)
	channel := &model.Channel{Name: "Movies", Enabled: true}
	channelRepo.Create(ctx, channel)

	svc := channels.NewService(channelRepo, slotRepo, itemRepo)

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &item.ID, Recurring: false, StartTime: &start}
	if err := svc.AddSlot(ctx, slot); err != nil {
		t.Fatalf("AddSlot returned error: %v", err)
	}

	fetched, err := svc.GetSlot(ctx, slot.ID)
	if err != nil {
		t.Fatalf("GetSlot returned error: %v", err)
	}
	if fetched.ChannelID != channel.ID {
		t.Errorf("expected channel ID %d, got %d", channel.ID, fetched.ChannelID)
	}

	if err := svc.RemoveSlot(ctx, slot.ID); err != nil {
		t.Fatalf("RemoveSlot returned error: %v", err)
	}

	list, err := svc.ListSlots(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListSlots returned error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no slots after removal, got %+v", list)
	}
}

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
