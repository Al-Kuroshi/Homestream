package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"personaltv/internal/api"
	"personaltv/internal/channels"
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
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
}

func newTestServerWithConn(t *testing.T, conn *sql.DB) *api.Server {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
}

func TestSourcesAPI_CreateListDelete(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	mediaDir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"name": "Movies", "path": mediaDir})
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

// TestSourcesAPI_CreateRejectsInvalidPaths covers the minimal validation on
// the media source path: with no authentication in this MVP, an arbitrary
// path would let any caller point the scanner anywhere on the filesystem.
func TestSourcesAPI_CreateRejectsInvalidPaths(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "a-file.txt")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	cases := []struct {
		name string
		path string
	}{
		{"relative path", "media/movies"},
		{"file instead of directory", filePath},
		{"nonexistent directory", filepath.Join(dir, "does-not-exist")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"name": "Movies", "path": tc.path})
			resp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST /api/sources failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s %q, got %d", tc.name, tc.path, resp.StatusCode)
			}
		})
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
	if len(sources) != 0 {
		t.Fatalf("expected no sources to be persisted, got %+v", sources)
	}
}

// TestMediaAPI_ListEmpty asserts on the raw body, not just the decoded
// length: `null` and `[]` both decode to a zero-length slice, so only the
// raw check catches a List method handing back a nil slice.
func TestMediaAPI_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	for _, path := range []string{"/api/media", "/api/sources", "/api/channels"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s failed: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			if got := strings.TrimSpace(string(raw)); got != "[]" {
				t.Fatalf("expected an empty JSON array %q, got %q", "[]", got)
			}
		})
	}
}

// TestProgramsAPI_ListEmptyForExistingChannel is the same raw-body check for
// the per-channel programs list.
func TestProgramsAPI_ListEmptyForExistingChannel(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)
	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	srv := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/programs")
	if err != nil {
		t.Fatalf("GET programs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Fatalf("expected an empty JSON array %q, got %q", "[]", got)
	}
}

// TestSourcesAPI_ResponsesUseSnakeCase pins the public response contract:
// responses that marshal a model struct must use the same snake_case keys
// as the request bodies, not Go's default capitalized field names.
func TestSourcesAPI_ResponsesUseSnakeCase(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"name": "Movies", "path": t.TempDir()})
	resp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/sources failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	for _, key := range []string{"id", "name", "path", "created_at"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected snake_case key %q in response, got keys %v", key, decoded)
		}
	}
	for _, key := range []string{"ID", "CreatedAt"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("unexpected PascalCase key %q in response", key)
		}
	}
}
