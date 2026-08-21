package mediastore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// TestScanner_UnreadableSubdirectoryDoesNotAbortScan covers the binding
// constraint that a single unavailable item never aborts a whole scan. An
// unreadable subdirectory (a routine occurrence on NAS/SMB mounts) makes
// WalkDir hand the callback a permission error; the rest of the tree must
// still be scanned.
func TestScanner_UnreadableSubdirectoryDoesNotAbortScan(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()

	// "aaa-locked" sorts before "zzz-good.mp4", so WalkDir hits the failing
	// entry first — if the error aborted the walk, the good file would never
	// be reached.
	locked := filepath.Join(mediaDir, "aaa-locked")
	if err := os.Mkdir(locked, 0755); err != nil {
		t.Fatalf("failed to create locked dir: %v", err)
	}
	generateTestVideo(t, locked, "hidden.mp4", 1)
	generateTestVideo(t, mediaDir, "zzz-good.mp4", 2)

	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatalf("failed to chmod locked dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0755) })

	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("directory is still readable (running as root?); cannot exercise the permission-error path")
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
		t.Fatalf("ScanSource returned error despite an unreadable subdirectory: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 || items[0].RelPath != "zzz-good.mp4" {
		t.Fatalf("expected the readable video to still be scanned, got %+v", items)
	}
	if items[0].Invalid {
		t.Error("expected the readable video to be valid")
	}
}

// TestScanner_RescanOfUnchangedFilePreservesMetadata locks in the
// "skip re-probe if unchanged" branch: rescanning a file whose size and
// modtime are untouched must leave the row intact and correct, and must not
// re-upsert it (observed via updated_at staying put).
func TestScanner_RescanOfUnchangedFilePreservesMetadata(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	generateTestVideo(t, mediaDir, "stable.mp4", 2)

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

	first, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 media item after first scan, got %d", len(first))
	}

	// Upsert stamps updated_at from time.Now(), so sleeping past the clock's
	// resolution makes a re-upsert observable.
	time.Sleep(10 * time.Millisecond)

	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("second ScanSource returned error: %v", err)
	}

	second, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("expected 1 media item after rescan, got %d", len(second))
	}
	if !second[0].UpdatedAt.Equal(first[0].UpdatedAt) {
		t.Errorf("expected an unchanged file to be skipped (updated_at %v), but it was re-upserted (updated_at %v)",
			first[0].UpdatedAt, second[0].UpdatedAt)
	}
	if second[0].ID != first[0].ID {
		t.Errorf("expected the same row id across rescans, got %d then %d", first[0].ID, second[0].ID)
	}
	if second[0].Invalid || second[0].DurationSec < 1.5 || second[0].VideoCodec != "h264" {
		t.Errorf("expected metadata preserved across rescan, got %+v", second[0])
	}
}

// TestScanner_RescanRepairsPreviouslyInvalidFile locks in the `!existingItem.Invalid`
// half of the skip condition: an item marked Invalid is always re-probed, so
// replacing a broken file with a real video heals it on the next scan.
func TestScanner_RescanRepairsPreviouslyInvalidFile(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()

	relPath := "repaired.mp4"
	fullPath := filepath.Join(mediaDir, relPath)
	if err := os.WriteFile(fullPath, []byte("not a real video"), 0644); err != nil {
		t.Fatalf("failed to write broken file: %v", err)
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
		t.Fatalf("first ScanSource returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 || !items[0].Invalid {
		t.Fatalf("expected the broken file to be recorded as invalid, got %+v", items)
	}

	// Replace the broken file with a real video at the same relative path.
	if err := os.Remove(fullPath); err != nil {
		t.Fatalf("failed to remove broken file: %v", err)
	}
	generateTestVideo(t, mediaDir, relPath, 2)

	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("second ScanSource returned error: %v", err)
	}

	items, err = itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 media item after repair, got %d", len(items))
	}
	repaired := items[0]
	if repaired.Invalid {
		t.Error("expected the repaired file to no longer be marked invalid")
	}
	if repaired.DurationSec < 1.5 || repaired.DurationSec > 2.5 {
		t.Errorf("expected duration ~2s after repair, got %f", repaired.DurationSec)
	}
	if repaired.VideoCodec != "h264" {
		t.Errorf("expected video codec h264 after repair, got %q", repaired.VideoCodec)
	}
	if repaired.AudioCodec != "aac" {
		t.Errorf("expected audio codec aac after repair, got %q", repaired.AudioCodec)
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
