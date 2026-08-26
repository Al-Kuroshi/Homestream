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
