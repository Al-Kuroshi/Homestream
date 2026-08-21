package sqlite_test

import (
	"context"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestMediaItemRepository_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	item := &model.MediaItem{
		SourceID:    source.ID,
		RelPath:     "action/movie-a.mp4",
		Title:       "movie-a",
		DurationSec: 7200,
		VideoCodec:  "h264",
		AudioCodec:  "aac",
		Container:   "mov,mp4,m4a,3gp,3g2,mj2",
		SizeBytes:   123456,
		ModTime:     time.Now().UTC().Truncate(time.Second),
	}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("expected Upsert to set an ID")
	}

	fetched, err := itemRepo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.Title != "movie-a" || fetched.DurationSec != 7200 {
		t.Errorf("unexpected item: %+v", fetched)
	}

	// Upsert again with the same source_id/rel_path should update, not duplicate.
	item.Title = "movie-a-renamed"
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after re-upsert, got %d", len(items))
	}
	if items[0].Title != "movie-a-renamed" {
		t.Errorf("expected updated title, got %q", items[0].Title)
	}
}

func TestMediaItemRepository_DeleteMissing(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	keep := &model.MediaItem{SourceID: source.ID, RelPath: "keep.mp4", Title: "keep", ModTime: time.Now().UTC()}
	remove := &model.MediaItem{SourceID: source.ID, RelPath: "remove.mp4", Title: "remove", ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, keep); err != nil {
		t.Fatalf("failed to upsert keep item: %v", err)
	}
	if err := itemRepo.Upsert(ctx, remove); err != nil {
		t.Fatalf("failed to upsert remove item: %v", err)
	}

	if err := itemRepo.DeleteMissing(ctx, source.ID, []string{"keep.mp4"}); err != nil {
		t.Fatalf("DeleteMissing returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 || items[0].RelPath != "keep.mp4" {
		t.Fatalf("expected only keep.mp4 to remain, got %+v", items)
	}
}
