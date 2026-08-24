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
