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
