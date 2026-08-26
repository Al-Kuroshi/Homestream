package api_test

import (
	"bytes"
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

func TestChannelsAPI_CreateGetUpdateDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"name": "Movies", "description": "Movie channel"})
	resp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/channels failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.Channel
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("expected created channel to have an ID")
	}

	getResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(created.ID, 10))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	updateBody, _ := json.Marshal(map[string]any{"name": "Movies HD", "description": "Movie channel", "enabled": true, "position": 1})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/channels/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updResp.StatusCode)
	}
	updResp.Body.Close()

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/channels/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestChannelsAPI_UpdateNonexistent404(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	updateBody, _ := json.Marshal(map[string]any{"name": "Movies HD", "description": "Movie channel", "enabled": true, "position": 1})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/channels/999999", bytes.NewReader(updateBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestChannelsAPI_MissingChannelIsConsistently404 pins one behavior for
// "this channel doesn't exist" across all three endpoints on the resource,
// rather than 404 / 200-with-null / 200-with-empty-list.
func TestChannelsAPI_MissingChannelIsConsistently404(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	paths := []string{
		"/api/channels/999999",
		"/api/channels/999999/now",
		"/api/channels/999999/slots",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s failed: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404 for a nonexistent channel, got %d", resp.StatusCode)
			}
		})
	}
}

// TestSlotsAPI_UpdateRejectsEmptyBody guards the PUT validation: without
// it a partial body silently wrote media_item_id = 0, orphaning the slot.
func TestSlotsAPI_UpdateRejectsEmptyBody(t *testing.T) {
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
	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	mediaItemID := item.ID
	slot := &model.Slot{ChannelID: channel.ID, Kind: model.SlotKindMedia, MediaItemID: &mediaItemID, Recurring: false, StartTime: &start}
	if err := slotRepo.Create(ctx, slot); err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}

	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	bodies := map[string]string{
		"empty object":       `{}`,
		"missing start_time": `{"kind": "media", "media_item_id": ` + strconv.FormatInt(item.ID, 10) + `, "recurring": false}`,
		"missing media item": `{"kind": "media", "recurring": false, "start_time": "2026-01-01T19:00:00Z"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPut,
				ts.URL+"/api/slots/"+strconv.FormatInt(slot.ID, 10), bytes.NewReader([]byte(body)))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("PUT slot failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", name, resp.StatusCode)
			}
		})
	}

	// The stored slot must be untouched by the rejected requests.
	stored, err := slotRepo.Get(ctx, slot.ID)
	if err != nil {
		t.Fatalf("failed to re-read slot: %v", err)
	}
	if stored.MediaItemID == nil || *stored.MediaItemID != item.ID || stored.StartTime == nil || !stored.StartTime.Equal(start) {
		t.Errorf("expected the slot to be unchanged, got %+v", stored)
	}
}

func TestSlotsAPI_AddListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

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

	srv, _ := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{"kind": "media", "media_item_id": item.ID, "recurring": false, "start_time": start})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/slots", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST slot failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.Slot
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	listResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/slots")
	if err != nil {
		t.Fatalf("GET slots failed: %v", err)
	}
	var slots []model.Slot
	json.NewDecoder(listResp.Body).Decode(&slots)
	listResp.Body.Close()
	if len(slots) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(slots))
	}

	newStart := start.Add(time.Hour)
	updateBody, _ := json.Marshal(map[string]any{"kind": "media", "media_item_id": item.ID, "recurring": false, "start_time": newStart})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/slots/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT slot failed: %v", err)
	}
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updResp.StatusCode)
	}
	updResp.Body.Close()

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/slots/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE slot failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}
