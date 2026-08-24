package playback

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func generateSessionTestVideo(t *testing.T, dir, name string, durationSec int) string {
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

func TestSessionManager_StartSession_ProducesPlaylistAndSegments(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 12)

	m := NewSessionManager(t.TempDir(), time.Minute)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	defer m.Close()

	if _, err := os.Stat(filepath.Join(sess.Dir, "playlist.m3u8")); err != nil {
		t.Errorf("expected playlist.m3u8 to exist: %v", err)
	}

	// StartSession only waits for the playlist itself to appear; poll for
	// the segment count to reach at least 2, proving the playlist is
	// genuinely segmenting/growing over time (with 2s-forced keyframes and
	// a 12s source, a single giant segment would indicate -hls_time/forced
	// keyframes regressed), not just that one segment eventually shows up.
	deadline := time.Now().Add(10 * time.Second)
	var segmentCount int
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(sess.Dir)
		segmentCount = 0
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".ts" {
				segmentCount++
			}
		}
		if segmentCount >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if segmentCount < 2 {
		t.Fatalf("expected at least 2 segments after 10s, got %d", segmentCount)
	}

	got, ok := m.Get(sess.ID)
	if !ok || got != sess {
		t.Error("expected Get to return the started session")
	}
}

func TestSessionManager_StartSession_FailsOnMissingInput(t *testing.T) {
	m := NewSessionManager(t.TempDir(), time.Minute)
	_, err := m.StartSession("/no/such/file.mp4", 0)
	if err == nil {
		t.Fatal("expected an error for a nonexistent input file")
	}
}

func TestSessionManager_Sweep_RemovesIdleSessions(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 6)

	m := NewSessionManager(t.TempDir(), 200*time.Millisecond)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	// StartSession's own (variable-latency) ffmpeg startup shouldn't count
	// against the idle window under test — touch explicitly right before
	// exercising it, so this test only measures Sweep's own behavior.
	m.Touch(sess.ID)

	// Not idle yet: sweeping "now" (well within the timeout) must not
	// remove it.
	m.Sweep(time.Now())
	if _, ok := m.Get(sess.ID); !ok {
		t.Fatal("expected the fresh session to survive an immediate sweep")
	}

	// Sweeping as if idleTimeout had already elapsed since the touch must
	// remove it.
	m.Sweep(time.Now().Add(300 * time.Millisecond))
	if _, ok := m.Get(sess.ID); ok {
		t.Error("expected the idle session to be removed")
	}
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Error("expected the session directory to be removed")
	}
}

func TestSessionManager_Touch_ResetsIdleClock(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 6)

	m := NewSessionManager(t.TempDir(), 200*time.Millisecond)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	defer m.Close()

	future := time.Now().Add(150 * time.Millisecond)
	m.Touch(sess.ID)
	// A sweep at "future" is only ~150ms after the touch, under the 200ms
	// timeout: the session must survive.
	m.Sweep(future)
	if _, ok := m.Get(sess.ID); !ok {
		t.Error("expected a touched session to survive a sweep within the idle window")
	}
}

func TestCleanOrphanedSessions(t *testing.T) {
	base := t.TempDir()
	leftover := filepath.Join(base, "orphaned-session-id")
	if err := os.MkdirAll(leftover, 0755); err != nil {
		t.Fatalf("setting up leftover dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leftover, "playlist.m3u8"), []byte("stale"), 0644); err != nil {
		t.Fatalf("writing leftover file: %v", err)
	}

	if err := CleanOrphanedSessions(base); err != nil {
		t.Fatalf("CleanOrphanedSessions returned error: %v", err)
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("reading base dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected the leftover session directory to be removed, found %v", entries)
	}
}

func TestCleanOrphanedSessions_MissingBaseDirIsNotAnError(t *testing.T) {
	base := filepath.Join(t.TempDir(), "does-not-exist-yet")
	if err := CleanOrphanedSessions(base); err != nil {
		t.Errorf("expected no error for a base directory that doesn't exist yet, got %v", err)
	}
}
