package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type SlotRepository struct {
	db *sql.DB
}

func NewSlotRepository(conn *sql.DB) *SlotRepository {
	return &SlotRepository{db: conn}
}

func (r *SlotRepository) Create(ctx context.Context, s *model.Slot) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO slots (channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ChannelID, s.Kind, nullableInt64(s.MediaItemID), nullableFloat64(s.GapDurationSec), s.GapLabel,
		s.Recurring, nullableIntAsInt64(s.DayOfWeek), nullableIntAsInt64(s.Position), nullableTime(s.StartTime),
		db.FormatTime(now), db.FormatTime(now))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (r *SlotRepository) Get(ctx context.Context, id int64) (*model.Slot, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at
		 FROM slots WHERE id = ?`, id)
	return scanSlot(row.Scan)
}

func (r *SlotRepository) ListByChannel(ctx context.Context, channelID int64) ([]*model.Slot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, kind, media_item_id, gap_duration_sec, gap_label, recurring, day_of_week, position, start_time, created_at, updated_at
		 FROM slots WHERE channel_id = ?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]*model.Slot, 0)
	for rows.Next() {
		s, err := scanSlot(rows.Scan)
		if err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

func (r *SlotRepository) Update(ctx context.Context, s *model.Slot) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE slots SET channel_id = ?, kind = ?, media_item_id = ?, gap_duration_sec = ?, gap_label = ?,
		 recurring = ?, day_of_week = ?, position = ?, start_time = ?, updated_at = ? WHERE id = ?`,
		s.ChannelID, s.Kind, nullableInt64(s.MediaItemID), nullableFloat64(s.GapDurationSec), s.GapLabel,
		s.Recurring, nullableIntAsInt64(s.DayOfWeek), nullableIntAsInt64(s.Position), nullableTime(s.StartTime),
		db.FormatTime(now), s.ID)
	if err != nil {
		return err
	}
	s.UpdatedAt = now
	return nil
}

func (r *SlotRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM slots WHERE id = ?`, id)
	return err
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableIntAsInt64(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

func nullableFloat64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return db.FormatTime(*v)
}

func scanSlot(scan scanRow) (*model.Slot, error) {
	var s model.Slot
	var mediaItemID sql.NullInt64
	var gapDurationSec sql.NullFloat64
	var dayOfWeek sql.NullInt64
	var position sql.NullInt64
	var startTime sql.NullString
	var createdAt, updatedAt string

	err := scan(&s.ID, &s.ChannelID, &s.Kind, &mediaItemID, &gapDurationSec, &s.GapLabel,
		&s.Recurring, &dayOfWeek, &position, &startTime, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if mediaItemID.Valid {
		v := mediaItemID.Int64
		s.MediaItemID = &v
	}
	if gapDurationSec.Valid {
		v := gapDurationSec.Float64
		s.GapDurationSec = &v
	}
	if dayOfWeek.Valid {
		v := int(dayOfWeek.Int64)
		s.DayOfWeek = &v
	}
	if position.Valid {
		v := int(position.Int64)
		s.Position = &v
	}
	if startTime.Valid {
		t, err := db.ParseTime(startTime.String)
		if err != nil {
			return nil, err
		}
		s.StartTime = &t
	}
	if s.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}
