package api_test

import (
	"bytes"
	"context"
	"database/sql"
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

func seedChannelAndMediaItem(t *testing.T, conn *sql.DB) (*model.Channel, *model.MediaItem) {
	t.Helper()
	ctx := context.Background()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("creating media item: %v", err)
	}
	channel := &model.Channel{Name: "Movies"}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}
	return channel, item
}

func TestHandleAddSlot_Recurring(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": true, "day_of_week": 1, "position": 1000,
	})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d", resp.StatusCode)
	}
}

func TestHandleAddSlot_RejectsWhenValidationFails(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": false,
		// 30m before midnight; a 1h-long item spills over.
		"start_time": time.Date(2026, 8, 31, 23, 30, 0, 0, time.UTC).Format(time.RFC3339),
	})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400 (spills past midnight)", resp.StatusCode)
	}
}

func TestHandleResolvedSlots(t *testing.T) {
	conn := db.OpenTest(t)
	channel, item := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	addBody, _ := json.Marshal(map[string]any{
		"kind": "media", "media_item_id": item.ID, "recurring": true, "day_of_week": 1, "position": 1000,
	})
	addResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(addBody))
	if err != nil {
		t.Fatalf("setup POST failed: %v", err)
	}
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: got status %d", addResp.StatusCode)
	}

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	url := ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/slots/resolved?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d", resp.StatusCode)
	}
	var resolved []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("got %d resolved slots, want 1", len(resolved))
	}
	if resolved[0]["kind"] != "media" {
		t.Errorf("got kind %v, want media", resolved[0]["kind"])
	}
}

// A gap slot resolves with media_item_id 0, so kind/gap_label are the only
// way a client can tell "deliberate scheduled break" from "broken media
// reference" — without them the Guide renders a gap as literally "Media #0".
func TestHandleResolvedSlots_CarriesGapKindAndLabel(t *testing.T) {
	conn := db.OpenTest(t)
	channel, _ := seedChannelAndMediaItem(t, conn)
	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	addBody, _ := json.Marshal(map[string]any{
		"kind": "gap", "gap_duration_sec": 300, "gap_label": "Ad Break",
		"recurring": true, "day_of_week": 1, "position": 1000,
	})
	addResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(addBody))
	if err != nil {
		t.Fatalf("setup POST failed: %v", err)
	}
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: got status %d", addResp.StatusCode)
	}

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	url := ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/slots/resolved?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	var resolved []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("got %d resolved slots, want 1", len(resolved))
	}
	if resolved[0]["kind"] != "gap" {
		t.Errorf("got kind %v, want gap", resolved[0]["kind"])
	}
	if resolved[0]["gap_label"] != "Ad Break" {
		t.Errorf("got gap_label %v, want \"Ad Break\"", resolved[0]["gap_label"])
	}
}
