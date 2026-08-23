package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ServesIndexAtRoot(t *testing.T) {
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
