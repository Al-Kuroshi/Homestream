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
	srv := newTestServer(t)
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

func TestProgramsAPI_AddListUpdateDelete(t *testing.T) {
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

	srv := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST program failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.Program
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	listResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/programs")
	if err != nil {
		t.Fatalf("GET programs failed: %v", err)
	}
	var programs []model.Program
	json.NewDecoder(listResp.Body).Decode(&programs)
	listResp.Body.Close()
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}

	newStart := start.Add(time.Hour)
	updateBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": newStart})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/programs/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT program failed: %v", err)
	}
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updResp.StatusCode)
	}
	updResp.Body.Close()

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/programs/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE program failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}
