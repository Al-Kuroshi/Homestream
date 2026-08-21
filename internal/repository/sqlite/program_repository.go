package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type ProgramRepository struct {
	db *sql.DB
}

func NewProgramRepository(conn *sql.DB) *ProgramRepository {
	return &ProgramRepository{db: conn}
}

func (r *ProgramRepository) Create(ctx context.Context, p *model.Program) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO programs (channel_id, media_item_id, start_time, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ChannelID, p.MediaItemID, db.FormatTime(p.StartTime), db.FormatTime(now), db.FormatTime(now))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = id
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (r *ProgramRepository) Get(ctx context.Context, id int64) (*model.Program, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, channel_id, media_item_id, start_time, created_at, updated_at FROM programs WHERE id = ?`, id)
	return scanProgram(row.Scan)
}

func (r *ProgramRepository) ListByChannel(ctx context.Context, channelID int64) ([]*model.Program, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, media_item_id, start_time, created_at, updated_at FROM programs WHERE channel_id = ? ORDER BY start_time`,
		channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialized (not nil) so an empty result set marshals to a JSON [] and
	// not null, which would break any client that iterates the response.
	programs := make([]*model.Program, 0)
	for rows.Next() {
		p, err := scanProgram(rows.Scan)
		if err != nil {
			return nil, err
		}
		programs = append(programs, p)
	}
	return programs, rows.Err()
}

func (r *ProgramRepository) Update(ctx context.Context, p *model.Program) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE programs SET channel_id = ?, media_item_id = ?, start_time = ?, updated_at = ? WHERE id = ?`,
		p.ChannelID, p.MediaItemID, db.FormatTime(p.StartTime), db.FormatTime(now), p.ID)
	if err != nil {
		return err
	}
	p.UpdatedAt = now
	return nil
}

func (r *ProgramRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM programs WHERE id = ?`, id)
	return err
}

func scanProgram(scan scanRow) (*model.Program, error) {
	var p model.Program
	var startTime, createdAt, updatedAt string
	err := scan(&p.ID, &p.ChannelID, &p.MediaItemID, &startTime, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if p.StartTime, err = db.ParseTime(startTime); err != nil {
		return nil, err
	}
	if p.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
