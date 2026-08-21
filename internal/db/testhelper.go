package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// OpenTest opens a fresh, migrated SQLite database backed by a temp file
// (not :memory:, which behaves inconsistently under sql.DB's connection
// pooling unless shared-cache mode is used). The connection is closed
// automatically when the test ends.
func OpenTest(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
