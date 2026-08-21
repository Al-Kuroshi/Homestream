package mediastore

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"personaltv/internal/model"
	"personaltv/internal/repository"
)

var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".m4v": true, ".webm": true, ".ts": true, ".wmv": true,
}

type Scanner struct {
	sources repository.MediaSourceRepository
	items   repository.MediaItemRepository
}

func NewScanner(sources repository.MediaSourceRepository, items repository.MediaItemRepository) *Scanner {
	return &Scanner{sources: sources, items: items}
}

// ScanSource walks the source's configured directory, probes new or
// changed video files, and prunes items whose file no longer exists.
// A single unreadable file is marked Invalid, not treated as a scan failure.
func (s *Scanner) ScanSource(ctx context.Context, sourceID int64) error {
	source, err := s.sources.Get(ctx, sourceID)
	if err != nil {
		return err
	}

	existing, err := s.items.ListBySource(ctx, sourceID)
	if err != nil {
		return err
	}
	existingByPath := make(map[string]*model.MediaItem, len(existing))
	for _, item := range existing {
		existingByPath[item.RelPath] = item
	}

	var seenRelPaths []string

	walkErr := filepath.WalkDir(source.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !videoExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		relPath, relErr := filepath.Rel(source.Path, path)
		if relErr != nil {
			return nil
		}
		seenRelPaths = append(seenRelPaths, relPath)

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		if existingItem, ok := existingByPath[relPath]; ok && !existingItem.Invalid {
			if existingItem.SizeBytes == info.Size() && existingItem.ModTime.Equal(info.ModTime().UTC()) {
				return nil // unchanged since last scan, skip re-probe
			}
		}

		item := &model.MediaItem{
			SourceID:  sourceID,
			RelPath:   relPath,
			Title:     titleFromFilename(path),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().UTC(),
		}

		if probeResult, probeErr := Probe(path); probeErr != nil {
			item.Invalid = true
		} else {
			item.DurationSec = probeResult.DurationSec
			item.VideoCodec = probeResult.VideoCodec
			item.AudioCodec = probeResult.AudioCodec
			item.Container = probeResult.Container
			item.Invalid = false
		}

		return s.items.Upsert(ctx, item)
	})
	if walkErr != nil {
		return walkErr
	}

	return s.items.DeleteMissing(ctx, sourceID, seenRelPaths)
}

func titleFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
