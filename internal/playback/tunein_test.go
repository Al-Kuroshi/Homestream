package playback_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/playback"
	"personaltv/internal/repository/sqlite"
)

func generateTuneInTestVideo(t *testing.T, dir, name string, durationSec int) string {
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

type tuneInFixture struct {
	ctx      context.Context
	svc      *playback.Service
	channels *channels.Service
	items    *sqlite.MediaItemRepository
	sourceID int64
	mediaDir string
}

func newTuneInFixture(t *testing.T) *tuneInFixture {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)
	channelSvc := channels.NewService(channelRepo, slotRepo, itemRepo)
	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	t.Cleanup(func() { sessions.Close() })
	svc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	ctx := context.Background()
	mediaDir := t.TempDir()
	source := &model.MediaSource{Name: "Movies", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}

	return &tuneInFixture{
		ctx: ctx, svc: svc, channels: channelSvc, items: itemRepo,
		sourceID: source.ID, mediaDir: mediaDir,
	}
}

// addItem inserts a media item row. If relPath names a file that doesn't
// actually exist under the fixture's media directory, the resulting item
// is "scheduled but not playable" — exactly the missing-file case TuneIn
// must exclude.
func (f *tuneInFixture) addItem(t *testing.T, relPath string, durationSec float64, videoCodec, audioCodec, container string) *model.MediaItem {
	t.Helper()
	item := &model.MediaItem{
		SourceID: f.sourceID, RelPath: relPath, Title: relPath,
		DurationSec: durationSec, VideoCodec: videoCodec, AudioCodec: audioCodec,
		Container: container, ModTime: time.Now().UTC(),
	}
	if err := f.items.Upsert(f.ctx, item); err != nil {
		t.Fatalf("upserting item %s: %v", relPath, err)
	}
	return item
}

func (f *tuneInFixture) addChannel(t *testing.T) *model.Channel {
	t.Helper()
	channel := &model.Channel{Name: "Test Channel", Enabled: true}
	if err := f.channels.CreateChannel(f.ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}
	return channel
}

func (f *tuneInFixture) addProgram(t *testing.T, channelID, mediaItemID int64, startTime time.Time) {
	t.Helper()
	slot := &model.Slot{
		ChannelID: channelID, Kind: model.SlotKindMedia,
		MediaItemID: &mediaItemID, Recurring: false, StartTime: &startTime,
	}
	if err := f.channels.AddSlot(f.ctx, slot); err != nil {
		t.Fatalf("adding program: %v", err)
	}
}

func TestTuneIn_OffAirWhenNothingScheduled(t *testing.T) {
	f := newTuneInFixture(t)
	channel := f.addChannel(t)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "off_air" {
		t.Errorf("expected status off_air, got %+v", result)
	}
}

func TestTuneIn_DirectPlayForCompatibleFile(t *testing.T) {
	f := newTuneInFixture(t)
	generateTuneInTestVideo(t, f.mediaDir, "movie.mp4", 10)
	item := f.addItem(t, "movie.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "playing" || result.Mode != "direct" {
		t.Fatalf("expected playing/direct, got %+v", result)
	}
	if result.MediaItemID != item.ID {
		t.Errorf("expected media item %d, got %d", item.ID, result.MediaItemID)
	}
	if result.OffsetSec < 2.5 || result.OffsetSec > 4 {
		t.Errorf("expected offset ~3s, got %v", result.OffsetSec)
	}
}

func TestTuneIn_HLSForIncompatibleFile(t *testing.T) {
	f := newTuneInFixture(t)
	generateTuneInTestVideo(t, f.mediaDir, "movie.mp4", 10)
	// A real h264/aac file, but its stored codec info is deliberately set
	// to something the compatibility matrix (Task 1) rejects — this
	// exercises the hls path without needing an actual hevc/vp9 encoder
	// to be available wherever this test runs.
	item := f.addItem(t, "movie.mp4", 10, "hevc", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "playing" || result.Mode != "hls" {
		t.Fatalf("expected playing/hls, got %+v", result)
	}
	if result.SessionID == "" {
		t.Error("expected a non-empty session id")
	}
	if _, ok := f.svc.GetSession(result.SessionID); !ok {
		t.Error("expected the returned session id to be a real, tracked session")
	}
}

func TestTuneIn_UnavailableWhenScheduledFileIsMissing(t *testing.T) {
	f := newTuneInFixture(t)
	// No file is ever generated at this RelPath — it's scheduled but
	// missing on disk.
	item := f.addItem(t, "missing.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "unavailable" {
		t.Errorf("expected status unavailable, got %+v", result)
	}
}

func TestTuneIn_DoesNotJumpAheadToAFutureProgramWhenCurrentFileIsMissing(t *testing.T) {
	f := newTuneInFixture(t)
	missingItem := f.addItem(t, "missing.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	generateTuneInTestVideo(t, f.mediaDir, "future.mp4", 10)
	futureItem := f.addItem(t, "future.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)

	now := time.Now().UTC()
	// missingItem is airing right now (its file just happens to be
	// missing); futureItem is scheduled to start later today.
	f.addProgram(t, channel.ID, missingItem.ID, now.Add(-3*time.Second))
	f.addProgram(t, channel.ID, futureItem.ID, now.Add(time.Hour))

	result, err := f.svc.TuneIn(f.ctx, channel.ID, now)
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	// Must report unavailable right now, not jump ahead to futureItem
	// before its own scheduled start time — channel state stays a pure
	// function of (schedule, now), never advancing early.
	if result.Status != "unavailable" {
		t.Fatalf("expected status unavailable (not jumping ahead to a future program), got %+v", result)
	}
}
