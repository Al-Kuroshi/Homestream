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
