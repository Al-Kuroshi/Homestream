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

func setupChannelAndMediaItem(t *testing.T, ctx context.Context, conn *sql.DB) (*model.Channel, *model.MediaItem) {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

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
	return channel, item
}

func TestProgramRepository_CreateGetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel, item := setupChannelAndMediaItem(t, ctx, conn)
	repo := sqlite.NewProgramRepository(conn)

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := repo.Create(ctx, program); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if program.ID == 0 {
		t.Fatal("expected Create to set an ID")
	}

	fetched, err := repo.Get(ctx, program.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !fetched.StartTime.Equal(start) {
		t.Errorf("expected start time %v, got %v", start, fetched.StartTime)
	}

	newStart := start.Add(time.Hour)
	fetched.StartTime = newStart
	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	updated, err := repo.Get(ctx, program.ID)
	if err != nil {
		t.Fatalf("Get after update returned error: %v", err)
	}
	if !updated.StartTime.Equal(newStart) {
		t.Errorf("expected updated start time %v, got %v", newStart, updated.StartTime)
	}

	if err := repo.Delete(ctx, program.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := repo.Get(ctx, program.ID); err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestProgramRepository_ListByChannelOrdersByStartTime(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel, item := setupChannelAndMediaItem(t, ctx, conn)
	repo := sqlite.NewProgramRepository(conn)

	base := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	later := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: base.Add(2 * time.Hour)}
	earlier := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: base}
	if err := repo.Create(ctx, later); err != nil {
		t.Fatalf("failed to create later program: %v", err)
	}
	if err := repo.Create(ctx, earlier); err != nil {
		t.Fatalf("failed to create earlier program: %v", err)
	}

	programs, err := repo.ListByChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListByChannel returned error: %v", err)
	}
	if len(programs) != 2 || !programs[0].StartTime.Equal(base) {
		t.Fatalf("expected programs ordered by start_time, got %+v", programs)
	}
}
