package model

import "time"

type MediaSource struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

type MediaItem struct {
	ID          int64
	SourceID    int64
	RelPath     string
	Title       string
	DurationSec float64
	VideoCodec  string
	AudioCodec  string
	Container   string
	SizeBytes   int64
	ModTime     time.Time
	Invalid     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Channel struct {
	ID          int64
	Name        string
	Description string
	Enabled     bool
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Program struct {
	ID          int64
	ChannelID   int64
	MediaItemID int64
	StartTime   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
