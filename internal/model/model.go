package model

import "time"

// These structs are marshalled straight to the wire by the API handlers, so
// their json tags are the public response contract. They use the same
// snake_case names as the request bodies and the DB columns, rather than
// Go's default capitalized field names, so a client sees one consistent
// casing across every request and response.

type MediaSource struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type MediaItem struct {
	ID          int64     `json:"id"`
	SourceID    int64     `json:"source_id"`
	RelPath     string    `json:"rel_path"`
	Title       string    `json:"title"`
	DurationSec float64   `json:"duration_sec"`
	VideoCodec  string    `json:"video_codec"`
	AudioCodec  string    `json:"audio_codec"`
	Container   string    `json:"container"`
	SizeBytes   int64     `json:"size_bytes"`
	ModTime     time.Time `json:"mod_time"`
	Invalid     bool      `json:"invalid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Channel struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	SlotKindMedia = "media"
	SlotKindGap   = "gap"
)

type Slot struct {
	ID             int64      `json:"id"`
	ChannelID      int64      `json:"channel_id"`
	Kind           string     `json:"kind"`
	MediaItemID    *int64     `json:"media_item_id,omitempty"`
	GapDurationSec *float64   `json:"gap_duration_sec,omitempty"`
	GapLabel       string     `json:"gap_label"`
	Recurring      bool       `json:"recurring"`
	DayOfWeek      *int       `json:"day_of_week,omitempty"`
	Position       *int       `json:"position,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
