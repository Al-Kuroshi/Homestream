package channels

import (
	"context"
	"errors"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/repository"
	"personaltv/internal/scheduler"
)

type Service struct {
	channels repository.ChannelRepository
	slots    repository.SlotRepository
	items    repository.MediaItemRepository
}

func NewService(channels repository.ChannelRepository, slots repository.SlotRepository, items repository.MediaItemRepository) *Service {
	return &Service{channels: channels, slots: slots, items: items}
}

func (s *Service) CreateChannel(ctx context.Context, c *model.Channel) error {
	return s.channels.Create(ctx, c)
}

func (s *Service) ListChannels(ctx context.Context) ([]*model.Channel, error) {
	return s.channels.List(ctx)
}

func (s *Service) GetChannel(ctx context.Context, id int64) (*model.Channel, error) {
	return s.channels.Get(ctx, id)
}

func (s *Service) UpdateChannel(ctx context.Context, c *model.Channel) error {
	return s.channels.Update(ctx, c)
}

func (s *Service) DeleteChannel(ctx context.Context, id int64) error {
	return s.channels.Delete(ctx, id)
}

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

// CurrentState reports what's currently playing on a channel (if anything)
// and what plays next, as of now. Each call recomputes from persisted
// schedule + media duration — nothing is cached or ticking in the
// background, so this is correct even immediately after an app restart.
func (s *Service) CurrentState(ctx context.Context, channelID int64, now time.Time) (scheduler.CurrentState, error) {
	slots, err := s.slots.ListByChannel(ctx, channelID)
	if err != nil {
		return scheduler.CurrentState{}, err
	}

	// Interim: only resolves one-off slots directly (Task 5 replaces this
	// whole body with real ResolveDate-based resolution covering recurring
	// slots too).
	scheduled := make([]scheduler.ScheduledProgram, 0, len(slots))
	for _, sl := range slots {
		if sl.Recurring || sl.StartTime == nil || sl.MediaItemID == nil {
			continue
		}
		item, err := s.items.Get(ctx, *sl.MediaItemID)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return scheduler.CurrentState{}, err
		}
		if item.Invalid || item.DurationSec <= 0 {
			continue
		}
		scheduled = append(scheduled, scheduler.ScheduledProgram{
			ProgramID:   sl.ID,
			MediaItemID: *sl.MediaItemID,
			StartTime:   *sl.StartTime,
			Duration:    time.Duration(item.DurationSec * float64(time.Second)),
		})
	}

	return scheduler.Evaluate(scheduled, now), nil
}
