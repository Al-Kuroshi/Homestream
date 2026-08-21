package channels

import (
	"context"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/repository"
	"personaltv/internal/scheduler"
)

type Service struct {
	channels repository.ChannelRepository
	programs repository.ProgramRepository
	items    repository.MediaItemRepository
}

func NewService(channels repository.ChannelRepository, programs repository.ProgramRepository, items repository.MediaItemRepository) *Service {
	return &Service{channels: channels, programs: programs, items: items}
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

func (s *Service) AddProgram(ctx context.Context, p *model.Program) error {
	return s.programs.Create(ctx, p)
}

func (s *Service) GetProgram(ctx context.Context, id int64) (*model.Program, error) {
	return s.programs.Get(ctx, id)
}

func (s *Service) UpdateProgram(ctx context.Context, p *model.Program) error {
	return s.programs.Update(ctx, p)
}

func (s *Service) RemoveProgram(ctx context.Context, id int64) error {
	return s.programs.Delete(ctx, id)
}

func (s *Service) ListPrograms(ctx context.Context, channelID int64) ([]*model.Program, error) {
	return s.programs.ListByChannel(ctx, channelID)
}

// CurrentState reports what's currently playing on a channel (if anything)
// and what plays next, as of now. Each call recomputes from persisted
// schedule + media duration — nothing is cached or ticking in the
// background, so this is correct even immediately after an app restart.
func (s *Service) CurrentState(ctx context.Context, channelID int64, now time.Time) (scheduler.CurrentState, error) {
	programs, err := s.programs.ListByChannel(ctx, channelID)
	if err != nil {
		return scheduler.CurrentState{}, err
	}

	scheduled := make([]scheduler.ScheduledProgram, 0, len(programs))
	for _, p := range programs {
		item, err := s.items.Get(ctx, p.MediaItemID)
		if err != nil {
			return scheduler.CurrentState{}, err
		}
		scheduled = append(scheduled, scheduler.ScheduledProgram{
			ProgramID:   p.ID,
			MediaItemID: p.MediaItemID,
			StartTime:   p.StartTime,
			Duration:    time.Duration(item.DurationSec * float64(time.Second)),
		})
	}

	return scheduler.Evaluate(scheduled, now), nil
}
