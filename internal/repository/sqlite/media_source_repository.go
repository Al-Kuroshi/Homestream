package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type MediaSourceRepository struct {
	db *sql.DB
}

func NewMediaSourceRepository(conn *sql.DB) *MediaSourceRepository {
	return &MediaSourceRepository{db: conn}
}

func (r *MediaSourceRepository) Create(ctx context.Context, s *model.MediaSource) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO media_sources (name, path, created_at) VALUES (?, ?, ?)`,
		s.Name, s.Path, db.FormatTime(now))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	s.CreatedAt = now
	return nil
}

func (r *MediaSourceRepository) Get(ctx context.Context, id int64) (*model.MediaSource, error) {
	var s model.MediaSource
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, path, created_at FROM media_sources WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Path, &createdAt)
	if err != nil {
		return nil, err
	}
	if s.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *MediaSourceRepository) List(ctx context.Context) ([]*model.MediaSource, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, path, created_at FROM media_sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialized (not nil) so an empty result set marshals to a JSON [] and
	// not null, which would break any client that iterates the response.
	sources := make([]*model.MediaSource, 0)
	for rows.Next() {
		var s model.MediaSource
		var createdAt string
		if err := rows.Scan(&s.ID, &s.Name, &s.Path, &createdAt); err != nil {
			return nil, err
		}
		if s.CreatedAt, err = db.ParseTime(createdAt); err != nil {
			return nil, err
		}
		sources = append(sources, &s)
	}
	return sources, rows.Err()
}

func (r *MediaSourceRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_sources WHERE id = ?`, id)
	return err
}
