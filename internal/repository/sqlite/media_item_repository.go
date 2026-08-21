package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(conn *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: conn}
}

const mediaItemColumns = `id, source_id, rel_path, title, duration_sec, video_codec, audio_codec, container, size_bytes, mod_time, invalid, created_at, updated_at`

func (r *MediaItemRepository) Upsert(ctx context.Context, m *model.MediaItem) error {
	now := time.Now().UTC()

	var existingID int64
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, created_at FROM media_items WHERE source_id = ? AND rel_path = ?`,
		m.SourceID, m.RelPath,
	).Scan(&existingID, &createdAt)

	switch {
	case err == sql.ErrNoRows:
		res, insErr := r.db.ExecContext(ctx, `
			INSERT INTO media_items
				(source_id, rel_path, title, duration_sec, video_codec, audio_codec, container, size_bytes, mod_time, invalid, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.SourceID, m.RelPath, m.Title, m.DurationSec, m.VideoCodec, m.AudioCodec, m.Container,
			m.SizeBytes, db.FormatTime(m.ModTime), m.Invalid, db.FormatTime(now), db.FormatTime(now))
		if insErr != nil {
			return insErr
		}
		id, idErr := res.LastInsertId()
		if idErr != nil {
			return idErr
		}
		m.ID = id
		m.CreatedAt = now
		m.UpdatedAt = now
		return nil
	case err != nil:
		return err
	default:
		if _, updErr := r.db.ExecContext(ctx, `
			UPDATE media_items SET
				title = ?, duration_sec = ?, video_codec = ?, audio_codec = ?, container = ?,
				size_bytes = ?, mod_time = ?, invalid = ?, updated_at = ?
			WHERE id = ?`,
			m.Title, m.DurationSec, m.VideoCodec, m.AudioCodec, m.Container,
			m.SizeBytes, db.FormatTime(m.ModTime), m.Invalid, db.FormatTime(now), existingID); updErr != nil {
			return updErr
		}
		m.ID = existingID
		if m.CreatedAt, err = db.ParseTime(createdAt); err != nil {
			return err
		}
		m.UpdatedAt = now
		return nil
	}
}

func (r *MediaItemRepository) Get(ctx context.Context, id int64) (*model.MediaItem, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, id)
	return scanMediaItem(row.Scan)
}

func (r *MediaItemRepository) ListBySource(ctx context.Context, sourceID int64) ([]*model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items WHERE source_id = ? ORDER BY rel_path`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

func (r *MediaItemRepository) List(ctx context.Context) ([]*model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

func (r *MediaItemRepository) DeleteBySource(ctx context.Context, sourceID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE source_id = ?`, sourceID)
	return err
}

func (r *MediaItemRepository) DeleteMissing(ctx context.Context, sourceID int64, keepRelPaths []string) error {
	keep := make(map[string]bool, len(keepRelPaths))
	for _, p := range keepRelPaths {
		keep[p] = true
	}

	existing, err := r.ListBySource(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, item := range existing {
		if !keep[item.RelPath] {
			if _, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, item.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// scanRow is satisfied by both *sql.Row.Scan and *sql.Rows.Scan.
type scanRow func(dest ...any) error

func scanMediaItem(scan scanRow) (*model.MediaItem, error) {
	var m model.MediaItem
	var modTime, createdAt, updatedAt string
	err := scan(&m.ID, &m.SourceID, &m.RelPath, &m.Title, &m.DurationSec, &m.VideoCodec, &m.AudioCodec,
		&m.Container, &m.SizeBytes, &modTime, &m.Invalid, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if m.ModTime, err = db.ParseTime(modTime); err != nil {
		return nil, err
	}
	if m.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if m.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func scanMediaItems(rows *sql.Rows) ([]*model.MediaItem, error) {
	var items []*model.MediaItem
	for rows.Next() {
		m, err := scanMediaItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}
