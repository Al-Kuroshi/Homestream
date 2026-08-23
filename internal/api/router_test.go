package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes_WithoutStaticHandler_UnmatchedPath404s(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/some/unmatched/path")
	if err != nil {
		t.Fatalf("GET returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for an unmatched path with no static handler set, got %d", resp.StatusCode)
	}
}

func TestRoutes_WithStaticHandler_FallsBackForUnmatchedPaths(t *testing.T) {
	srv := newTestServer(t)
	srv.SetStaticHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("static: " + r.URL.Path))
	}))
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/guide")
	if err != nil {
		t.Fatalf("GET returned error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from the static handler, got %d", resp.StatusCode)
	}
	if string(body) != "static: /guide" {
		t.Errorf("expected the static handler to receive the unmatched path, got %q", string(body))
	}

	// /api/sources must still route to the real API handler, not the static fallback.
	apiResp, err := http.Get(ts.URL + "/api/sources")
	if err != nil {
		t.Fatalf("GET /api/sources returned error: %v", err)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusOK {
		t.Errorf("expected /api/sources to still be routed to the API, got status %d", apiResp.StatusCode)
	}
}

func TestRoutes_WithStaticHandler_UnmatchedApiPath404s(t *testing.T) {
	srv := newTestServer(t)
	srv.SetStaticHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("static: " + r.URL.Path))
	}))
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// An unmatched /api/* path must 404, not silently fall through to the
	// SPA static handler (which would return 200 text/html and break
	// web/src/api/http.ts's error handling).
	resp, err := http.Get(ts.URL + "/api/nonexistent-endpoint")
	if err != nil {
		t.Fatalf("GET /api/nonexistent-endpoint returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for an unmatched /api/* path, got %d", resp.StatusCode)
	}

	// A real, registered endpoint must still work — proving the more-specific
	// exact routes still win over the new /api/ catch-all.
	sourcesResp, err := http.Get(ts.URL + "/api/sources")
	if err != nil {
		t.Fatalf("GET /api/sources returned error: %v", err)
	}
	defer sourcesResp.Body.Close()
	if sourcesResp.StatusCode != http.StatusOK {
		t.Errorf("expected /api/sources to still return 200, got %d", sourcesResp.StatusCode)
	}
}
