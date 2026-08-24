# Personal TV — Playback Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the playback backend — a new `internal/playback` package (direct-play/transcode compatibility decision, tune-in orchestration with missing-file exclusion, an in-memory session manager with idle-timeout cleanup, `ffmpeg`-based HLS transcoding) plus three new REST endpoints — fully working and curl-testable, with no TV/player UI.

**Architecture:** `internal/playback` sits alongside `internal/channels`/`internal/scheduler`, consuming the already-built `channels.Service` (for current-program/schedule data) and the repository interfaces (for resolving a media item to its file). A `SessionManager` owns the lifecycle of transcode sessions (spawning/tracking/killing `ffmpeg` processes, sweeping idle ones). Three new `internal/api` handlers wrap this package the same way existing handlers wrap `channels.Service` — thin JSON/HTTP glue, no business logic. The Go server's `Routes()`/`NewServer` are extended additively (a new field + an additive `SetPlaybackService` method, mirroring the existing `SetStaticHandler` pattern), so none of the 15 existing `srv.Routes()`/`api.NewServer` call sites need to change.

**Tech Stack:** Go (matches the existing module), `ffmpeg`/`ffprobe` (external subprocess, already a `PATH` requirement from Plan 1 — this plan adds real HLS transcoding to that same requirement), `github.com/google/uuid` (already an indirect dependency of this module via `modernc.org/sqlite`'s dependency graph — this plan is what first imports it directly).

**Spec:** `docs/design/2026-08-23-personal-tv-playback-design.md` (which itself extends `docs/design/2026-08-21-personal-tv-design.md` §4.4, and the PRD at `docs/prd/HomeStreamer.md`). This plan implements the spec's §2 (API contract), §3 (compatibility matrix), §4 (missing-file handling), §5 (session lifecycle), and §6 (testing strategy) in full. TV/player UI (spec §1, explicitly out of scope) is a separate future plan.

## Global Constraints

- Go module `personaltv`, Go 1.22+ (existing floor from Plan 1; this repo currently builds with Go 1.25).
- `ffmpeg`/`ffprobe` must be installed and on `PATH` for this plan's tests — they generate real short synthetic videos and run real `ffmpeg` HLS transcodes against them, not mocks (design spec §6; matches Plan 1's existing testing convention for `internal/mediastore`).
- Every persisted/timestamp value already goes through `db.FormatTime`/`db.ParseTime` (Plan 1) — irrelevant here, since this plan's session state is deliberately **in-memory only**, never persisted (spec §5: "nothing lost on restart, nothing ticking with no viewers" — the same principle the scheduler already established, extended to playback sessions).
- Business logic depends only on `repository` interfaces, never on `database/sql` directly (Plan 1's established rule, unchanged and still binding for this plan's new code).
- All JSON field names are `snake_case`, matching every existing response type in `internal/model` and `internal/api` exactly (e.g. `media_item_id`, `offset_sec`, `session_id`) — never Go's default PascalCase.
- The compatibility-decision function is a pure function of already-probed codec strings — no new `ffprobe` calls at tune-in or playback time (spec §3: codec info is cached from scan time on every `MediaItem`).
- Playback never advances to a program before its own scheduled `start_time`, even to route around a missing file at the current slot — channel state stays a pure function of `(schedule, wall-clock time, which files exist on disk)`, matching the scheduler's core principle (`internal/scheduler/scheduler.go`) that nothing is tracked as separate mutable state. A missing file at the current slot is excluded from scheduling for that evaluation, exactly like an invalid/zero-duration item already is in `channels.Service.CurrentState` — never "skipped ahead to."
- No changes to any existing HTTP route, to `api.NewServer`'s signature, or to any of the 15 pre-existing `srv.Routes()`/`api.NewServer(...)` call sites across `internal/api/*_test.go` and `internal/integration/end_to_end_test.go` — all playback wiring on `*api.Server` is additive (a new field + an additive setter method), mirroring the existing `SetStaticHandler` pattern in `internal/api/router.go`.
- No stream-copy optimization: when transcoding, both video and audio are always fully re-encoded to `h264`/`aac`, even if only one track is technically incompatible (spec §1 — an explicit MVP simplification, not a gap).
- No backward seeking to before the tune-in point (spec §1/§5) — `ffmpeg` always input-seeks forward to the computed offset and produces an appending (`event`-type) HLS playlist from there.

---

## Task 1: Compatibility Matrix

**Files:**
- Create: `internal/playback/compat.go`
- Test: `internal/playback/compat_test.go`

**Interfaces:**
- Consumes: nothing (pure function, first task).
- Produces: `playback.IsDirectPlayCompatible(videoCodec, audioCodec, container string) bool`. **Task 5's `TuneIn` is the only consumer.**

- [ ] **Step 1: Write the failing tests**

`internal/playback/compat_test.go`:

```go
package playback

import "testing"

func TestIsDirectPlayCompatible(t *testing.T) {
	tests := []struct {
		name                              string
		videoCodec, audioCodec, container string
		want                              bool
	}{
		{"h264/aac/mp4 is compatible", "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2", true},
		{"h264/mp3/mp4 is compatible", "h264", "mp3", "mov,mp4,m4a,3gp,3g2,mj2", true},
		{"h265 video is not compatible", "hevc", "aac", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"vp9 video is not compatible", "vp9", "aac", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"ac3 audio is not compatible", "h264", "ac3", "mov,mp4,m4a,3gp,3g2,mj2", false},
		{"mkv container is not compatible even with compatible codecs", "h264", "aac", "matroska,webm", false},
		{"avi container is not compatible", "h264", "aac", "avi", false},
		{"empty codec info is not compatible", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDirectPlayCompatible(tt.videoCodec, tt.audioCodec, tt.container)
			if got != tt.want {
				t.Errorf("IsDirectPlayCompatible(%q, %q, %q) = %v, want %v", tt.videoCodec, tt.audioCodec, tt.container, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/playback/...`
Expected: FAIL — build error (package `playback` doesn't exist yet).

- [ ] **Step 3: Write the implementation**

`internal/playback/compat.go`:

```go
package playback

// mp4Container is the exact ffprobe format_name string for the mp4 family
// (mov,mp4,m4a,3gp,3g2,mj2) — the only container this MVP treats as
// direct-play compatible. Even h264/aac content inside an mkv/avi/webm
// container is transcoded, since browsers generally cannot demux those
// containers via a plain <video> element regardless of what's inside them.
const mp4Container = "mov,mp4,m4a,3gp,3g2,mj2"

// IsDirectPlayCompatible reports whether a media item with the given
// (scan-time-probed) codec/container info can be served directly via HTTP
// range requests, or needs transcoding to HLS. Deliberately the narrowest
// matrix that covers "a typical h264/aac mp4 rip plays with zero CPU
// cost": a false negative here just costs some transcode CPU, a false
// positive means a broken player, so this stays conservative rather than
// maximizing direct-play coverage (design spec §3).
func IsDirectPlayCompatible(videoCodec, audioCodec, container string) bool {
	if videoCodec != "h264" {
		return false
	}
	if audioCodec != "aac" && audioCodec != "mp3" {
		return false
	}
	return container == mp4Container
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/playback/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add internal/playback/compat.go internal/playback/compat_test.go
git commit -m "feat: add direct-play compatibility matrix"
```

---

## Task 2: Media Path Resolution and the Direct-Play Endpoint

**Files:**
- Create: `internal/playback/resolve.go`
- Create: `internal/api/playback_handlers.go`
- Modify: `internal/api/router.go`
- Test: `internal/playback/resolve_test.go`
- Test: `internal/api/playback_handlers_test.go`

**Interfaces:**
- Consumes: `repository.MediaSourceRepository`/`MediaItemRepository` (Plan 1, already on `api.Server` as `s.sources`/`s.items`), `model.MediaSource`/`MediaItem` (Plan 1).
- Produces: `playback.ResolvePath(source *model.MediaSource, item *model.MediaItem) string`. **Task 3's `SessionManager.StartSession` and Task 5's `TuneIn` both consume this.** `GET /api/media/{id}/stream` — the direct-play endpoint, a leaf feature with no other in-plan consumer.

- [ ] **Step 1: Write the failing test for path resolution**

`internal/playback/resolve_test.go`:

```go
package playback

import (
	"testing"

	"personaltv/internal/model"
)

func TestResolvePath(t *testing.T) {
	source := &model.MediaSource{Path: "/media/movies"}
	item := &model.MediaItem{RelPath: "action/movie-a.mp4"}

	got := ResolvePath(source, item)
	want := "/media/movies/action/movie-a.mp4"
	if got != want {
		t.Errorf("ResolvePath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/playback/... -run TestResolvePath`
Expected: FAIL — `undefined: ResolvePath`

- [ ] **Step 3: Write the implementation**

`internal/playback/resolve.go`:

```go
package playback

import (
	"path/filepath"

	"personaltv/internal/model"
)

// ResolvePath returns the absolute filesystem path for a media item, given
// the source it belongs to. This is the inverse of the join the media
// scanner performs when it records each item's path relative to its
// source's root (internal/mediastore/scan.go).
func ResolvePath(source *model.MediaSource, item *model.MediaItem) string {
	return filepath.Join(source.Path, item.RelPath)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/playback/... -run TestResolvePath`
Expected: PASS

- [ ] **Step 5: Write the failing test for the direct-play endpoint**

`internal/api/playback_handlers_test.go`:

```go
package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMediaStream_ServesFileWithRangeSupport(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	mediaDir := t.TempDir()
	videoPath := generatePlaybackTestVideo(t, mediaDir, "movie-a.mp4", 3)

	sourceID := createTestSource(t, ts, "Movies", mediaDir)
	itemID := scanAndGetFirstItem(t, ts, sourceID)

	// Fetch the whole file first, to know its real size and content for
	// comparison.
	fullResp, err := http.Get(ts.URL + "/api/media/" + strconv.FormatInt(itemID, 10) + "/stream")
	if err != nil {
		t.Fatalf("GET stream failed: %v", err)
	}
	defer fullResp.Body.Close()
	fullBody, _ := io.ReadAll(fullResp.Body)
	if fullResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for a full request, got %d", fullResp.StatusCode)
	}

	onDisk, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatalf("reading generated video: %v", err)
	}
	if len(fullBody) != len(onDisk) {
		t.Fatalf("expected full response to match the file on disk (%d bytes), got %d bytes", len(onDisk), len(fullBody))
	}

	// Now request a byte range and confirm the server actually honors it
	// (proves http.ServeContent is wired against a real, seekable file).
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/media/"+strconv.FormatInt(itemID, 10)+"/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	rangeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range GET failed: %v", err)
	}
	defer rangeResp.Body.Close()
	rangeBody, _ := io.ReadAll(rangeResp.Body)

	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 for a range request, got %d", rangeResp.StatusCode)
	}
	if len(rangeBody) != 100 {
		t.Fatalf("expected a 100-byte range response, got %d bytes", len(rangeBody))
	}
	if string(rangeBody) != string(onDisk[:100]) {
		t.Fatalf("range response bytes did not match the corresponding bytes on disk")
	}
}

func TestMediaStream_404sForUnknownMediaItem(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/media/999/stream")
	if err != nil {
		t.Fatalf("GET stream failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown media item, got %d", resp.StatusCode)
	}
}

// generatePlaybackTestVideo generates a real short h264/aac mp4 video for
// this package's tests. Duplicated deliberately (not shared) from the
// equivalent helpers in internal/mediastore and internal/integration — Go
// does not let unexported test helpers be shared across packages via
// _test.go files.
func generatePlaybackTestVideo(t *testing.T, dir, name string, durationSec int) string {
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

// createTestSource POSTs a new media source and returns its id.
func createTestSource(t *testing.T, ts *httptest.Server, name, path string) int64 {
	t.Helper()
	body := `{"name":"` + name + `","path":"` + path + `"}`
	resp, err := http.Post(ts.URL+"/api/sources", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create source failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 creating source, got %d: %s", resp.StatusCode, b)
	}
	var source struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&source); err != nil {
		t.Fatalf("decoding created source: %v", err)
	}
	return source.ID
}

// scanAndGetFirstItem triggers a scan of sourceID and returns the id of the
// first (only, in these tests) resulting media item.
func scanAndGetFirstItem(t *testing.T, ts *httptest.Server, sourceID int64) int64 {
	t.Helper()
	scanResp, err := http.Post(ts.URL+"/api/sources/"+strconv.FormatInt(sourceID, 10)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	scanResp.Body.Close()
	if scanResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from scan, got %d", scanResp.StatusCode)
	}

	mediaResp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	defer mediaResp.Body.Close()
	var items []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(mediaResp.Body).Decode(&items); err != nil {
		t.Fatalf("decoding media items: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one media item after scan, got none")
	}
	return items[0].ID
}
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run TestMediaStream`
Expected: FAIL — `undefined: s.handleMediaStream` / route not registered (compile error until Step 7).

- [ ] **Step 7: Write the direct-play handler**

`internal/api/playback_handlers.go`:

```go
package api

import (
	"net/http"
	"os"
	"strconv"

	"personaltv/internal/playback"
)

func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	item, err := s.items.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	source, err := s.sources.Get(r.Context(), item.SourceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	path := playback.ResolvePath(source, item)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	http.ServeContent(w, r, item.Title, info.ModTime(), f)
}
```

- [ ] **Step 8: Register the route**

In `internal/api/router.go`, add this line inside `Routes()`, grouped with the other `/api/media` routes (after `mux.HandleFunc("GET /api/media", s.handleListMedia)`):

```go
	mux.HandleFunc("GET /api/media/{id}/stream", s.handleMediaStream)
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/api/... ./internal/playback/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add internal/playback/resolve.go internal/playback/resolve_test.go internal/api/playback_handlers.go internal/api/playback_handlers_test.go internal/api/router.go
git commit -m "feat: add media path resolution and direct-play streaming endpoint"
```

---

## Task 3: SessionManager — `ffmpeg` Transcode Sessions and Idle-Timeout Cleanup

**Files:**
- Create: `internal/playback/session.go`
- Create: `internal/playback/service.go`
- Test: `internal/playback/session_test.go`

**Interfaces:**
- Consumes: nothing new from earlier tasks (a self-contained subsystem).
- Produces: `playback.NewSessionManager(baseDir string, idleTimeout time.Duration) *SessionManager`, `(*SessionManager) StartSession(mediaPath string, offsetSec float64) (*Session, error)`, `(*SessionManager) Get(id string) (*Session, bool)`, `(*SessionManager) Touch(id string)`, `(*SessionManager) Sweep(now time.Time)`, `(*SessionManager) Run(ctx context.Context, interval time.Duration)`, `(*SessionManager) Close()`, `playback.CleanOrphanedSessions(baseDir string) error`, and the `Session` type (`ID string`, `Dir string` exported fields, plus `(*Session) Failed() (bool, error)`). **Task 4's session-serving endpoint consumes `Get`/`Touch`/`Session.Failed`/`Session.Dir`. Task 5's `TuneIn` consumes `StartSession`. Task 6's `main.go` consumes `NewSessionManager`, `Run`, `Close`, `CleanOrphanedSessions`.**
  Also produces `playback.Service` (an empty-but-typed struct with just a constructor for now) and `playback.NewService(channelSvc *channels.Service, sources repository.MediaSourceRepository, items repository.MediaItemRepository, sessions *SessionManager) *Service`. **Task 4 adds delegating methods to this same type; Task 5 adds `TuneIn` to it — its constructor signature is fixed here and does not change in either later task.**

- [ ] **Step 1: Add the `uuid` dependency**

`github.com/google/uuid` is already an indirect dependency of this module (pulled in transitively). This task is what first imports it directly:

```bash
cd /home/daslaptop/HomeStreamProject
go get github.com/google/uuid
```

- [ ] **Step 2: Write the failing tests**

`internal/playback/session_test.go`:

```go
package playback

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func generateSessionTestVideo(t *testing.T, dir, name string, durationSec int) string {
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

func TestSessionManager_StartSession_ProducesPlaylistAndSegments(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 6)

	m := NewSessionManager(t.TempDir(), time.Minute)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	defer m.Close()

	if _, err := os.Stat(filepath.Join(sess.Dir, "playlist.m3u8")); err != nil {
		t.Errorf("expected playlist.m3u8 to exist: %v", err)
	}

	// StartSession only waits for the playlist itself; give ffmpeg a short
	// moment to also flush at least one real segment.
	deadline := time.Now().Add(5 * time.Second)
	var sawSegment bool
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(sess.Dir)
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".ts" {
				sawSegment = true
			}
		}
		if sawSegment {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawSegment {
		t.Error("expected at least one .ts segment to be produced")
	}

	got, ok := m.Get(sess.ID)
	if !ok || got != sess {
		t.Error("expected Get to return the started session")
	}
}

func TestSessionManager_StartSession_FailsOnMissingInput(t *testing.T) {
	m := NewSessionManager(t.TempDir(), time.Minute)
	_, err := m.StartSession("/no/such/file.mp4", 0)
	if err == nil {
		t.Fatal("expected an error for a nonexistent input file")
	}
}

func TestSessionManager_Sweep_RemovesIdleSessions(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 6)

	m := NewSessionManager(t.TempDir(), 200*time.Millisecond)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	// StartSession's own (variable-latency) ffmpeg startup shouldn't count
	// against the idle window under test — touch explicitly right before
	// exercising it, so this test only measures Sweep's own behavior.
	m.Touch(sess.ID)

	// Not idle yet: sweeping "now" (well within the timeout) must not
	// remove it.
	m.Sweep(time.Now())
	if _, ok := m.Get(sess.ID); !ok {
		t.Fatal("expected the fresh session to survive an immediate sweep")
	}

	// Sweeping as if idleTimeout had already elapsed since the touch must
	// remove it.
	m.Sweep(time.Now().Add(300 * time.Millisecond))
	if _, ok := m.Get(sess.ID); ok {
		t.Error("expected the idle session to be removed")
	}
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Error("expected the session directory to be removed")
	}
}

func TestSessionManager_Touch_ResetsIdleClock(t *testing.T) {
	videoDir := t.TempDir()
	video := generateSessionTestVideo(t, videoDir, "movie.mp4", 6)

	m := NewSessionManager(t.TempDir(), 200*time.Millisecond)
	sess, err := m.StartSession(video, 0)
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	defer m.Close()

	future := time.Now().Add(150 * time.Millisecond)
	m.Touch(sess.ID)
	// A sweep at "future" is only ~150ms after the touch, under the 200ms
	// timeout: the session must survive.
	m.Sweep(future)
	if _, ok := m.Get(sess.ID); !ok {
		t.Error("expected a touched session to survive a sweep within the idle window")
	}
}

func TestCleanOrphanedSessions(t *testing.T) {
	base := t.TempDir()
	leftover := filepath.Join(base, "orphaned-session-id")
	if err := os.MkdirAll(leftover, 0755); err != nil {
		t.Fatalf("setting up leftover dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leftover, "playlist.m3u8"), []byte("stale"), 0644); err != nil {
		t.Fatalf("writing leftover file: %v", err)
	}

	if err := CleanOrphanedSessions(base); err != nil {
		t.Fatalf("CleanOrphanedSessions returned error: %v", err)
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("reading base dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected the leftover session directory to be removed, found %v", entries)
	}
}

func TestCleanOrphanedSessions_MissingBaseDirIsNotAnError(t *testing.T) {
	base := filepath.Join(t.TempDir(), "does-not-exist-yet")
	if err := CleanOrphanedSessions(base); err != nil {
		t.Errorf("expected no error for a base directory that doesn't exist yet, got %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/playback/... -run 'TestSessionManager|TestCleanOrphanedSessions'`
Expected: FAIL — build errors (`SessionManager`, `NewSessionManager`, `CleanOrphanedSessions` don't exist yet).

- [ ] **Step 4: Write the SessionManager implementation**

`internal/playback/session.go`:

```go
package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session is one viewer's active HLS transcode: a running ffmpeg process
// writing segments and a playlist into Dir.
type Session struct {
	ID  string
	Dir string

	mu       sync.Mutex
	cmd      *exec.Cmd
	lastUsed time.Time
	failed   bool
	failErr  error
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *Session) idleSince(cutoff time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsed.Before(cutoff)
}

func (s *Session) markFailed(err error) {
	s.mu.Lock()
	s.failed = true
	s.failErr = err
	s.mu.Unlock()
}

// Failed reports whether this session's ffmpeg process has exited or
// crashed. Once true, the session's playlist/segment endpoint should stop
// serving it as if more content were coming.
func (s *Session) Failed() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed, s.failErr
}

func (s *Session) kill() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// SessionManager owns every active transcode session: starting new ones,
// looking them up, and cleaning up idle ones. Entirely in-memory — nothing
// here is persisted, matching this product's principle that nothing is
// lost on restart and nothing ticks with no viewers (a server restart
// simply drops all active sessions).
type SessionManager struct {
	baseDir     string
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewSessionManager(baseDir string, idleTimeout time.Duration) *SessionManager {
	return &SessionManager{
		baseDir:     baseDir,
		idleTimeout: idleTimeout,
		sessions:    make(map[string]*Session),
	}
}

// StartSession starts an ffmpeg process transcoding mediaPath to HLS
// beginning at offsetSec, and waits (bounded by a short startup timeout)
// for it to actually produce a playlist before returning — so a bad
// input/binary fails this call directly, rather than only surfacing later
// as a 404 on the first segment request.
func (m *SessionManager) StartSession(mediaPath string, offsetSec float64) (*Session, error) {
	id := uuid.New().String()
	dir := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}

	playlistPath := filepath.Join(dir, "playlist.m3u8")
	segmentPattern := filepath.Join(dir, "segment%03d.ts")

	// exec.Command, not exec.CommandContext: this process must outlive the
	// HTTP request that starts it. Its lifecycle is owned by this
	// SessionManager — torn down by the idle sweep (Sweep) or on server
	// shutdown (Close) — not by any single request's context.
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", strconv.FormatFloat(offsetSec, 'f', 3, 64),
		"-i", mediaPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_playlist_type", "event",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}

	sess := &Session{ID: id, Dir: dir, cmd: cmd, lastUsed: time.Now()}

	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			sess.markFailed(fmt.Errorf("ffmpeg exited: %w: %s", waitErr, stderr.String()))
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(playlistPath); statErr == nil {
			m.mu.Lock()
			m.sessions[id] = sess
			m.mu.Unlock()
			return sess, nil
		}
		if failed, ferr := sess.Failed(); failed {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("ffmpeg failed to start: %w", ferr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	sess.kill()
	os.RemoveAll(dir)
	return nil, errors.New("ffmpeg did not produce a playlist within the startup timeout")
}

// Get returns the session with the given id, if one is currently tracked.
func (m *SessionManager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Touch records that id was just accessed (a playlist or segment
// request), resetting its idle clock. A no-op if id isn't tracked.
func (m *SessionManager) Touch(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.touch()
	}
}

// Sweep tears down (kills the process, removes the directory) every
// session that hasn't been touched since idleTimeout before now. Called
// periodically by Run in production; called directly with a controlled
// now in tests, so tests never wait out a real idle timeout.
func (m *SessionManager) Sweep(now time.Time) {
	cutoff := now.Add(-m.idleTimeout)

	m.mu.Lock()
	var idle []*Session
	for id, s := range m.sessions {
		if s.idleSince(cutoff) {
			idle = append(idle, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	for _, s := range idle {
		s.kill()
		os.RemoveAll(s.Dir)
	}
}

// Run periodically sweeps idle sessions until ctx is cancelled. Intended
// to be started once, in its own goroutine, at server startup.
func (m *SessionManager) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.Sweep(now)
		}
	}
}

// Close tears down every currently active session immediately, regardless
// of idle time. Intended for graceful server shutdown, so no ffmpeg
// process is ever left running after the server process exits.
func (m *SessionManager) Close() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range all {
		s.kill()
		os.RemoveAll(s.Dir)
	}
}

// CleanOrphanedSessions removes any leftover session directories under
// baseDir from an unclean prior shutdown (e.g. the process was killed
// before Close() ran). Call once at startup, before serving traffic.
func CleanOrphanedSessions(baseDir string) error {
	entries, err := os.ReadDir(baseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(baseDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Write the `Service` scaffold**

`internal/playback/service.go`:

```go
package playback

import (
	"personaltv/internal/channels"
	"personaltv/internal/repository"
)

// Service ties together the schedule (channels.Service), the repositories
// needed to resolve a media item to a file, and the SessionManager that
// owns transcode sessions. Task 4 adds session-lookup delegating methods;
// Task 5 adds TuneIn, its primary operation. This constructor's signature
// is fixed here and does not change in either later task.
type Service struct {
	channels *channels.Service
	sources  repository.MediaSourceRepository
	items    repository.MediaItemRepository
	sessions *SessionManager
}

func NewService(channelSvc *channels.Service, sources repository.MediaSourceRepository, items repository.MediaItemRepository, sessions *SessionManager) *Service {
	return &Service{channels: channelSvc, sources: sources, items: items, sessions: sessions}
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/playback/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add go.mod go.sum internal/playback/session.go internal/playback/session_test.go internal/playback/service.go
git commit -m "feat: add SessionManager for ffmpeg HLS transcode sessions"
```

---

## Task 4: HLS Session-Serving Endpoint

**Files:**
- Modify: `internal/playback/service.go` (add `GetSession`/`TouchSession` delegating methods)
- Modify: `internal/api/router.go` (add `playback` field + `SetPlaybackService` method + route registration)
- Modify: `internal/api/playback_handlers.go` (add the session-file handler)
- Modify: `internal/api/sources_handlers_test.go` (wire a `playback.Service` into the shared `newTestServer`/`newTestServerWithConn` test helpers)
- Test: `internal/api/playback_handlers_test.go` (append)

**Interfaces:**
- Consumes: `playback.SessionManager.Get`/`Touch`/`Session.Failed`/`Session.Dir` (Task 3), `playback.Service`/`NewService` (Task 3).
- Produces: `(*playback.Service) GetSession(id string) (*playback.Session, bool)`, `(*playback.Service) TouchSession(id string)`. `(*api.Server) SetPlaybackService(p *playback.Service)` — additive, mirrors the existing `SetStaticHandler` pattern exactly; **the 15 pre-existing `api.NewServer(...)`/`srv.Routes()` call sites remain untouched.** `GET /api/playback/sessions/{id}/{file}`. **Task 5's `TuneIn` and Task 6's `main.go` both rely on `Server.SetPlaybackService` existing; Task 5 adds `TuneIn` to the same `Service` type these delegating methods live on.**

- [ ] **Step 1: Write the failing tests**

Append to `internal/api/playback_handlers_test.go`:

```go

func TestPlaybackSession_ServesPlaylistAndSegmentsAndTracksLastUse(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	videoDir := t.TempDir()
	video := generatePlaybackTestVideo(t, videoDir, "movie.mp4", 6)

	sess, err := srv.PlaybackServiceForTest().StartTestSession(video)
	if err != nil {
		t.Fatalf("starting a test session: %v", err)
	}

	playlistResp, err := http.Get(ts.URL + "/api/playback/sessions/" + sess.ID + "/playlist.m3u8")
	if err != nil {
		t.Fatalf("GET playlist failed: %v", err)
	}
	defer playlistResp.Body.Close()
	if playlistResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for the playlist, got %d", playlistResp.StatusCode)
	}
	if ct := playlistResp.Header.Get("Content-Type"); ct != "application/vnd.apple.mpegurl" {
		t.Errorf("expected playlist content-type application/vnd.apple.mpegurl, got %q", ct)
	}
	playlistBody, _ := io.ReadAll(playlistResp.Body)
	if len(playlistBody) == 0 {
		t.Error("expected a non-empty playlist body")
	}
}

func TestPlaybackSession_404sForUnknownSession(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/playback/sessions/no-such-session/playlist.m3u8")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown session, got %d", resp.StatusCode)
	}
}

func TestPlaybackSession_RejectsPathTraversal(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	videoDir := t.TempDir()
	video := generatePlaybackTestVideo(t, videoDir, "movie.mp4", 6)
	sess, err := srv.PlaybackServiceForTest().StartTestSession(video)
	if err != nil {
		t.Fatalf("starting a test session: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/playback/sessions/" + sess.ID + "/..%2f..%2f..%2fetc%2fpasswd")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected a path-traversal attempt to be rejected, got 200")
	}
}
```

This test needs a way to start a real session without going through the not-yet-built `TuneIn` (Task 5) — it calls `srv.PlaybackServiceForTest().StartTestSession(video)`, both added in the steps below.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run TestPlaybackSession`
Expected: FAIL — `s.PlaybackServiceForTest` and `(*playback.Service).StartTestSession` don't exist yet (compile error).

- [ ] **Step 3: Add the delegating methods to `Service`**

In `internal/playback/service.go`, add below `NewService`:

```go

// GetSession and TouchSession delegate to the underlying SessionManager —
// the session-serving HTTP handler (internal/api) only ever talks to
// Service, never to a SessionManager directly.
func (svc *Service) GetSession(id string) (*Session, bool) {
	return svc.sessions.Get(id)
}

func (svc *Service) TouchSession(id string) {
	svc.sessions.Touch(id)
}

// StartTestSession starts a real transcode session directly against the
// given media path, bypassing TuneIn's schedule/compatibility logic
// entirely. Exported only for tests that need a real session to exist
// without depending on Task 5's tune-in orchestration.
func (svc *Service) StartTestSession(mediaPath string) (*Session, error) {
	return svc.sessions.StartSession(mediaPath, 0)
}
```

- [ ] **Step 4: Add the `playback` field and `SetPlaybackService` to `Server`**

In `internal/api/router.go`, add `playback *playback.Service` to the `Server` struct (below the existing `static http.Handler` field), add the import `"personaltv/internal/playback"`, and add this method below `SetStaticHandler`:

```go
// SetPlaybackService registers the playback service used by the
// tune-in/direct-play/session-serving endpoints. If never called, those
// endpoints will panic on a nil Service — every existing test and
// NewServer call site that doesn't touch playback is unaffected, since
// none of them exercise these routes.
func (s *Server) SetPlaybackService(p *playback.Service) {
	s.playback = p
}

// PlaybackServiceForTest exposes the wired playback.Service to this
// package's own tests (internal/api/playback_handlers_test.go), so tests
// can start a real session directly via Service.StartTestSession. Not
// part of the public HTTP API.
func (s *Server) PlaybackServiceForTest() *playback.Service {
	return s.playback
}
```

- [ ] **Step 5: Write the session-file handler**

Append to `internal/api/playback_handlers.go` (and add `"errors"`, `"fmt"`, `"path/filepath"` to its imports):

```go

func (s *Server) handleSessionFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")

	sess, ok := s.playback.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such playback session"))
		return
	}
	if failed, ferr := sess.Failed(); failed {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("playback session failed: %w", ferr))
		return
	}

	// file must be a plain filename within the session's own directory —
	// no path traversal (e.g. "../../etc/passwd") outside it.
	if file != filepath.Base(file) {
		writeError(w, http.StatusBadRequest, errors.New("invalid file name"))
		return
	}
	path := filepath.Join(sess.Dir, file)

	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()

	s.playback.TouchSession(id)

	switch filepath.Ext(file) {
	case ".m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	}

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeContent(w, r, file, info.ModTime(), f)
}
```

- [ ] **Step 6: Register the route**

In `internal/api/router.go`, add this line inside `Routes()`, after the `GET /api/media/{id}/stream` line added in Task 2:

```go
	mux.HandleFunc("GET /api/playback/sessions/{id}/{file}", s.handleSessionFile)
```

- [ ] **Step 7: Wire a `playback.Service` into the shared test server helpers**

In `internal/api/sources_handlers_test.go`, add `"time"` and `"personaltv/internal/playback"` to the imports, and update both `newTestServer` and `newTestServerWithConn` to construct and wire a real (but short-lived-per-test) `playback.Service`:

```go
func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	srv := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)

	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	srv.SetPlaybackService(playback.NewService(channelSvc, sourceRepo, itemRepo, sessions))

	return srv
}

func newTestServerWithConn(t *testing.T, conn *sql.DB) *api.Server {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	srv := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)

	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	srv.SetPlaybackService(playback.NewService(channelSvc, sourceRepo, itemRepo, sessions))

	return srv
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/api/... ./internal/playback/...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add internal/playback/service.go internal/api/router.go internal/api/playback_handlers.go internal/api/playback_handlers_test.go internal/api/sources_handlers_test.go
git commit -m "feat: add HLS session-serving endpoint"
```

---

## Task 5: Tune-In Orchestration and the Watch Endpoint

**Files:**
- Create: `internal/playback/tunein.go`
- Modify: `internal/api/playback_handlers.go` (add the watch handler)
- Modify: `internal/api/router.go` (route registration)
- Test: `internal/playback/tunein_test.go`
- Test: `internal/api/playback_handlers_test.go` (append)

**Interfaces:**
- Consumes: `channels.Service.CurrentState`/`ListPrograms` (Plan 1), `playback.IsDirectPlayCompatible` (Task 1), `playback.ResolvePath` (Task 2), `(*SessionManager) StartSession` via `Service.sessions` (Task 3), `repository.ErrNotFound` (Plan 1).
- Produces: `playback.TuneInResult{Status, Mode, MediaItemID, OffsetSec, SessionID}` and `(*Service) TuneIn(ctx context.Context, channelID int64, now time.Time) (*TuneInResult, error)`. `POST /api/channels/{id}/watch`. **Task 6's end-to-end test is the only further consumer — this is the plan's key product-facing deliverable, mirroring how Plan 1's `/now` endpoint was its own.**

Per spec §4: TuneIn stays a pure function of `(schedule, now, which files exist)` — it never advances to a program before its own scheduled start time, even to route around a missing file at the current slot. A missing file is excluded from scheduling exactly like an invalid/zero-duration item already is in `channels.Service.CurrentState`, never "skipped ahead to." Concretely, this means evaluating the schedule twice: once exactly as `CurrentState` already does (to detect genuine off-air, with zero divergence risk from the existing `/now` endpoint's own behavior), and once more with an additional file-existence filter (to detect "scheduled, but nothing playable" — a distinct `unavailable` state).

- [ ] **Step 1: Write the failing tests**

`internal/playback/tunein_test.go`:

```go
package playback_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/playback"
	"personaltv/internal/repository/sqlite"
)

func generateTuneInTestVideo(t *testing.T, dir, name string, durationSec int) string {
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

type tuneInFixture struct {
	ctx      context.Context
	svc      *playback.Service
	channels *channels.Service
	items    *sqlite.MediaItemRepository
	sourceID int64
	mediaDir string
}

func newTuneInFixture(t *testing.T) *tuneInFixture {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	svc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	ctx := context.Background()
	mediaDir := t.TempDir()
	source := &model.MediaSource{Name: "Movies", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("creating source: %v", err)
	}

	return &tuneInFixture{
		ctx: ctx, svc: svc, channels: channelSvc, items: itemRepo,
		sourceID: source.ID, mediaDir: mediaDir,
	}
}

// addItem inserts a media item row. If relPath names a file that doesn't
// actually exist under the fixture's media directory, the resulting item
// is "scheduled but not playable" — exactly the missing-file case TuneIn
// must exclude.
func (f *tuneInFixture) addItem(t *testing.T, relPath string, durationSec float64, videoCodec, audioCodec, container string) *model.MediaItem {
	t.Helper()
	item := &model.MediaItem{
		SourceID: f.sourceID, RelPath: relPath, Title: relPath,
		DurationSec: durationSec, VideoCodec: videoCodec, AudioCodec: audioCodec,
		Container: container, ModTime: time.Now().UTC(),
	}
	if err := f.items.Upsert(f.ctx, item); err != nil {
		t.Fatalf("upserting item %s: %v", relPath, err)
	}
	return item
}

func (f *tuneInFixture) addChannel(t *testing.T) *model.Channel {
	t.Helper()
	channel := &model.Channel{Name: "Test Channel", Enabled: true}
	if err := f.channels.CreateChannel(f.ctx, channel); err != nil {
		t.Fatalf("creating channel: %v", err)
	}
	return channel
}

func (f *tuneInFixture) addProgram(t *testing.T, channelID, mediaItemID int64, startTime time.Time) {
	t.Helper()
	program := &model.Program{ChannelID: channelID, MediaItemID: mediaItemID, StartTime: startTime}
	if err := f.channels.AddProgram(f.ctx, program); err != nil {
		t.Fatalf("adding program: %v", err)
	}
}

func TestTuneIn_OffAirWhenNothingScheduled(t *testing.T) {
	f := newTuneInFixture(t)
	channel := f.addChannel(t)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "off_air" {
		t.Errorf("expected status off_air, got %+v", result)
	}
}

func TestTuneIn_DirectPlayForCompatibleFile(t *testing.T) {
	f := newTuneInFixture(t)
	generateTuneInTestVideo(t, f.mediaDir, "movie.mp4", 10)
	item := f.addItem(t, "movie.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "playing" || result.Mode != "direct" {
		t.Fatalf("expected playing/direct, got %+v", result)
	}
	if result.MediaItemID != item.ID {
		t.Errorf("expected media item %d, got %d", item.ID, result.MediaItemID)
	}
	if result.OffsetSec < 2.5 || result.OffsetSec > 4 {
		t.Errorf("expected offset ~3s, got %v", result.OffsetSec)
	}
}

func TestTuneIn_HLSForIncompatibleFile(t *testing.T) {
	f := newTuneInFixture(t)
	generateTuneInTestVideo(t, f.mediaDir, "movie.mp4", 10)
	// A real h264/aac file, but its stored codec info is deliberately set
	// to something the compatibility matrix (Task 1) rejects — this
	// exercises the hls path without needing an actual hevc/vp9 encoder
	// to be available wherever this test runs.
	item := f.addItem(t, "movie.mp4", 10, "hevc", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "playing" || result.Mode != "hls" {
		t.Fatalf("expected playing/hls, got %+v", result)
	}
	if result.SessionID == "" {
		t.Error("expected a non-empty session id")
	}
	if _, ok := f.svc.GetSession(result.SessionID); !ok {
		t.Error("expected the returned session id to be a real, tracked session")
	}
}

func TestTuneIn_UnavailableWhenScheduledFileIsMissing(t *testing.T) {
	f := newTuneInFixture(t)
	// No file is ever generated at this RelPath — it's scheduled but
	// missing on disk.
	item := f.addItem(t, "missing.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)
	start := time.Now().UTC().Add(-3 * time.Second)
	f.addProgram(t, channel.ID, item.ID, start)

	result, err := f.svc.TuneIn(f.ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	if result.Status != "unavailable" {
		t.Errorf("expected status unavailable, got %+v", result)
	}
}

func TestTuneIn_DoesNotJumpAheadToAFutureProgramWhenCurrentFileIsMissing(t *testing.T) {
	f := newTuneInFixture(t)
	missingItem := f.addItem(t, "missing.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	generateTuneInTestVideo(t, f.mediaDir, "future.mp4", 10)
	futureItem := f.addItem(t, "future.mp4", 10, "h264", "aac", "mov,mp4,m4a,3gp,3g2,mj2")
	channel := f.addChannel(t)

	now := time.Now().UTC()
	// missingItem is airing right now (its file just happens to be
	// missing); futureItem is scheduled to start later today.
	f.addProgram(t, channel.ID, missingItem.ID, now.Add(-3*time.Second))
	f.addProgram(t, channel.ID, futureItem.ID, now.Add(time.Hour))

	result, err := f.svc.TuneIn(f.ctx, channel.ID, now)
	if err != nil {
		t.Fatalf("TuneIn returned error: %v", err)
	}
	// Must report unavailable right now, not jump ahead to futureItem
	// before its own scheduled start time — channel state stays a pure
	// function of (schedule, now), never advancing early.
	if result.Status != "unavailable" {
		t.Fatalf("expected status unavailable (not jumping ahead to a future program), got %+v", result)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/playback/... -run TestTuneIn`
Expected: FAIL — `undefined: (*Service).TuneIn`

- [ ] **Step 3: Write the implementation**

`internal/playback/tunein.go`:

```go
package playback

import (
	"context"
	"errors"
	"os"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/repository"
	"personaltv/internal/scheduler"
)

// TuneInResult is what a viewer sees when they tune in to a channel right
// now.
type TuneInResult struct {
	Status      string // "playing", "off_air", or "unavailable"
	Mode        string // "direct" or "hls"; empty unless Status == "playing"
	MediaItemID int64
	OffsetSec   float64
	SessionID   string // only set when Mode == "hls"
}

// TuneIn decides what a viewer sees when they tune in to channelID right
// now. It stays a pure function of (schedule, wall-clock time, which
// files exist on disk) — it never advances to a program before its own
// scheduled start time, matching this product's core principle that
// channel state is always recomputed from (schedule, now), never tracked
// as separate mutable state. A file missing at its scheduled slot is
// therefore treated exactly like an invalid/zero-duration item already is
// (see channels.Service.CurrentState): excluded from scheduling for this
// evaluation, never "skipped ahead to."
func (svc *Service) TuneIn(ctx context.Context, channelID int64, now time.Time) (*TuneInResult, error) {
	// Pass A: exactly what channels.CurrentState computes (schedulable per
	// the DB — not yet filtered by file existence). If nothing is
	// scheduled at all right now, this is off-air, identical to what
	// GET /api/channels/{id}/now already reports — there's no candidate to
	// even check for file existence.
	stateA, err := svc.channels.CurrentState(ctx, channelID, now)
	if err != nil {
		return nil, err
	}
	if stateA.Current == nil {
		return &TuneInResult{Status: "off_air"}, nil
	}

	// Pass B: the same schedule, additionally excluding any program whose
	// underlying file doesn't exist on disk right now.
	programs, err := svc.channels.ListPrograms(ctx, channelID)
	if err != nil {
		return nil, err
	}

	playable := make([]scheduler.ScheduledProgram, 0, len(programs))
	itemsByID := make(map[int64]*model.MediaItem, len(programs))
	sourcesByID := make(map[int64]*model.MediaSource)
	for _, p := range programs {
		item, err := svc.items.Get(ctx, p.MediaItemID)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if item.Invalid || item.DurationSec <= 0 {
			continue
		}
		source, ok := sourcesByID[item.SourceID]
		if !ok {
			source, err = svc.sources.Get(ctx, item.SourceID)
			if err != nil {
				continue // source itself gone; item can't be playable
			}
			sourcesByID[item.SourceID] = source
		}
		if _, statErr := os.Stat(ResolvePath(source, item)); statErr != nil {
			continue // file missing: excluded from scheduling, not skipped-to
		}

		itemsByID[item.ID] = item
		playable = append(playable, scheduler.ScheduledProgram{
			ProgramID:   p.ID,
			MediaItemID: p.MediaItemID,
			StartTime:   p.StartTime,
			Duration:    time.Duration(item.DurationSec * float64(time.Second)),
		})
	}

	stateB := scheduler.Evaluate(playable, now)
	if stateB.Current == nil {
		// Something IS scheduled right now per pass A, but its file (and
		// nothing else covering this exact moment) is playable.
		return &TuneInResult{Status: "unavailable"}, nil
	}

	item := itemsByID[stateB.Current.MediaItemID]
	source := sourcesByID[item.SourceID]
	offsetSec := stateB.Offset.Seconds()

	if IsDirectPlayCompatible(item.VideoCodec, item.AudioCodec, item.Container) {
		return &TuneInResult{
			Status: "playing", Mode: "direct",
			MediaItemID: item.ID, OffsetSec: offsetSec,
		}, nil
	}

	sess, err := svc.sessions.StartSession(ResolvePath(source, item), offsetSec)
	if err != nil {
		return nil, err
	}
	return &TuneInResult{
		Status: "playing", Mode: "hls",
		MediaItemID: item.ID, OffsetSec: offsetSec, SessionID: sess.ID,
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/playback/...`
Expected: PASS

- [ ] **Step 5: Write the failing tests for the watch endpoint**

Append to `internal/api/playback_handlers_test.go`:

```go

func TestChannelWatch_ReturnsOffAirForChannelWithNothingScheduled(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	chResp, err := http.Post(ts.URL+"/api/channels", "application/json", strings.NewReader(`{"name":"Empty"}`))
	if err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	defer chResp.Body.Close()
	var channel struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(chResp.Body).Decode(&channel)

	watchResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/watch", "application/json", nil)
	if err != nil {
		t.Fatalf("POST watch failed: %v", err)
	}
	defer watchResp.Body.Close()
	if watchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", watchResp.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
	}
	json.NewDecoder(watchResp.Body).Decode(&result)
	if result.Status != "off_air" {
		t.Errorf("expected status off_air, got %q", result.Status)
	}
}

func TestChannelWatch_404sForUnknownChannel(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/channels/999/watch", "application/json", nil)
	if err != nil {
		t.Fatalf("POST watch failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown channel, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run TestChannelWatch`
Expected: FAIL — `undefined: s.handleChannelWatch`

- [ ] **Step 7: Write the watch handler**

Append to `internal/api/playback_handlers.go` (add `"time"` to its imports):

```go

func (s *Server) handleChannelWatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	result, err := s.playback.TuneIn(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, watchResponse{
		Status:      result.Status,
		Mode:        result.Mode,
		MediaItemID: result.MediaItemID,
		OffsetSec:   result.OffsetSec,
		SessionID:   result.SessionID,
	})
}

type watchResponse struct {
	Status      string  `json:"status"`
	Mode        string  `json:"mode,omitempty"`
	MediaItemID int64   `json:"media_item_id,omitempty"`
	OffsetSec   float64 `json:"offset_sec,omitempty"`
	SessionID   string  `json:"session_id,omitempty"`
}
```

- [ ] **Step 8: Register the route**

In `internal/api/router.go`, add this line inside `Routes()`, grouped with the other `/api/channels/{id}` routes (after `mux.HandleFunc("GET /api/channels/{id}/now", s.handleChannelNow)`):

```go
	mux.HandleFunc("POST /api/channels/{id}/watch", s.handleChannelWatch)
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/api/... ./internal/playback/...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add internal/playback/tunein.go internal/playback/tunein_test.go internal/api/playback_handlers.go internal/api/playback_handlers_test.go internal/api/router.go
git commit -m "feat: add tune-in orchestration and the watch endpoint"
```

---

## Task 6: Wire `main.go` and Add the End-to-End Test

**Files:**
- Modify: `cmd/personaltv/main.go`
- Modify: `internal/integration/end_to_end_test.go`

**Interfaces:**
- Consumes: `playback.CleanOrphanedSessions`, `playback.NewSessionManager`, `(*SessionManager) Run`/`Close` (Task 3), `playback.NewService` (Task 3), `(*api.Server) SetPlaybackService` (Task 4), the full stack from Tasks 1-5.
- Produces: a fully wired `go run ./cmd/personaltv` binary with working playback, and this plan's Definition-of-Done proof.

- [ ] **Step 1: Update `main.go`**

Replace the full contents of `cmd/personaltv/main.go`:

```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"personaltv/internal/api"
	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/playback"
	"personaltv/internal/repository/sqlite"
	"personaltv/web"
)

func main() {
	dbPath := getEnv("PERSONALTV_DB_PATH", "personaltv.db")
	port := getEnv("PERSONALTV_PORT", "8080")

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)

	sessionsDir := filepath.Join(os.TempDir(), "personaltv-playback")
	if err := playback.CleanOrphanedSessions(sessionsDir); err != nil {
		log.Printf("warning: failed to clean orphaned playback sessions: %v", err)
	}
	sessions := playback.NewSessionManager(sessionsDir, 60*time.Second)
	playbackSvc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go sessions.Run(sweepCtx, 30*time.Second)

	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	server.SetPlaybackService(playbackSvc)

	webHandler, err := web.Handler()
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}
	server.SetStaticHandler(webHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: server.Routes(),
		// ReadHeaderTimeout and IdleTimeout bound how long a slow or idle
		// client can hold a connection. WriteTimeout is deliberately left
		// unset: playback streams long-lived responses (direct-play range
		// requests, HLS segments), and a write deadline would cut those
		// off mid-stream.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// SIGINT/SIGTERM (what `docker stop` sends) must drain in-flight requests
	// instead of killing them mid-response.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Personal TV listening on :%s (db: %s)", port, dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("server failed: %v", err)
		}
	case <-ctx.Done():
		stop() // restore default signal handling: a second signal aborts immediately
		log.Println("shutdown signal received, draining in-flight requests")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		stopSweep()
		sessions.Close()
		log.Println("Personal TV stopped")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Run the existing suite to verify `main.go` still builds and nothing regressed**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (this step has no new test of its own yet — it's a checkpoint before adding the end-to-end test in the next step)

- [ ] **Step 3: Write the failing end-to-end test**

In `internal/integration/end_to_end_test.go`, add `"io"`, `"os"`, and `"personaltv/internal/playback"` to the import block (alongside the existing imports — `os/exec` stays, this adds the separate `os` package too), then append this test function at the end of the file:

```go

// TestFullUserJourney_Playback extends the same real-user-journey pattern
// as TestFullUserJourney to prove tuning in and streaming actually works
// end-to-end through the full stack: a media source, a scheduled channel,
// POST .../watch, and a real byte range fetched from the resulting stream
// URL. This is Plan 3's Definition of Done, the playback equivalent of
// TestFullUserJourney.
func TestFullUserJourney_Playback(t *testing.T) {
	mediaDir := t.TempDir()
	videoPath := generateTestVideo(t, mediaDir, "movie-a.mp4", 10)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)

	sessions := playback.NewSessionManager(t.TempDir(), time.Minute)
	playbackSvc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	server.SetPlaybackService(playbackSvc)
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	// 1. configure and scan a media source
	srcBody, _ := json.Marshal(map[string]any{"name": "Movies", "path": mediaDir})
	srcResp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(srcBody))
	if err != nil {
		t.Fatalf("create source request failed: %v", err)
	}
	var source model.MediaSource
	json.NewDecoder(srcResp.Body).Decode(&source)
	srcResp.Body.Close()

	scanResp, err := http.Post(ts.URL+"/api/sources/"+strconv.FormatInt(source.ID, 10)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatalf("scan request failed: %v", err)
	}
	scanResp.Body.Close()

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

	// 2. create a channel and schedule the item on it, starting a few
	// seconds ago so it's playing now (same timing rationale as
	// TestFullUserJourney above: keep the offset well under the 10s
	// generated video's duration).
	chBody, _ := json.Marshal(map[string]any{"name": "Movies"})
	chResp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(chBody))
	if err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	var channel model.Channel
	json.NewDecoder(chResp.Body).Decode(&channel)
	chResp.Body.Close()

	start := time.Now().UTC().Add(-3 * time.Second)
	progBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(progBody))
	if err != nil {
		t.Fatalf("add program failed: %v", err)
	}
	progResp.Body.Close()

	// 3. tune in
	watchResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/watch", "application/json", nil)
	if err != nil {
		t.Fatalf("watch request failed: %v", err)
	}
	defer watchResp.Body.Close()
	var watch struct {
		Status      string  `json:"status"`
		Mode        string  `json:"mode"`
		MediaItemID int64   `json:"media_item_id"`
		OffsetSec   float64 `json:"offset_sec"`
	}
	json.NewDecoder(watchResp.Body).Decode(&watch)

	if watch.Status != "playing" || watch.Mode != "direct" {
		t.Fatalf("expected status=playing mode=direct for a plain h264/aac/mp4 file, got %+v", watch)
	}
	if watch.MediaItemID != item.ID {
		t.Errorf("expected media item %d, got %d", item.ID, watch.MediaItemID)
	}

	// 4. actually fetch the stream and confirm it's the real file
	streamResp, err := http.Get(ts.URL + "/api/media/" + strconv.FormatInt(watch.MediaItemID, 10) + "/stream")
	if err != nil {
		t.Fatalf("GET stream failed: %v", err)
	}
	defer streamResp.Body.Close()
	streamBody, _ := io.ReadAll(streamResp.Body)
	onDisk, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatalf("reading generated video: %v", err)
	}
	if len(streamBody) != len(onDisk) {
		t.Fatalf("expected the streamed body to match the file on disk (%d bytes), got %d bytes", len(onDisk), len(streamBody))
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/integration/... -run TestFullUserJourney_Playback`
Expected: FAIL — build error until `main.go`'s Step 1 and the import fix above are both in place (if Step 1-2 are already done, this instead fails on an assertion until the test itself is correct, but the code path already exists — this should in fact pass on the first real run once written correctly; treat any failure as a real bug to fix, not an expected-red step to shrug off).

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/integration/...`
Expected: PASS

- [ ] **Step 6: Run the full verification suite**

```bash
cd /home/daslaptop/HomeStreamProject
go build ./...
go vet ./...
gofmt -l .
go test ./... -race
```

Expected: everything passes/exits 0, `gofmt -l .` prints nothing.

- [ ] **Step 7: Manual sanity check**

Not automatable in this plan, but worth doing once: `go run ./cmd/personaltv`, then in another terminal, create a source/channel/program via `curl` (mirroring `TestFullUserJourney`'s steps), `curl -X POST .../watch`, and `curl` the resulting stream URL to confirm real video bytes come back.

- [ ] **Step 8: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add cmd/personaltv/main.go internal/integration/end_to_end_test.go
git commit -m "feat: wire playback into main.go and add the end-to-end test"
```

---

## Definition of Done

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./... -race` all pass from a clean checkout with Go 1.22+ and `ffmpeg`/`ffprobe` on `PATH` (unchanged prerequisites from Plan 1, now also required for this plan's own tests). `go run ./cmd/personaltv` serves a working playback backend: `POST /api/channels/{id}/watch` reports `off_air`/`unavailable`/`playing` correctly per the current schedule and which files actually exist on disk; a `playing`/`direct` response's media streams via `GET /api/media/{id}/stream` with working `Range` support; a `playing`/`hls` response's session streams via `GET /api/playback/sessions/{id}/{file}` with a real, growing HLS playlist; idle transcode sessions clean up their `ffmpeg` process and temp directory without intervention; and none of the 15 pre-existing API test call sites or routes changed. TV/player UI is not built (spec §1) — no frontend code in this plan.
