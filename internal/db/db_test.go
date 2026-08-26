package db_test

import (
	"context"
	"database/sql"
	"testing"

	"personaltv/internal/db"
)

func TestOpen_CreatesSchema(t *testing.T) {
	conn := db.OpenTest(t)

	tables := []string{"media_sources", "media_items", "channels", "slots"}
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

// TestOpen_EnforcesForeignKeysOnEveryPooledConnection guards against the
// pragma regressing back to a single post-open `PRAGMA foreign_keys = ON`
// Exec, which is connection-scoped and therefore only configures whichever
// pooled connection happens to serve it. Several connections are checked
// out at once (forcing the pool to open distinct physical connections) and
// each is required to reject a row whose foreign key has no parent.
func TestOpen_EnforcesForeignKeysOnEveryPooledConnection(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	const numConns = 4
	conns := make([]*sql.Conn, 0, numConns)
	for i := 0; i < numConns; i++ {
		c, err := conn.Conn(ctx)
		if err != nil {
			t.Fatalf("failed to check out connection %d: %v", i, err)
		}
		defer c.Close()
		conns = append(conns, c)
	}

	const ts = "2026-01-01T00:00:00Z"
	for i, c := range conns {
		var fk int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatalf("connection %d: reading foreign_keys pragma: %v", i, err)
		}
		if fk != 1 {
			t.Errorf("connection %d: expected foreign_keys = 1, got %d", i, fk)
		}

		_, err := c.ExecContext(ctx,
			`INSERT INTO media_items (source_id, rel_path, title, mod_time, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			99999, "orphan.mp4", "orphan", ts, ts, ts)
		if err == nil {
			t.Errorf("connection %d: expected a foreign key violation inserting a media_item "+
				"referencing a nonexistent media_source, got nil", i)
		}
	}
}

// TestOpen_CascadeDeleteIsEnforced is the positive counterpart: with foreign
// keys actually on, deleting a parent row removes its children.
func TestOpen_CascadeDeleteIsEnforced(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	const ts = "2026-01-01T00:00:00Z"
	res, err := conn.ExecContext(ctx,
		`INSERT INTO media_sources (name, path, created_at) VALUES (?, ?, ?)`, "Movies", "/media/movies", ts)
	if err != nil {
		t.Fatalf("failed to insert media source: %v", err)
	}
	sourceID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to read source id: %v", err)
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO media_items (source_id, rel_path, title, mod_time, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sourceID, "a.mp4", "a", ts, ts, ts); err != nil {
		t.Fatalf("failed to insert media item: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `DELETE FROM media_sources WHERE id = ?`, sourceID); err != nil {
		t.Fatalf("failed to delete media source: %v", err)
	}

	var remaining int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items WHERE source_id = ?`, sourceID).Scan(&remaining); err != nil {
		t.Fatalf("failed to count media items: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected ON DELETE CASCADE to remove child media_items, %d remain", remaining)
	}
}
