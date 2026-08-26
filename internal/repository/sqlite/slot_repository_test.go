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
