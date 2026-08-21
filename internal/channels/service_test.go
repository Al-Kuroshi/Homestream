package channels_test

import (
	"context"
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestService_CurrentState(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

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
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := programRepo.Create(ctx, program); err != nil {
		t.Fatalf("failed to create program: %v", err)
	}

	svc := channels.NewService(channelRepo, programRepo, itemRepo)

	state, err := svc.CurrentState(ctx, channel.ID, start.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != program.ID {
		t.Fatalf("expected program %d to be current, got %+v", program.ID, state.Current)
	}
	if state.Offset != 30*time.Minute {
		t.Errorf("expected offset 30m, got %v", state.Offset)
	}
}

func TestService_CurrentState_NoPrograms(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	svc := channels.NewService(channelRepo, programRepo, itemRepo)
	state, err := svc.CurrentState(ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current != nil || state.Next != nil {
		t.Fatalf("expected empty state for a channel with no programs, got %+v", state)
	}
}

// TestService_CurrentState_SkipsMissingAndInvalidMediaItems covers the
// binding constraint that a single unavailable/invalid media item must never
// abort a whole channel's schedule computation. A channel carries three
// programs — one whose media item was deleted out from under it, one whose
// media item is marked Invalid, and one healthy one — and the healthy one
// must still be reported correctly.
func TestService_CurrentState_SkipsMissingAndInvalidMediaItems(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

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
	ghostProgram := &model.Program{ChannelID: channel.ID, MediaItemID: ghost.ID, StartTime: base}
	brokenProgram := &model.Program{ChannelID: channel.ID, MediaItemID: broken.ID, StartTime: base.Add(time.Hour)}
	goodProgram := &model.Program{ChannelID: channel.ID, MediaItemID: good.ID, StartTime: base.Add(2 * time.Hour)}
	for _, p := range []*model.Program{ghostProgram, brokenProgram, goodProgram} {
		if err := programRepo.Create(ctx, p); err != nil {
			t.Fatalf("failed to create program: %v", err)
		}
	}

	// Orphan ghostProgram by deleting its media item with foreign keys
	// disabled on one pinned connection. Normally ON DELETE CASCADE would
	// take the program with it; this reproduces the state a database written
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

	svc := channels.NewService(channelRepo, programRepo, itemRepo)

	// During the orphaned slot's window the channel is simply off-air, and
	// the next healthy program is still reported.
	state, err := svc.CurrentState(ctx, channel.ID, base.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error despite a missing media item: %v", err)
	}
	if state.Current != nil {
		t.Errorf("expected no current program while the orphaned slot's window is active, got %+v", state.Current)
	}
	if state.Next == nil || state.Next.ProgramID != goodProgram.ID {
		t.Fatalf("expected the healthy program %d to be next, got %+v", goodProgram.ID, state.Next)
	}

	// And the healthy program still computes correctly during its own window.
	state, err = svc.CurrentState(ctx, channel.ID, base.Add(2*time.Hour+15*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != goodProgram.ID {
		t.Fatalf("expected program %d to be current, got %+v", goodProgram.ID, state.Current)
	}
	if state.Offset != 15*time.Minute {
		t.Errorf("expected offset 15m, got %v", state.Offset)
	}
}

func TestService_ProgramCRUD(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	sourceRepo.Create(ctx, source)
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "A", DurationSec: 60, ModTime: time.Now().UTC()}
	itemRepo.Upsert(ctx, item)
	channel := &model.Channel{Name: "Movies", Enabled: true}
	channelRepo.Create(ctx, channel)

	svc := channels.NewService(channelRepo, programRepo, itemRepo)

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := svc.AddProgram(ctx, program); err != nil {
		t.Fatalf("AddProgram returned error: %v", err)
	}

	fetched, err := svc.GetProgram(ctx, program.ID)
	if err != nil {
		t.Fatalf("GetProgram returned error: %v", err)
	}
	if fetched.ChannelID != channel.ID {
		t.Errorf("expected channel ID %d, got %d", channel.ID, fetched.ChannelID)
	}

	if err := svc.RemoveProgram(ctx, program.ID); err != nil {
		t.Fatalf("RemoveProgram returned error: %v", err)
	}

	list, err := svc.ListPrograms(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListPrograms returned error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no programs after removal, got %+v", list)
	}
}
