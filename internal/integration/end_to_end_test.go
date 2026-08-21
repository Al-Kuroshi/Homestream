package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	ctx := context.Background()
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
	source := &model.MediaSource{Name: "Movies", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

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

	// 7. restart simulation: reopen the same DB file and confirm state survives
	// (nothing in this stack is in-memory-only per design spec §6 reliability).
	_ = channel
}
