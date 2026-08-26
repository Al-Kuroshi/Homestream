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
