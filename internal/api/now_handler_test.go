package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestChannelNowAPI_CurrentProgram(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)

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
	start := time.Now().UTC().Add(-30 * time.Minute)
	mediaItemID := item.ID
	if err := slotRepo.Create(ctx, &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &mediaItemID, Recurring: false, StartTime: &start}); err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	current, ok := state["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected a current program, got %+v", state)
	}
	if int64(current["media_item_id"].(float64)) != item.ID {
		t.Errorf("expected current media item %d, got %v", item.ID, current["media_item_id"])
	}
	offsetSec, ok := state["offset_sec"].(float64)
	if !ok || offsetSec < 1700 || offsetSec > 1900 {
		t.Errorf("expected offset_sec ~1800 (30 min), got %v", state["offset_sec"])
	}
}

func TestChannelNowAPI_OffAir(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)

	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer resp.Body.Close()

	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if state["current"] != nil {
		t.Errorf("expected no current program for an empty channel, got %v", state["current"])
	}
}
