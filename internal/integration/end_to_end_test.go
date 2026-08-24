package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/api"
	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/model"
	"personaltv/internal/playback"
	"personaltv/internal/repository/sqlite"
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

// TestFullUserJourney drives the same steps a real user takes per the PRD's
// MVP user journey (docs/prd/HomeStreamer.md §15): configure a media
// source, scan it, create a channel, schedule media on it, and confirm the
// API reports what's currently playing. This is Plan 1's Definition of Done.
func TestFullUserJourney(t *testing.T) {
	mediaDir := t.TempDir()
	generateTestVideo(t, mediaDir, "movie-a.mp4", 10)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	// 1. configure a media source
	srcBody, _ := json.Marshal(map[string]any{"name": "Movies", "path": mediaDir})
	srcResp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(srcBody))
	if err != nil {
		t.Fatalf("create source request failed: %v", err)
	}
	if srcResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating source, got %d", srcResp.StatusCode)
	}
	var source model.MediaSource
	json.NewDecoder(srcResp.Body).Decode(&source)
	srcResp.Body.Close()

	// 2. scan it
	scanResp, err := http.Post(ts.URL+"/api/sources/"+strconv.FormatInt(source.ID, 10)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatalf("scan request failed: %v", err)
	}
	if scanResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from scan, got %d", scanResp.StatusCode)
	}

	// 3. confirm the media library was populated
	mediaResp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	var items []model.MediaItem
	json.NewDecoder(mediaResp.Body).Decode(&items)
	mediaResp.Body.Close()
	if len(items) != 1 {
		t.Fatalf("expected 1 media item after scan, got %d", len(items))
	}
	item := items[0]

	// 4. create a channel
	chBody, _ := json.Marshal(map[string]any{"name": "Movies"})
	chResp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(chBody))
	if err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	var channel model.Channel
	json.NewDecoder(chResp.Body).Decode(&channel)
	chResp.Body.Close()

	// 5. schedule the media item to start a few seconds ago, so it's playing
	// now. The offset must stay smaller than the video's duration (10s,
	// generated above) — the scheduler (Task 7) deliberately reports a
	// channel as off-air once now passes a program's start+duration, with
	// no auto-repeat/looping (see scheduler.Evaluate and its
	// TestEvaluate_NothingScheduledAfterNow case, and PRD §93 which lists
	// recurring/looping programs as explicitly out of MVP scope). A 3s
	// offset leaves margin against slower CI while staying well under the
	// 10s program length.
	start := time.Now().UTC().Add(-3 * time.Second)
	progBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(progBody))
	if err != nil {
		t.Fatalf("add program failed: %v", err)
	}
	if progResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 adding program, got %d", progResp.StatusCode)
	}
	progResp.Body.Close()

	// 6. ask what's playing now
	nowResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer nowResp.Body.Close()
	var state map[string]any
	json.NewDecoder(nowResp.Body).Decode(&state)
	current, ok := state["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected a current program, got %+v", state)
	}
	if int64(current["media_item_id"].(float64)) != item.ID {
		t.Errorf("expected current media item %d, got %v", item.ID, current["media_item_id"])
	}
}

// TestFullUserJourney_Playback extends the same real-user-journey pattern
// as TestFullUserJourney to prove tuning in and streaming actually works
// end-to-end through the full stack: a media source, a scheduled channel,
// POST .../watch, and a real byte range fetched from the resulting stream
// URL. This is Plan 3's Definition of Done, the playback equivalent of
// TestFullUserJourney.
func TestFullUserJourney_Playback(t *testing.T) {
	mediaDir := t.TempDir()
	videoPath := generateTestVideo(t, mediaDir, "movie-a.mp4", 10)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)

	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	playbackSvc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	server.SetPlaybackService(playbackSvc)
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	// 1. configure and scan a media source
	srcBody, _ := json.Marshal(map[string]any{"name": "Movies", "path": mediaDir})
	srcResp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(srcBody))
	if err != nil {
		t.Fatalf("create source request failed: %v", err)
	}
	var source model.MediaSource
	json.NewDecoder(srcResp.Body).Decode(&source)
	srcResp.Body.Close()

	scanResp, err := http.Post(ts.URL+"/api/sources/"+strconv.FormatInt(source.ID, 10)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatalf("scan request failed: %v", err)
	}
	scanResp.Body.Close()

	mediaResp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	var items []model.MediaItem
	json.NewDecoder(mediaResp.Body).Decode(&items)
	mediaResp.Body.Close()
	if len(items) != 1 {
		t.Fatalf("expected 1 media item after scan, got %d", len(items))
	}
	item := items[0]

	// 2. create a channel and schedule the item on it, starting a few
	// seconds ago so it's playing now (same timing rationale as
	// TestFullUserJourney above: keep the offset well under the 10s
	// generated video's duration).
	chBody, _ := json.Marshal(map[string]any{"name": "Movies"})
	chResp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(chBody))
	if err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	var channel model.Channel
	json.NewDecoder(chResp.Body).Decode(&channel)
	chResp.Body.Close()

	start := time.Now().UTC().Add(-3 * time.Second)
	progBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(progBody))
	if err != nil {
		t.Fatalf("add program failed: %v", err)
	}
	progResp.Body.Close()

	// 3. tune in
	watchResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/watch", "application/json", nil)
	if err != nil {
		t.Fatalf("watch request failed: %v", err)
	}
	defer watchResp.Body.Close()
	var watch struct {
		Status      string  `json:"status"`
		Mode        string  `json:"mode"`
		MediaItemID int64   `json:"media_item_id"`
		OffsetSec   float64 `json:"offset_sec"`
	}
	json.NewDecoder(watchResp.Body).Decode(&watch)

	if watch.Status != "playing" || watch.Mode != "direct" {
		t.Fatalf("expected status=playing mode=direct for a plain h264/aac/mp4 file, got %+v", watch)
	}
	if watch.MediaItemID != item.ID {
		t.Errorf("expected media item %d, got %d", item.ID, watch.MediaItemID)
	}

	// 4. actually fetch the stream and confirm it's the real file
	streamResp, err := http.Get(ts.URL + "/api/media/" + strconv.FormatInt(watch.MediaItemID, 10) + "/stream")
	if err != nil {
		t.Fatalf("GET stream failed: %v", err)
	}
	defer streamResp.Body.Close()
	streamBody, _ := io.ReadAll(streamResp.Body)
	onDisk, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatalf("reading generated video: %v", err)
	}
	if len(streamBody) != len(onDisk) {
		t.Fatalf("expected the streamed body to match the file on disk (%d bytes), got %d bytes", len(onDisk), len(streamBody))
	}
}
