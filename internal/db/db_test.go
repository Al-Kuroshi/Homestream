package db_test

import (
	"testing"

	"personaltv/internal/db"
)

func TestOpen_CreatesSchema(t *testing.T) {
	conn := db.OpenTest(t)

	tables := []string{"media_sources", "media_items", "channels", "programs"}
	for _, table := range tables {
		var name string
		err := conn.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	// Opening the same database twice (simulating an app restart) must not
	// fail or re-apply migrations.
	path := t.TempDir() + "/restart.db"

	conn1, err := db.Open(path)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	conn1.Close()

	conn2, err := db.Open(path)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer conn2.Close()

	var name string
	if err := conn2.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'channels'`,
	).Scan(&name); err != nil {
		t.Errorf("expected channels table to survive reopen: %v", err)
	}
}
