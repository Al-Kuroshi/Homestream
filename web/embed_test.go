package web

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

// skipUnlessBuilt skips the calling test when web/dist doesn't contain a
// real build (i.e. `npm run build` hasn't been run yet, so distFS only
// holds the tracked .gitkeep/.gitignore placeholders). Without this guard
// these tests fail on a fresh clone instead of skipping, and the
// fallback-routing test even passes vacuously (both responses are the same
// placeholder directory listing).
func skipUnlessBuilt(t *testing.T) {
	t.Helper()
	if _, err := fs.Stat(distFS, "dist/index.html"); err != nil {
		t.Skip("web/dist not built (run `npm run build` in web/); skipping embedded-SPA test")
	}
}

func TestHandler_ServesIndexAtRoot(t *testing.T) {
	skipUnlessBuilt(t)

	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `<div id="root">`) {
		t.Errorf("expected index.html to contain the SPA's root div, got: %s", w.Body.String())
	}
}

func TestHandler_FallsBackToIndexForClientRoutes(t *testing.T) {
	skipUnlessBuilt(t)

	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	rootW := httptest.NewRecorder()
	handler.ServeHTTP(rootW, httptest.NewRequest("GET", "/", nil))

	routeW := httptest.NewRecorder()
	handler.ServeHTTP(routeW, httptest.NewRequest("GET", "/channels/5", nil))

	if routeW.Code != 200 {
		t.Fatalf("expected status 200 for a client-side route, got %d", routeW.Code)
	}
	if routeW.Body.String() != rootW.Body.String() {
		t.Errorf("expected /channels/5 to fall back to the same body as /, but it differed")
	}
}
