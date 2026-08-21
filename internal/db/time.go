package db

import "time"

// FormatTime and ParseTime are the only sanctioned way to move a time.Time
// into/out of a TEXT column. We do not rely on driver-specific automatic
// time.Time scanning, which varies across SQLite drivers.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ParseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
