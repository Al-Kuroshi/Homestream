package mediastore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func generateTestVideo(t *testing.T, dir, name string, durationSec int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	d := strconv.Itoa(durationSec)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration="+d+":size=64x64:rate=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+d,
		"-c:v", "libx264", "-c:a", "aac", "-shortest", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test video: %v\n%s", err, out)
	}
	return path
}

func TestProbe_ValidVideo(t *testing.T) {
	dir := t.TempDir()
	path := generateTestVideo(t, dir, "test.mp4", 2)

	result, err := Probe(context.Background(), path)
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if result.DurationSec < 1.5 || result.DurationSec > 2.5 {
		t.Errorf("expected duration ~2s, got %f", result.DurationSec)
	}
	if result.VideoCodec != "h264" {
		t.Errorf("expected video codec h264, got %q", result.VideoCodec)
	}
	if result.AudioCodec != "aac" {
		t.Errorf("expected audio codec aac, got %q", result.AudioCodec)
	}
}

func TestProbe_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-video.txt")
	if err := os.WriteFile(path, []byte("not a video"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := Probe(context.Background(), path); err == nil {
		t.Fatal("expected error for invalid file, got nil")
	}
}

// TestProbe_RespectsCanceledContext confirms Probe is actually bounded by its
// context — a hung ffprobe (a stalled network mount) must not block forever.
// An already-canceled context is used so the check is instant.
func TestProbe_RespectsCanceledContext(t *testing.T) {
	dir := t.TempDir()
	path := generateTestVideo(t, dir, "test.mp4", 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Probe(ctx, path); err == nil {
		t.Fatal("expected an error when probing with a canceled context, got nil")
	}
}
