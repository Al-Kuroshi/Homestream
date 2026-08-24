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
