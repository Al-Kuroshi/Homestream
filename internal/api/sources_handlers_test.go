package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"personaltv/internal/api"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner)
}

func TestSourcesAPI_CreateListDelete(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"name": "Movies", "path": "/media/movies"})
	resp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/sources failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.MediaSource
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("expected created source to have an ID")
	}

	listResp, err := http.Get(ts.URL + "/api/sources")
	if err != nil {
		t.Fatalf("GET /api/sources failed: %v", err)
	}
	defer listResp.Body.Close()
	var sources []model.MediaSource
	if err := json.NewDecoder(listResp.Body).Decode(&sources); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/sources/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestMediaAPI_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var items []model.MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no media items, got %d", len(items))
	}
}
