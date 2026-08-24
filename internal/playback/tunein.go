package playback

import (
	"context"
	"errors"
	"os"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/repository"
	"personaltv/internal/scheduler"
)

// TuneInResult is what a viewer sees when they tune in to a channel right
// now.
type TuneInResult struct {
	Status      string // "playing", "off_air", or "unavailable"
	Mode        string // "direct" or "hls"; empty unless Status == "playing"
	MediaItemID int64
	OffsetSec   float64
	SessionID   string // only set when Mode == "hls"
}

// TuneIn decides what a viewer sees when they tune in to channelID right
// now. It stays a pure function of (schedule, wall-clock time, which
// files exist on disk) — it never advances to a program before its own
// scheduled start time, matching this product's core principle that
// channel state is always recomputed from (schedule, now), never tracked
// as separate mutable state. A file missing at its scheduled slot is
// therefore treated exactly like an invalid/zero-duration item already is
// (see channels.Service.CurrentState): excluded from scheduling for this
// evaluation, never "skipped ahead to."
func (svc *Service) TuneIn(ctx context.Context, channelID int64, now time.Time) (*TuneInResult, error) {
	// Pass A: exactly what channels.CurrentState computes (schedulable per
	// the DB — not yet filtered by file existence). If nothing is
	// scheduled at all right now, this is off-air, identical to what
	// GET /api/channels/{id}/now already reports — there's no candidate to
	// even check for file existence.
	stateA, err := svc.channels.CurrentState(ctx, channelID, now)
	if err != nil {
		return nil, err
	}
	if stateA.Current == nil {
		return &TuneInResult{Status: "off_air"}, nil
	}

	// Pass B: the same schedule, additionally excluding any program whose
	// underlying file doesn't exist on disk right now.
	programs, err := svc.channels.ListPrograms(ctx, channelID)
	if err != nil {
		return nil, err
	}

	playable := make([]scheduler.ScheduledProgram, 0, len(programs))
	itemsByID := make(map[int64]*model.MediaItem, len(programs))
	sourcesByID := make(map[int64]*model.MediaSource)
	for _, p := range programs {
		item, err := svc.items.Get(ctx, p.MediaItemID)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if item.Invalid || item.DurationSec <= 0 {
			continue
		}
		source, ok := sourcesByID[item.SourceID]
		if !ok {
			source, err = svc.sources.Get(ctx, item.SourceID)
			if err != nil {
				continue // source itself gone; item can't be playable
			}
			sourcesByID[item.SourceID] = source
		}
		if _, statErr := os.Stat(ResolvePath(source, item)); statErr != nil {
			continue // file missing: excluded from scheduling, not skipped-to
		}

		itemsByID[item.ID] = item
		playable = append(playable, scheduler.ScheduledProgram{
			ProgramID:   p.ID,
			MediaItemID: p.MediaItemID,
			StartTime:   p.StartTime,
			Duration:    time.Duration(item.DurationSec * float64(time.Second)),
		})
	}

	stateB := scheduler.Evaluate(playable, now)
	if stateB.Current == nil {
		// Something IS scheduled right now per pass A, but its file (and
		// nothing else covering this exact moment) is playable.
		return &TuneInResult{Status: "unavailable"}, nil
	}

	item := itemsByID[stateB.Current.MediaItemID]
	source := sourcesByID[item.SourceID]
	offsetSec := stateB.Offset.Seconds()

	if IsDirectPlayCompatible(item.VideoCodec, item.AudioCodec, item.Container) {
		return &TuneInResult{
			Status: "playing", Mode: "direct",
			MediaItemID: item.ID, OffsetSec: offsetSec,
		}, nil
	}

	sess, err := svc.sessions.StartSession(ResolvePath(source, item), offsetSec)
	if err != nil {
		return nil, err
	}
	return &TuneInResult{
		Status: "playing", Mode: "hls",
		MediaItemID: item.ID, OffsetSec: offsetSec, SessionID: sess.ID,
	}, nil
}
