package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type ChannelRepository struct {
	db *sql.DB
}

func NewChannelRepository(conn *sql.DB) *ChannelRepository {
	return &ChannelRepository{db: conn}
}

func (r *ChannelRepository) Create(ctx context.Context, c *model.Channel) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO channels (name, description, enabled, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Name, c.Description, c.Enabled, c.Position, db.FormatTime(now), db.FormatTime(now))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (r *ChannelRepository) Get(ctx context.Context, id int64) (*model.Channel, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, enabled, position, created_at, updated_at FROM channels WHERE id = ?`, id)
	return scanChannel(row.Scan)
}

func (r *ChannelRepository) List(ctx context.Context) ([]*model.Channel, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, enabled, position, created_at, updated_at FROM channels ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialized (not nil) so an empty result set marshals to a JSON [] and
	// not null, which would break any client that iterates the response.
	channels := make([]*model.Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows.Scan)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

func (r *ChannelRepository) Update(ctx context.Context, c *model.Channel) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE channels SET name = ?, description = ?, enabled = ?, position = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Description, c.Enabled, c.Position, db.FormatTime(now), c.ID)
	if err != nil {
		return err
	}
	c.UpdatedAt = now
	return nil
}

func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	return err
}

func scanChannel(scan scanRow) (*model.Channel, error) {
	var c model.Channel
	var createdAt, updatedAt string
	err := scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Position, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if c.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
