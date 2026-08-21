package mediastore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestScanner_ScanSource(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()

	generateTestVideo(t, mediaDir, "test.mp4", 2)

	if err := os.WriteFile(filepath.Join(mediaDir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("failed to write notes.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "broken.mp4"), []byte("not a real video"), 0644); err != nil {
		t.Fatalf("failed to write broken.mp4: %v", err)
	}

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Test Source", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	scanner := NewScanner(sourceRepo, itemRepo)
	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("ScanSource returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 media items (valid + broken), got %d", len(items))
	}

	var valid, broken *model.MediaItem
	for _, item := range items {
		switch item.RelPath {
		case "test.mp4":
			valid = item
		case "broken.mp4":
			broken = item
		}
	}

	if valid == nil {
		t.Fatal("expected test.mp4 to be scanned")
	}
	if valid.Invalid {
		t.Error("expected test.mp4 to be valid")
	}
	if valid.DurationSec < 1.5 {
		t.Errorf("expected test.mp4 duration ~2s, got %f", valid.DurationSec)
	}
	if valid.Title != "test" {
		t.Errorf("expected title derived from filename 'test', got %q", valid.Title)
	}

	if broken == nil {
		t.Fatal("expected broken.mp4 to be scanned and marked invalid")
	}
	if !broken.Invalid {
		t.Error("expected broken.mp4 to be marked invalid")
	}
}

func TestScanner_ScanSourceRemovesDeletedFiles(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	videoPath := generateTestVideo(t, mediaDir, "gone.mp4", 2)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Test Source", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	scanner := NewScanner(sourceRepo, itemRepo)
	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("first ScanSource returned error: %v", err)
	}

	if err := os.Remove(videoPath); err != nil {
		t.Fatalf("failed to remove test video: %v", err)
	}

	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("second ScanSource returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected deleted file to be pruned, got %+v", items)
	}
}
