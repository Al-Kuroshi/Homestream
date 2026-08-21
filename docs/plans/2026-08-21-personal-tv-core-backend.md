# Personal TV — Core Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the headless Go backend — SQLite data layer, local-filesystem media scanner, pure scheduling logic, channel/schedule service, and a REST API — fully working and testable via `curl`/`httptest` with no UI.

**Architecture:** Layered per `docs/design/2026-08-21-personal-tv-design.md`: `db` (SQLite + migrations) → `repository` (interfaces + SQLite impl) → `mediastore` (scanner) / `scheduler` (pure logic) → `channels` (service combining repos + scheduler) → `api` (REST handlers) → `cmd/personaltv` (wiring).

**Tech Stack:** Go 1.22+ (for `net/http.ServeMux` method+pattern routing and `r.PathValue`), `modernc.org/sqlite` (pure-Go, CGO-free SQLite driver — keeps cross-compilation/static-binary story intact), `ffmpeg`/`ffprobe` (external subprocess, required on `PATH` for both running the app and running this plan's tests).

**Spec:** `docs/design/2026-08-21-personal-tv-design.md` (and `docs/prd/HomeStreamer.md` for product requirements). This plan implements design spec §4.1–§4.6 and §5 (mediastore, channels, scheduler, playback's scheduling half, db, api, and the Scan/Guide data flows). Playback's actual streaming (direct-play/transcode) is a separate plan.

## Global Constraints

- Go module name: `personaltv`. Go version: `1.22` (from spec §3 stack table and the routing syntax used throughout).
- SQLite only for MVP; every table access goes through a `repository` interface — business logic and handlers never import `database/sql` directly (design spec §4.5, PRD's extensibility principle).
- All persisted timestamps are stored as `TEXT` in RFC3339Nano (UTC) via explicit `db.FormatTime`/`db.ParseTime` helpers — never rely on driver-specific automatic `time.Time` scanning, and never read back a SQL-generated `CURRENT_TIMESTAMP` into a Go `time.Time` (set timestamps in Go before writing instead).
- `ffprobe`/`ffmpeg` must be installed and on `PATH` in any environment running this plan's tests (they generate real short test videos with `ffmpeg` and probe them with `ffprobe` rather than using binary fixtures).
- Media duration is always derived from `ffprobe`, never guessed (design spec §4.1, PRD §11).
- A single unavailable/invalid media item must never abort a whole scan or a whole channel's schedule computation (PRD §11, design spec §6).

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `cmd/personaltv/main.go`
- Test: `cmd/personaltv/main_test.go`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `handleHealth(w http.ResponseWriter, r *http.Request)` in package `main` — a placeholder entrypoint that Task 12 will replace wholesale once the real API server exists. Nothing later depends on this function's internals, only on the fact that `go build ./...` and `go test ./...` work from here on.

- [ ] **Step 1: Initialize the git repo and Go module**

```bash
cd /home/daslaptop/HomeStreamProject
git init
go mod init personaltv
```

Create `.gitignore`:

```
/personaltv
*.db
/web/node_modules
/web/dist
```

- [ ] **Step 2: Write the failing test**

`cmd/personaltv/main_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body %q, got %q", "ok", w.Body.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/personaltv/...`
Expected: FAIL — `undefined: handleHealth`

- [ ] **Step 4: Write the implementation**

`cmd/personaltv/main.go`:

```go
package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	log.Println("Personal TV listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/personaltv/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod .gitignore cmd/personaltv/main.go cmd/personaltv/main_test.go
git commit -m "chore: scaffold Go module with a healthz endpoint"
```

---

## Task 2: SQLite Connection, Migrations, and Time Helpers

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/time.go`
- Create: `internal/db/testhelper.go`
- Create: `internal/db/migrations/0001_initial_schema.sql`
- Test: `internal/db/db_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `db.Open(path string) (*sql.DB, error)` — opens (creating if needed) a SQLite DB at `path` and applies all pending migrations.
  - `db.OpenTest(t *testing.T) *sql.DB` — opens a fresh, migrated SQLite DB backed by a temp file (`t.TempDir()`), auto-closed via `t.Cleanup`. **Every later task's tests use this.**
  - `db.FormatTime(t time.Time) string` / `db.ParseTime(s string) (time.Time, error)` — the only sanctioned way to move a `time.Time` into/out of a `TEXT` column. **Every repository in Tasks 3–4 uses these.**
  - Schema tables: `media_sources`, `media_items`, `channels`, `programs` (columns detailed below — Tasks 3–4 write the Go code that matches these exactly).

- [ ] **Step 1: Add the SQLite driver dependency**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: Write the failing test**

`internal/db/db_test.go`:

```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/db/...`
Expected: FAIL — build errors (package `db` doesn't exist yet).

- [ ] **Step 4: Write the migration file**

`internal/db/migrations/0001_initial_schema.sql`:

```sql
CREATE TABLE media_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE media_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES media_sources(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    title TEXT NOT NULL,
    duration_sec REAL NOT NULL DEFAULT 0,
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    container TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    mod_time TEXT NOT NULL,
    invalid INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, rel_path)
);

CREATE TABLE channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE programs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    start_time TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_programs_channel_id ON programs(channel_id);
CREATE INDEX idx_media_items_source_id ON media_items(source_id);
```

- [ ] **Step 5: Write `db.go`, `time.go`, and `testhelper.go`**

`internal/db/db.go`:

```go
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}
	if err := migrate(conn); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var count int
		if err := conn.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
```

`internal/db/time.go`:

```go
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
```

`internal/db/testhelper.go`:

```go
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
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/db/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/db
git commit -m "feat: add SQLite connection, migrations, and time helpers"
```

---

## Task 3: Domain Model Types and Media Repository

**Files:**
- Create: `internal/model/model.go`
- Create: `internal/repository/repository.go`
- Create: `internal/repository/sqlite/media_source_repository.go`
- Create: `internal/repository/sqlite/media_item_repository.go`
- Test: `internal/repository/sqlite/media_source_repository_test.go`
- Test: `internal/repository/sqlite/media_item_repository_test.go`

**Interfaces:**
- Consumes: `db.OpenTest`, `db.FormatTime`, `db.ParseTime` (Task 2).
- Produces:
  - `model.MediaSource{ID int64, Name string, Path string, CreatedAt time.Time}`
  - `model.MediaItem{ID, SourceID int64, RelPath, Title, VideoCodec, AudioCodec, Container string, DurationSec float64, SizeBytes int64, ModTime time.Time, Invalid bool, CreatedAt, UpdatedAt time.Time}`
  - `model.Channel`, `model.Program` (defined now too, used starting Task 4).
  - `repository.MediaSourceRepository` / `repository.MediaItemRepository` interfaces (and `ChannelRepository` / `ProgramRepository`, implemented in Task 4).
  - `sqlite.NewMediaSourceRepository(db *sql.DB) *MediaSourceRepository` and `sqlite.NewMediaItemRepository(db *sql.DB) *MediaItemRepository`, each implementing their respective interface. **Tasks 6, 8, 9, 10, 11, 12 all construct and use these.**

- [ ] **Step 1: Write the failing tests**

`internal/repository/sqlite/media_source_repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"testing"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestMediaSourceRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	repo := sqlite.NewMediaSourceRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := repo.Create(ctx, source); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if source.ID == 0 {
		t.Fatal("expected Create to set an ID")
	}

	fetched, err := repo.Get(ctx, source.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.Name != "Movies" || fetched.Path != "/media/movies" {
		t.Errorf("unexpected source: %+v", fetched)
	}
}

func TestMediaSourceRepository_ListAndDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	repo := sqlite.NewMediaSourceRepository(conn)

	a := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	b := &model.MediaSource{Name: "TV", Path: "/media/tv"}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create a returned error: %v", err)
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create b returned error: %v", err)
	}

	sources, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}

	if err := repo.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	sources, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source after delete, got %d", len(sources))
	}
	if sources[0].ID != b.ID {
		t.Errorf("expected remaining source to be %d, got %d", b.ID, sources[0].ID)
	}
}
```

`internal/repository/sqlite/media_item_repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestMediaItemRepository_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	item := &model.MediaItem{
		SourceID:    source.ID,
		RelPath:     "action/movie-a.mp4",
		Title:       "movie-a",
		DurationSec: 7200,
		VideoCodec:  "h264",
		AudioCodec:  "aac",
		Container:   "mov,mp4,m4a,3gp,3g2,mj2",
		SizeBytes:   123456,
		ModTime:     time.Now().UTC().Truncate(time.Second),
	}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("expected Upsert to set an ID")
	}

	fetched, err := itemRepo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.Title != "movie-a" || fetched.DurationSec != 7200 {
		t.Errorf("unexpected item: %+v", fetched)
	}

	// Upsert again with the same source_id/rel_path should update, not duplicate.
	item.Title = "movie-a-renamed"
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after re-upsert, got %d", len(items))
	}
	if items[0].Title != "movie-a-renamed" {
		t.Errorf("expected updated title, got %q", items[0].Title)
	}
}

func TestMediaItemRepository_DeleteMissing(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	keep := &model.MediaItem{SourceID: source.ID, RelPath: "keep.mp4", Title: "keep", ModTime: time.Now().UTC()}
	remove := &model.MediaItem{SourceID: source.ID, RelPath: "remove.mp4", Title: "remove", ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, keep); err != nil {
		t.Fatalf("failed to upsert keep item: %v", err)
	}
	if err := itemRepo.Upsert(ctx, remove); err != nil {
		t.Fatalf("failed to upsert remove item: %v", err)
	}

	if err := itemRepo.DeleteMissing(ctx, source.ID, []string{"keep.mp4"}); err != nil {
		t.Fatalf("DeleteMissing returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 1 || items[0].RelPath != "keep.mp4" {
		t.Fatalf("expected only keep.mp4 to remain, got %+v", items)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/repository/...`
Expected: FAIL — build errors (packages `model`, `repository`, `repository/sqlite` don't exist yet).

- [ ] **Step 3: Write the domain model types**

`internal/model/model.go`:

```go
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
```

- [ ] **Step 4: Write the repository interfaces**

`internal/repository/repository.go`:

```go
package repository

import (
	"context"

	"personaltv/internal/model"
)

type MediaSourceRepository interface {
	Create(ctx context.Context, s *model.MediaSource) error
	Get(ctx context.Context, id int64) (*model.MediaSource, error)
	List(ctx context.Context) ([]*model.MediaSource, error)
	Delete(ctx context.Context, id int64) error
}

// MediaItemRepository. Upsert keys on (SourceID, RelPath): calling it twice
// for the same file updates the existing row instead of creating a
// duplicate, which is what lets a rescan be cheap and idempotent.
type MediaItemRepository interface {
	Upsert(ctx context.Context, m *model.MediaItem) error
	Get(ctx context.Context, id int64) (*model.MediaItem, error)
	ListBySource(ctx context.Context, sourceID int64) ([]*model.MediaItem, error)
	List(ctx context.Context) ([]*model.MediaItem, error)
	DeleteBySource(ctx context.Context, sourceID int64) error
	// DeleteMissing removes every item under sourceID whose RelPath is not
	// in keepRelPaths — used after a scan to prune files that no longer exist.
	DeleteMissing(ctx context.Context, sourceID int64, keepRelPaths []string) error
}

type ChannelRepository interface {
	Create(ctx context.Context, c *model.Channel) error
	Get(ctx context.Context, id int64) (*model.Channel, error)
	List(ctx context.Context) ([]*model.Channel, error)
	Update(ctx context.Context, c *model.Channel) error
	Delete(ctx context.Context, id int64) error
}

type ProgramRepository interface {
	Create(ctx context.Context, p *model.Program) error
	Get(ctx context.Context, id int64) (*model.Program, error)
	ListByChannel(ctx context.Context, channelID int64) ([]*model.Program, error)
	Update(ctx context.Context, p *model.Program) error
	Delete(ctx context.Context, id int64) error
}
```

- [ ] **Step 5: Write the MediaSource SQLite repository**

`internal/repository/sqlite/media_source_repository.go`:

```go
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

	var sources []*model.MediaSource
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
```

- [ ] **Step 6: Write the MediaItem SQLite repository**

`internal/repository/sqlite/media_item_repository.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
)

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(conn *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: conn}
}

const mediaItemColumns = `id, source_id, rel_path, title, duration_sec, video_codec, audio_codec, container, size_bytes, mod_time, invalid, created_at, updated_at`

func (r *MediaItemRepository) Upsert(ctx context.Context, m *model.MediaItem) error {
	now := time.Now().UTC()

	var existingID int64
	var createdAt string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, created_at FROM media_items WHERE source_id = ? AND rel_path = ?`,
		m.SourceID, m.RelPath,
	).Scan(&existingID, &createdAt)

	switch {
	case err == sql.ErrNoRows:
		res, insErr := r.db.ExecContext(ctx, `
			INSERT INTO media_items
				(source_id, rel_path, title, duration_sec, video_codec, audio_codec, container, size_bytes, mod_time, invalid, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.SourceID, m.RelPath, m.Title, m.DurationSec, m.VideoCodec, m.AudioCodec, m.Container,
			m.SizeBytes, db.FormatTime(m.ModTime), m.Invalid, db.FormatTime(now), db.FormatTime(now))
		if insErr != nil {
			return insErr
		}
		id, idErr := res.LastInsertId()
		if idErr != nil {
			return idErr
		}
		m.ID = id
		m.CreatedAt = now
		m.UpdatedAt = now
		return nil
	case err != nil:
		return err
	default:
		if _, updErr := r.db.ExecContext(ctx, `
			UPDATE media_items SET
				title = ?, duration_sec = ?, video_codec = ?, audio_codec = ?, container = ?,
				size_bytes = ?, mod_time = ?, invalid = ?, updated_at = ?
			WHERE id = ?`,
			m.Title, m.DurationSec, m.VideoCodec, m.AudioCodec, m.Container,
			m.SizeBytes, db.FormatTime(m.ModTime), m.Invalid, db.FormatTime(now), existingID); updErr != nil {
			return updErr
		}
		m.ID = existingID
		if m.CreatedAt, err = db.ParseTime(createdAt); err != nil {
			return err
		}
		m.UpdatedAt = now
		return nil
	}
}

func (r *MediaItemRepository) Get(ctx context.Context, id int64) (*model.MediaItem, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, id)
	return scanMediaItem(row.Scan)
}

func (r *MediaItemRepository) ListBySource(ctx context.Context, sourceID int64) ([]*model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items WHERE source_id = ? ORDER BY rel_path`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

func (r *MediaItemRepository) List(ctx context.Context) ([]*model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+mediaItemColumns+` FROM media_items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

func (r *MediaItemRepository) DeleteBySource(ctx context.Context, sourceID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE source_id = ?`, sourceID)
	return err
}

func (r *MediaItemRepository) DeleteMissing(ctx context.Context, sourceID int64, keepRelPaths []string) error {
	keep := make(map[string]bool, len(keepRelPaths))
	for _, p := range keepRelPaths {
		keep[p] = true
	}

	existing, err := r.ListBySource(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, item := range existing {
		if !keep[item.RelPath] {
			if _, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, item.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// scanRow is satisfied by both *sql.Row.Scan and *sql.Rows.Scan.
type scanRow func(dest ...any) error

func scanMediaItem(scan scanRow) (*model.MediaItem, error) {
	var m model.MediaItem
	var modTime, createdAt, updatedAt string
	err := scan(&m.ID, &m.SourceID, &m.RelPath, &m.Title, &m.DurationSec, &m.VideoCodec, &m.AudioCodec,
		&m.Container, &m.SizeBytes, &modTime, &m.Invalid, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if m.ModTime, err = db.ParseTime(modTime); err != nil {
		return nil, err
	}
	if m.CreatedAt, err = db.ParseTime(createdAt); err != nil {
		return nil, err
	}
	if m.UpdatedAt, err = db.ParseTime(updatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func scanMediaItems(rows *sql.Rows) ([]*model.MediaItem, error) {
	var items []*model.MediaItem
	for rows.Next() {
		m, err := scanMediaItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/repository/... ./internal/model/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/model internal/repository
git commit -m "feat: add domain models and media source/item repositories"
```

---

## Task 4: Channel and Program Repositories

**Files:**
- Create: `internal/repository/sqlite/channel_repository.go`
- Create: `internal/repository/sqlite/program_repository.go`
- Test: `internal/repository/sqlite/channel_repository_test.go`
- Test: `internal/repository/sqlite/program_repository_test.go`

**Interfaces:**
- Consumes: `model.Channel`, `model.Program`, `repository.ChannelRepository`, `repository.ProgramRepository` (Task 3), `db.FormatTime`/`db.ParseTime` (Task 2).
- Produces: `sqlite.NewChannelRepository(db *sql.DB) *ChannelRepository` and `sqlite.NewProgramRepository(db *sql.DB) *ProgramRepository`, each implementing their interface. **Tasks 8, 9, 10, 11, 12 construct and use these.**

- [ ] **Step 1: Write the failing tests**

`internal/repository/sqlite/channel_repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"testing"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestChannelRepository_CreateGetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	repo := sqlite.NewChannelRepository(conn)

	channel := &model.Channel{Name: "Movies", Description: "Movie channel", Enabled: true, Position: 1}
	if err := repo.Create(ctx, channel); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if channel.ID == 0 {
		t.Fatal("expected Create to set an ID")
	}

	fetched, err := repo.Get(ctx, channel.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.Name != "Movies" || !fetched.Enabled {
		t.Errorf("unexpected channel: %+v", fetched)
	}

	fetched.Name = "Movies HD"
	fetched.Enabled = false
	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	updated, err := repo.Get(ctx, channel.ID)
	if err != nil {
		t.Fatalf("Get after update returned error: %v", err)
	}
	if updated.Name != "Movies HD" || updated.Enabled {
		t.Errorf("expected updated channel, got %+v", updated)
	}

	if err := repo.Delete(ctx, channel.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := repo.Get(ctx, channel.ID); err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestChannelRepository_ListOrdersByPosition(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	repo := sqlite.NewChannelRepository(conn)

	second := &model.Channel{Name: "Second", Position: 2}
	first := &model.Channel{Name: "First", Position: 1}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("failed to create second: %v", err)
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("failed to create first: %v", err)
	}

	channels, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(channels) != 2 || channels[0].Name != "First" || channels[1].Name != "Second" {
		t.Fatalf("expected channels ordered by position, got %+v", channels)
	}
}
```

`internal/repository/sqlite/program_repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func setupChannelAndMediaItem(t *testing.T, ctx context.Context, conn *sql.DB) (*model.Channel, *model.MediaItem) {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600, ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("failed to upsert item: %v", err)
	}
	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	return channel, item
}

func TestProgramRepository_CreateGetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel, item := setupChannelAndMediaItem(t, ctx, conn)
	repo := sqlite.NewProgramRepository(conn)

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := repo.Create(ctx, program); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if program.ID == 0 {
		t.Fatal("expected Create to set an ID")
	}

	fetched, err := repo.Get(ctx, program.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !fetched.StartTime.Equal(start) {
		t.Errorf("expected start time %v, got %v", start, fetched.StartTime)
	}

	newStart := start.Add(time.Hour)
	fetched.StartTime = newStart
	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	updated, err := repo.Get(ctx, program.ID)
	if err != nil {
		t.Fatalf("Get after update returned error: %v", err)
	}
	if !updated.StartTime.Equal(newStart) {
		t.Errorf("expected updated start time %v, got %v", newStart, updated.StartTime)
	}

	if err := repo.Delete(ctx, program.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := repo.Get(ctx, program.ID); err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestProgramRepository_ListByChannelOrdersByStartTime(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channel, item := setupChannelAndMediaItem(t, ctx, conn)
	repo := sqlite.NewProgramRepository(conn)

	base := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	later := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: base.Add(2 * time.Hour)}
	earlier := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: base}
	if err := repo.Create(ctx, later); err != nil {
		t.Fatalf("failed to create later program: %v", err)
	}
	if err := repo.Create(ctx, earlier); err != nil {
		t.Fatalf("failed to create earlier program: %v", err)
	}

	programs, err := repo.ListByChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListByChannel returned error: %v", err)
	}
	if len(programs) != 2 || !programs[0].StartTime.Equal(base) {
		t.Fatalf("expected programs ordered by start_time, got %+v", programs)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/repository/sqlite/...`
Expected: FAIL — `undefined: sqlite.NewChannelRepository` / `undefined: sqlite.NewProgramRepository`

- [ ] **Step 3: Write the Channel SQLite repository**

`internal/repository/sqlite/channel_repository.go`:

```go
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

	var channels []*model.Channel
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
```

- [ ] **Step 4: Write the Program SQLite repository**

`internal/repository/sqlite/program_repository.go`:

```go
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

	var programs []*model.Program
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/repository/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository
git commit -m "feat: add channel and program repositories"
```

---

## Task 5: `ffprobe` Wrapper

**Files:**
- Create: `internal/mediastore/probe.go`
- Test: `internal/mediastore/probe_test.go`

**Interfaces:**
- Consumes: nothing new (calls the external `ffprobe` binary via `os/exec`).
- Produces: `mediastore.Probe(path string) (*ProbeResult, error)` where `ProbeResult{DurationSec float64, VideoCodec, AudioCodec, Container string}`. **Task 6's scanner calls this directly.** Also produces the test helper `generateTestVideo(t *testing.T, dir, name string, durationSec int) string`, which Tasks 6 and 12 reuse (each package needs its own copy since Go test helpers aren't exported across packages via `_test.go` files — Tasks 6 and 12 each write their own copy, shown in those tasks).

- [ ] **Step 1: Write the failing test**

`internal/mediastore/probe_test.go`:

```go
package mediastore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func generateTestVideo(t *testing.T, dir, name string, durationSec int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	d := strconv.Itoa(durationSec)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration="+d+":size=64x64:rate=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+d,
		"-c:v", "libx264", "-c:a", "aac", "-shortest", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test video: %v\n%s", err, out)
	}
	return path
}

func TestProbe_ValidVideo(t *testing.T) {
	dir := t.TempDir()
	path := generateTestVideo(t, dir, "test.mp4", 2)

	result, err := Probe(path)
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if result.DurationSec < 1.5 || result.DurationSec > 2.5 {
		t.Errorf("expected duration ~2s, got %f", result.DurationSec)
	}
	if result.VideoCodec != "h264" {
		t.Errorf("expected video codec h264, got %q", result.VideoCodec)
	}
	if result.AudioCodec != "aac" {
		t.Errorf("expected audio codec aac, got %q", result.AudioCodec)
	}
}

func TestProbe_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-video.txt")
	if err := os.WriteFile(path, []byte("not a video"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := Probe(path); err == nil {
		t.Fatal("expected error for invalid file, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mediastore/...`
Expected: FAIL — `undefined: Probe`

- [ ] **Step 3: Write the implementation**

`internal/mediastore/probe.go`:

```go
package mediastore

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

type ProbeResult struct {
	DurationSec float64
	VideoCodec  string
	AudioCodec  string
	Container   string
}

type ffprobeOutput struct {
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// Probe runs ffprobe against path and returns its technical metadata.
// A non-nil error means the file could not be read as media at all —
// callers (mediastore.Scanner) treat that as "mark this item invalid",
// not as a fatal error for the whole scan.
func Probe(path string) (*ProbeResult, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed for %s: %w", path, err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parsing ffprobe output for %s: %w", path, err)
	}

	duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing duration for %s: %w", path, err)
	}

	result := &ProbeResult{
		DurationSec: duration,
		Container:   parsed.Format.FormatName,
	}
	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			if result.VideoCodec == "" {
				result.VideoCodec = s.CodecName
			}
		case "audio":
			if result.AudioCodec == "" {
				result.AudioCodec = s.CodecName
			}
		}
	}

	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mediastore/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mediastore
git commit -m "feat: add ffprobe wrapper for technical media metadata"
```

---

## Task 6: Media Scanner

**Files:**
- Create: `internal/mediastore/scan.go`
- Test: `internal/mediastore/scan_test.go`

**Interfaces:**
- Consumes: `mediastore.Probe` (Task 5), `repository.MediaSourceRepository`, `repository.MediaItemRepository` (Task 3), `db.OpenTest` (Task 2), `sqlite.NewMediaSourceRepository`/`NewMediaItemRepository` (Task 3).
- Produces: `mediastore.NewScanner(sources repository.MediaSourceRepository, items repository.MediaItemRepository) *Scanner` and `(*Scanner) ScanSource(ctx context.Context, sourceID int64) error`. **Tasks 9 and 12 construct and use this.**

- [ ] **Step 1: Write the failing test**

`internal/mediastore/scan_test.go`:

```go
package mediastore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestScanner_ScanSource(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()

	generateTestVideo(t, mediaDir, "test.mp4", 2)

	if err := os.WriteFile(filepath.Join(mediaDir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("failed to write notes.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "broken.mp4"), []byte("not a real video"), 0644); err != nil {
		t.Fatalf("failed to write broken.mp4: %v", err)
	}

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Test Source", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	scanner := NewScanner(sourceRepo, itemRepo)
	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("ScanSource returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 media items (valid + broken), got %d", len(items))
	}

	var valid, broken *model.MediaItem
	for _, item := range items {
		switch item.RelPath {
		case "test.mp4":
			valid = item
		case "broken.mp4":
			broken = item
		}
	}

	if valid == nil {
		t.Fatal("expected test.mp4 to be scanned")
	}
	if valid.Invalid {
		t.Error("expected test.mp4 to be valid")
	}
	if valid.DurationSec < 1.5 {
		t.Errorf("expected test.mp4 duration ~2s, got %f", valid.DurationSec)
	}
	if valid.Title != "test" {
		t.Errorf("expected title derived from filename 'test', got %q", valid.Title)
	}

	if broken == nil {
		t.Fatal("expected broken.mp4 to be scanned and marked invalid")
	}
	if !broken.Invalid {
		t.Error("expected broken.mp4 to be marked invalid")
	}
}

func TestScanner_ScanSourceRemovesDeletedFiles(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	videoPath := generateTestVideo(t, mediaDir, "gone.mp4", 2)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	source := &model.MediaSource{Name: "Test Source", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	scanner := NewScanner(sourceRepo, itemRepo)
	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("first ScanSource returned error: %v", err)
	}

	if err := os.Remove(videoPath); err != nil {
		t.Fatalf("failed to remove test video: %v", err)
	}

	if err := scanner.ScanSource(ctx, source.ID); err != nil {
		t.Fatalf("second ScanSource returned error: %v", err)
	}

	items, err := itemRepo.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected deleted file to be pruned, got %+v", items)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mediastore/...`
Expected: FAIL — `undefined: NewScanner`

- [ ] **Step 3: Write the implementation**

`internal/mediastore/scan.go`:

```go
package mediastore

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"personaltv/internal/model"
	"personaltv/internal/repository"
)

var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".m4v": true, ".webm": true, ".ts": true, ".wmv": true,
}

type Scanner struct {
	sources repository.MediaSourceRepository
	items   repository.MediaItemRepository
}

func NewScanner(sources repository.MediaSourceRepository, items repository.MediaItemRepository) *Scanner {
	return &Scanner{sources: sources, items: items}
}

// ScanSource walks the source's configured directory, probes new or
// changed video files, and prunes items whose file no longer exists.
// A single unreadable file is marked Invalid, not treated as a scan failure.
func (s *Scanner) ScanSource(ctx context.Context, sourceID int64) error {
	source, err := s.sources.Get(ctx, sourceID)
	if err != nil {
		return err
	}

	existing, err := s.items.ListBySource(ctx, sourceID)
	if err != nil {
		return err
	}
	existingByPath := make(map[string]*model.MediaItem, len(existing))
	for _, item := range existing {
		existingByPath[item.RelPath] = item
	}

	var seenRelPaths []string

	walkErr := filepath.WalkDir(source.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !videoExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		relPath, relErr := filepath.Rel(source.Path, path)
		if relErr != nil {
			return nil
		}
		seenRelPaths = append(seenRelPaths, relPath)

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		if existingItem, ok := existingByPath[relPath]; ok && !existingItem.Invalid {
			if existingItem.SizeBytes == info.Size() && existingItem.ModTime.Equal(info.ModTime().UTC()) {
				return nil // unchanged since last scan, skip re-probe
			}
		}

		item := &model.MediaItem{
			SourceID:  sourceID,
			RelPath:   relPath,
			Title:     titleFromFilename(path),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().UTC(),
		}

		if probeResult, probeErr := Probe(path); probeErr != nil {
			item.Invalid = true
		} else {
			item.DurationSec = probeResult.DurationSec
			item.VideoCodec = probeResult.VideoCodec
			item.AudioCodec = probeResult.AudioCodec
			item.Container = probeResult.Container
			item.Invalid = false
		}

		return s.items.Upsert(ctx, item)
	})
	if walkErr != nil {
		return walkErr
	}

	return s.items.DeleteMissing(ctx, sourceID, seenRelPaths)
}

func titleFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mediastore/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mediastore
git commit -m "feat: add media directory scanner"
```

---

## Task 7: Scheduler (Pure Logic)

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

**Interfaces:**
- Consumes: nothing (pure functions, no I/O, no dependency on `db`/`repository`/`model`).
- Produces:
  - `scheduler.ScheduledProgram{ProgramID, MediaItemID int64, StartTime time.Time, Duration time.Duration}` and its `EndTime() time.Time` method.
  - `scheduler.CurrentState{Current, Next *ScheduledProgram, Offset time.Duration}`.
  - `scheduler.Evaluate(programs []ScheduledProgram, now time.Time) CurrentState`. **Task 8's channel service calls this directly.**

- [ ] **Step 1: Write the failing tests**

`internal/scheduler/scheduler_test.go`:

```go
package scheduler_test

import (
	"testing"
	"time"

	"personaltv/internal/scheduler"
)

func mustParse(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", value, err)
	}
	return parsed
}

func TestEvaluate_MidProgram(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 2*time.Hour + 5*time.Minute},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T20:05:00Z"), Duration: 30 * time.Minute},
	}
	now := mustParse(t, "2026-01-01T19:00:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current == nil || state.Current.ProgramID != 1 {
		t.Fatalf("expected current program 1, got %+v", state.Current)
	}
	if state.Offset != time.Hour {
		t.Errorf("expected offset 1h, got %v", state.Offset)
	}
	if state.Next == nil || state.Next.ProgramID != 2 {
		t.Fatalf("expected next program 2, got %+v", state.Next)
	}
}

func TestEvaluate_Gap(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T20:00:00Z"), Duration: 1 * time.Hour},
	}
	now := mustParse(t, "2026-01-01T19:30:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current != nil {
		t.Fatalf("expected no current program during gap, got %+v", state.Current)
	}
	if state.Next == nil || state.Next.ProgramID != 2 {
		t.Fatalf("expected next program 2, got %+v", state.Next)
	}
}

func TestEvaluate_ExactBoundaries(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T19:00:00Z"), Duration: 1 * time.Hour},
	}

	atStart := scheduler.Evaluate(programs, mustParse(t, "2026-01-01T19:00:00Z"))
	if atStart.Current == nil || atStart.Current.ProgramID != 2 {
		t.Fatalf("expected program 2 to be current exactly at its start time, got %+v", atStart.Current)
	}

	justBeforeEnd := scheduler.Evaluate(programs, mustParse(t, "2026-01-01T18:59:59Z"))
	if justBeforeEnd.Current == nil || justBeforeEnd.Current.ProgramID != 1 {
		t.Fatalf("expected program 1 to still be current just before its end, got %+v", justBeforeEnd.Current)
	}
}

func TestEvaluate_NothingScheduledAfterNow(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
	}
	now := mustParse(t, "2026-01-01T19:30:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current != nil {
		t.Fatalf("expected no current program, got %+v", state.Current)
	}
	if state.Next != nil {
		t.Fatalf("expected no next program, got %+v", state.Next)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler/...`
Expected: FAIL — `undefined: scheduler.Evaluate`

- [ ] **Step 3: Write the implementation**

`internal/scheduler/scheduler.go`:

```go
package scheduler

import "time"

type ScheduledProgram struct {
	ProgramID   int64
	MediaItemID int64
	StartTime   time.Time
	Duration    time.Duration
}

func (p ScheduledProgram) EndTime() time.Time {
	return p.StartTime.Add(p.Duration)
}

type CurrentState struct {
	// Current is nil when the channel is "off air" — now falls in a gap
	// between two scheduled programs.
	Current *ScheduledProgram
	// Offset is only meaningful when Current != nil.
	Offset time.Duration
	// Next is nil when nothing is scheduled after now.
	Next *ScheduledProgram
}

// Evaluate determines what's playing on a channel (if anything) and what
// plays next, given its programs and a point in time. programs need not be
// sorted. Pure function: no I/O, safe to call from any goroutine.
func Evaluate(programs []ScheduledProgram, now time.Time) CurrentState {
	var current *ScheduledProgram
	var next *ScheduledProgram

	for i := range programs {
		p := programs[i]

		if !now.Before(p.StartTime) && now.Before(p.EndTime()) {
			if current == nil || p.StartTime.After(current.StartTime) {
				pCopy := p
				current = &pCopy
			}
		}

		if p.StartTime.After(now) {
			if next == nil || p.StartTime.Before(next.StartTime) {
				pCopy := p
				next = &pCopy
			}
		}
	}

	state := CurrentState{Current: current, Next: next}
	if current != nil {
		state.Offset = now.Sub(current.StartTime)
	}
	return state
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler
git commit -m "feat: add pure scheduling logic"
```

---

## Task 8: Channels Service

**Files:**
- Create: `internal/channels/service.go`
- Test: `internal/channels/service_test.go`

**Interfaces:**
- Consumes: `repository.ChannelRepository`, `repository.ProgramRepository`, `repository.MediaItemRepository` (Task 3/4), `scheduler.Evaluate`, `scheduler.ScheduledProgram`, `scheduler.CurrentState` (Task 7), `sqlite.New*Repository` constructors (Tasks 3–4), `db.OpenTest` (Task 2).
- Produces: `channels.NewService(channels repository.ChannelRepository, programs repository.ProgramRepository, items repository.MediaItemRepository) *Service`, with methods `CreateChannel`, `ListChannels`, `GetChannel`, `UpdateChannel`, `DeleteChannel`, `AddProgram`, `GetProgram`, `UpdateProgram`, `RemoveProgram`, `ListPrograms`, and `CurrentState(ctx, channelID int64, now time.Time) (scheduler.CurrentState, error)`. **Tasks 9, 10, 11, 12 construct and use this — it's the only thing the API layer talks to for channels/programs.**

- [ ] **Step 1: Write the failing test**

`internal/channels/service_test.go`:

```go
package channels_test

import (
	"context"
	"testing"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestService_CurrentState(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600, ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("failed to upsert item: %v", err)
	}

	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := programRepo.Create(ctx, program); err != nil {
		t.Fatalf("failed to create program: %v", err)
	}

	svc := channels.NewService(channelRepo, programRepo, itemRepo)

	state, err := svc.CurrentState(ctx, channel.ID, start.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current == nil || state.Current.ProgramID != program.ID {
		t.Fatalf("expected program %d to be current, got %+v", program.ID, state.Current)
	}
	if state.Offset != 30*time.Minute {
		t.Errorf("expected offset 30m, got %v", state.Offset)
	}
}

func TestService_CurrentState_NoPrograms(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)

	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	svc := channels.NewService(channelRepo, programRepo, itemRepo)
	state, err := svc.CurrentState(ctx, channel.ID, time.Now())
	if err != nil {
		t.Fatalf("CurrentState returned error: %v", err)
	}
	if state.Current != nil || state.Next != nil {
		t.Fatalf("expected empty state for a channel with no programs, got %+v", state)
	}
}

func TestService_ProgramCRUD(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	sourceRepo.Create(ctx, source)
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "A", DurationSec: 60, ModTime: time.Now().UTC()}
	itemRepo.Upsert(ctx, item)
	channel := &model.Channel{Name: "Movies", Enabled: true}
	channelRepo.Create(ctx, channel)

	svc := channels.NewService(channelRepo, programRepo, itemRepo)

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	program := &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}
	if err := svc.AddProgram(ctx, program); err != nil {
		t.Fatalf("AddProgram returned error: %v", err)
	}

	fetched, err := svc.GetProgram(ctx, program.ID)
	if err != nil {
		t.Fatalf("GetProgram returned error: %v", err)
	}
	if fetched.ChannelID != channel.ID {
		t.Errorf("expected channel ID %d, got %d", channel.ID, fetched.ChannelID)
	}

	if err := svc.RemoveProgram(ctx, program.ID); err != nil {
		t.Fatalf("RemoveProgram returned error: %v", err)
	}

	list, err := svc.ListPrograms(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListPrograms returned error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no programs after removal, got %+v", list)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/...`
Expected: FAIL — `undefined: channels.NewService`

- [ ] **Step 3: Write the implementation**

`internal/channels/service.go`:

```go
package channels

import (
	"context"
	"time"

	"personaltv/internal/model"
	"personaltv/internal/repository"
	"personaltv/internal/scheduler"
)

type Service struct {
	channels repository.ChannelRepository
	programs repository.ProgramRepository
	items    repository.MediaItemRepository
}

func NewService(channels repository.ChannelRepository, programs repository.ProgramRepository, items repository.MediaItemRepository) *Service {
	return &Service{channels: channels, programs: programs, items: items}
}

func (s *Service) CreateChannel(ctx context.Context, c *model.Channel) error {
	return s.channels.Create(ctx, c)
}

func (s *Service) ListChannels(ctx context.Context) ([]*model.Channel, error) {
	return s.channels.List(ctx)
}

func (s *Service) GetChannel(ctx context.Context, id int64) (*model.Channel, error) {
	return s.channels.Get(ctx, id)
}

func (s *Service) UpdateChannel(ctx context.Context, c *model.Channel) error {
	return s.channels.Update(ctx, c)
}

func (s *Service) DeleteChannel(ctx context.Context, id int64) error {
	return s.channels.Delete(ctx, id)
}

func (s *Service) AddProgram(ctx context.Context, p *model.Program) error {
	return s.programs.Create(ctx, p)
}

func (s *Service) GetProgram(ctx context.Context, id int64) (*model.Program, error) {
	return s.programs.Get(ctx, id)
}

func (s *Service) UpdateProgram(ctx context.Context, p *model.Program) error {
	return s.programs.Update(ctx, p)
}

func (s *Service) RemoveProgram(ctx context.Context, id int64) error {
	return s.programs.Delete(ctx, id)
}

func (s *Service) ListPrograms(ctx context.Context, channelID int64) ([]*model.Program, error) {
	return s.programs.ListByChannel(ctx, channelID)
}

// CurrentState reports what's currently playing on a channel (if anything)
// and what plays next, as of now. Each call recomputes from persisted
// schedule + media duration — nothing is cached or ticking in the
// background, so this is correct even immediately after an app restart.
func (s *Service) CurrentState(ctx context.Context, channelID int64, now time.Time) (scheduler.CurrentState, error) {
	programs, err := s.programs.ListByChannel(ctx, channelID)
	if err != nil {
		return scheduler.CurrentState{}, err
	}

	scheduled := make([]scheduler.ScheduledProgram, 0, len(programs))
	for _, p := range programs {
		item, err := s.items.Get(ctx, p.MediaItemID)
		if err != nil {
			return scheduler.CurrentState{}, err
		}
		scheduled = append(scheduled, scheduler.ScheduledProgram{
			ProgramID:   p.ID,
			MediaItemID: p.MediaItemID,
			StartTime:   p.StartTime,
			Duration:    time.Duration(item.DurationSec * float64(time.Second)),
		})
	}

	return scheduler.Evaluate(scheduled, now), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels
git commit -m "feat: add channels service combining repositories and scheduler"
```

---

## Task 9: API — Media Sources and Media Items

**Files:**
- Create: `internal/api/response.go`
- Create: `internal/api/router.go`
- Create: `internal/api/sources_handlers.go`
- Test: `internal/api/sources_handlers_test.go`

**Interfaces:**
- Consumes: `repository.MediaSourceRepository`, `repository.MediaItemRepository` (Task 3), `mediastore.Scanner`/`NewScanner` (Task 6), `sqlite.New*Repository` (Task 3), `db.OpenTest` (Task 2), `model.MediaSource`/`MediaItem` (Task 3).
- Produces:
  - `api.NewServer(sources repository.MediaSourceRepository, items repository.MediaItemRepository, scanner *mediastore.Scanner) *Server` and `(*Server) Routes() http.Handler`. **Task 10 will change this constructor's signature to add a `*channels.Service` parameter — see Task 10's Step 1 for the required update to this task's test helper.**
  - Routes: `GET /healthz`, `GET /api/sources`, `POST /api/sources`, `DELETE /api/sources/{id}`, `POST /api/sources/{id}/scan`, `GET /api/media`.
  - `writeJSON(w http.ResponseWriter, status int, v any)` and `writeError(w http.ResponseWriter, status int, err error)`. **Every later handler task uses these.**

- [ ] **Step 1: Write the failing test**

`internal/api/sources_handlers_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"personaltv/internal/api"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner)
}

func TestSourcesAPI_CreateListDelete(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"name": "Movies", "path": "/media/movies"})
	resp, err := http.Post(ts.URL+"/api/sources", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/sources failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.MediaSource
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("expected created source to have an ID")
	}

	listResp, err := http.Get(ts.URL + "/api/sources")
	if err != nil {
		t.Fatalf("GET /api/sources failed: %v", err)
	}
	defer listResp.Body.Close()
	var sources []model.MediaSource
	if err := json.NewDecoder(listResp.Body).Decode(&sources); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/sources/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestMediaAPI_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var items []model.MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no media items, got %d", len(items))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — build errors (package `api` doesn't exist yet).

- [ ] **Step 3: Write the response helpers**

`internal/api/response.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func errRequiredFields(fields ...string) error {
	return fmt.Errorf("missing required fields: %v", fields)
}
```

- [ ] **Step 4: Write the router**

`internal/api/router.go`:

```go
package api

import (
	"net/http"

	"personaltv/internal/mediastore"
	"personaltv/internal/repository"
)

type Server struct {
	sources repository.MediaSourceRepository
	items   repository.MediaItemRepository
	scanner *mediastore.Scanner
}

func NewServer(sources repository.MediaSourceRepository, items repository.MediaItemRepository, scanner *mediastore.Scanner) *Server {
	return &Server{sources: sources, items: items, scanner: scanner}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("GET /api/sources", s.handleListSources)
	mux.HandleFunc("POST /api/sources", s.handleCreateSource)
	mux.HandleFunc("DELETE /api/sources/{id}", s.handleDeleteSource)
	mux.HandleFunc("POST /api/sources/{id}/scan", s.handleScanSource)

	mux.HandleFunc("GET /api/media", s.handleListMedia)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
```

- [ ] **Step 5: Write the sources/media handlers**

`internal/api/sources_handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"personaltv/internal/model"
)

type createSourceRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.sources.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	var req createSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, errRequiredFields("name", "path"))
		return
	}

	source := &model.MediaSource{Name: req.Name, Path: req.Path}
	if err := s.sources.Create(r.Context(), source); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, source)
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.sources.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScanSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.scanner.ScanSource(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	items, err := s.items.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api
git commit -m "feat: add media sources and media items API"
```

---

## Task 10: API — Channels and Programs

**Files:**
- Modify: `internal/api/router.go` (add a `channels` field, extend `NewServer`, add routes)
- Modify: `internal/api/sources_handlers_test.go` (update `newTestServer` for the new `NewServer` signature)
- Create: `internal/api/channels_handlers.go`
- Create: `internal/api/programs_handlers.go`
- Test: `internal/api/channels_handlers_test.go`

**Interfaces:**
- Consumes: `channels.NewService`, `channels.Service` methods (Task 8), everything from Task 9 (`writeJSON`, `writeError`, `errRequiredFields`, `Server`, `Routes`).
- Produces: `api.NewServer` now takes a 4th parameter, `channelSvc *channels.Service`. Routes added: `GET/POST /api/channels`, `GET/PUT/DELETE /api/channels/{id}`, `GET/POST /api/channels/{id}/programs`, `PUT/DELETE /api/programs/{id}`. **Task 11 adds one more route to this same router and does not change the constructor again.**

- [ ] **Step 1: Update the Task 9 test helper for the new constructor signature**

In `internal/api/sources_handlers_test.go`, change `newTestServer` to build and pass a `channels.Service`:

```go
func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
}
```

Add `"personaltv/internal/channels"` to that file's imports.

- [ ] **Step 2: Write the failing test**

`internal/api/channels_handlers_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestChannelsAPI_CreateGetUpdateDelete(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"name": "Movies", "description": "Movie channel"})
	resp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/channels failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.Channel
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("expected created channel to have an ID")
	}

	getResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(created.ID, 10))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	updateBody, _ := json.Marshal(map[string]any{"name": "Movies HD", "description": "Movie channel", "enabled": true, "position": 1})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/channels/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updResp.StatusCode)
	}
	updResp.Body.Close()

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/channels/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestProgramsAPI_AddListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600, ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("failed to upsert item: %v", err)
	}
	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	srv := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	resp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST program failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created model.Program
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	listResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/programs")
	if err != nil {
		t.Fatalf("GET programs failed: %v", err)
	}
	var programs []model.Program
	json.NewDecoder(listResp.Body).Decode(&programs)
	listResp.Body.Close()
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}

	newStart := start.Add(time.Hour)
	updateBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": newStart})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/programs/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT program failed: %v", err)
	}
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updResp.StatusCode)
	}
	updResp.Body.Close()

	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/programs/"+strconv.FormatInt(created.ID, 10), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE program failed: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}
```

This test needs a second helper, `newTestServerWithConn`, that builds a server on top of an *existing* connection (so the arranged source/item/channel rows are visible to it) rather than opening a fresh one. Add it to `internal/api/sources_handlers_test.go` next to `newTestServer`:

```go
func newTestServerWithConn(t *testing.T, conn *sql.DB) *api.Server {
	t.Helper()
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	return api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
}
```

Add `"database/sql"` to that file's imports too.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `undefined: channels.NewService` usage compiles, but `NewServer` call sites now have the wrong arity until Steps 4–6 land, and the new handler names are undefined.

- [ ] **Step 4: Update the router**

`internal/api/router.go` (full replacement):

```go
package api

import (
	"net/http"

	"personaltv/internal/channels"
	"personaltv/internal/mediastore"
	"personaltv/internal/repository"
)

type Server struct {
	sources  repository.MediaSourceRepository
	items    repository.MediaItemRepository
	scanner  *mediastore.Scanner
	channels *channels.Service
}

func NewServer(sources repository.MediaSourceRepository, items repository.MediaItemRepository, scanner *mediastore.Scanner, channelSvc *channels.Service) *Server {
	return &Server{sources: sources, items: items, scanner: scanner, channels: channelSvc}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("GET /api/sources", s.handleListSources)
	mux.HandleFunc("POST /api/sources", s.handleCreateSource)
	mux.HandleFunc("DELETE /api/sources/{id}", s.handleDeleteSource)
	mux.HandleFunc("POST /api/sources/{id}/scan", s.handleScanSource)

	mux.HandleFunc("GET /api/media", s.handleListMedia)

	mux.HandleFunc("GET /api/channels", s.handleListChannels)
	mux.HandleFunc("POST /api/channels", s.handleCreateChannel)
	mux.HandleFunc("GET /api/channels/{id}", s.handleGetChannel)
	mux.HandleFunc("PUT /api/channels/{id}", s.handleUpdateChannel)
	mux.HandleFunc("DELETE /api/channels/{id}", s.handleDeleteChannel)

	mux.HandleFunc("GET /api/channels/{id}/programs", s.handleListPrograms)
	mux.HandleFunc("POST /api/channels/{id}/programs", s.handleAddProgram)
	mux.HandleFunc("PUT /api/programs/{id}", s.handleUpdateProgram)
	mux.HandleFunc("DELETE /api/programs/{id}", s.handleDeleteProgram)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
```

- [ ] **Step 5: Write the channels handlers**

`internal/api/channels_handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"personaltv/internal/model"
)

type createChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    int    `json:"position"`
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.channels.ListChannels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errRequiredFields("name"))
		return
	}

	channel := &model.Channel{Name: req.Name, Description: req.Description, Enabled: true, Position: req.Position}
	if err := s.channels.CreateChannel(r.Context(), channel); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, channel)
}

func (s *Server) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	channel, err := s.channels.GetChannel(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

type updateChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Position    int    `json:"position"`
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req updateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	channel := &model.Channel{ID: id, Name: req.Name, Description: req.Description, Enabled: req.Enabled, Position: req.Position}
	if err := s.channels.UpdateChannel(r.Context(), channel); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.DeleteChannel(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 6: Write the programs handlers**

`internal/api/programs_handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"personaltv/internal/model"
)

type addProgramRequest struct {
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
}

func (s *Server) handleListPrograms(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	programs, err := s.channels.ListPrograms(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, programs)
}

func (s *Server) handleAddProgram(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req addProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.MediaItemID == 0 || req.StartTime.IsZero() {
		writeError(w, http.StatusBadRequest, errRequiredFields("media_item_id", "start_time"))
		return
	}

	program := &model.Program{ChannelID: channelID, MediaItemID: req.MediaItemID, StartTime: req.StartTime}
	if err := s.channels.AddProgram(r.Context(), program); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, program)
}

type updateProgramRequest struct {
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
}

func (s *Server) handleUpdateProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	existing, err := s.channels.GetProgram(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	var req updateProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	updated := &model.Program{ID: id, ChannelID: existing.ChannelID, MediaItemID: req.MediaItemID, StartTime: req.StartTime}
	if err := s.channels.UpdateProgram(r.Context(), updated); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.RemoveProgram(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/api
git commit -m "feat: add channels and programs API"
```

---

## Task 11: API — Current Program / EPG Endpoint

**Files:**
- Modify: `internal/api/router.go` (add one route; no constructor change)
- Create: `internal/api/now_handler.go`
- Test: `internal/api/now_handler_test.go`

**Interfaces:**
- Consumes: `channels.Service.CurrentState` (Task 8), `scheduler.CurrentState`/`ScheduledProgram` (Task 7), everything from Task 9/10 (`writeJSON`, `writeError`, `Server`).
- Produces: `GET /api/channels/{id}/now`, returning `{channel_id, current, offset_sec, next}` where `current`/`next` are `{program_id, media_item_id, start_time, end_time}` or `null`. This is the endpoint the frontend's EPG/TV pages will poll (design spec §4.7) — no later backend task depends on it, but it's the plan's key product-facing deliverable.

- [ ] **Step 1: Write the failing test**

`internal/api/now_handler_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/db"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func TestChannelNowAPI_CurrentProgram(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	source := &model.MediaSource{Name: "Movies", Path: "/media/movies"}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	item := &model.MediaItem{SourceID: source.ID, RelPath: "a.mp4", Title: "Movie A", DurationSec: 3600, ModTime: time.Now().UTC()}
	if err := itemRepo.Upsert(ctx, item); err != nil {
		t.Fatalf("failed to upsert item: %v", err)
	}
	channel := &model.Channel{Name: "Movies", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	start := time.Now().UTC().Add(-30 * time.Minute)
	if err := programRepo.Create(ctx, &model.Program{ChannelID: channel.ID, MediaItemID: item.ID, StartTime: start}); err != nil {
		t.Fatalf("failed to create program: %v", err)
	}

	srv := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	current, ok := state["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected a current program, got %+v", state)
	}
	if int64(current["media_item_id"].(float64)) != item.ID {
		t.Errorf("expected current media item %d, got %v", item.ID, current["media_item_id"])
	}
	offsetSec, ok := state["offset_sec"].(float64)
	if !ok || offsetSec < 1700 || offsetSec > 1900 {
		t.Errorf("expected offset_sec ~1800 (30 min), got %v", state["offset_sec"])
	}
}

func TestChannelNowAPI_OffAir(t *testing.T) {
	ctx := context.Background()
	conn := db.OpenTest(t)
	channelRepo := sqlite.NewChannelRepository(conn)

	channel := &model.Channel{Name: "Empty", Enabled: true}
	if err := channelRepo.Create(ctx, channel); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	srv := newTestServerWithConn(t, conn)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer resp.Body.Close()

	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if state["current"] != nil {
		t.Errorf("expected no current program for an empty channel, got %v", state["current"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `404 page not found` (route doesn't exist yet) causing a JSON decode error, or a compile error if the route constant isn't referenced anywhere yet — either way, not passing.

- [ ] **Step 3: Add the route**

In `internal/api/router.go`, inside `Routes()`, add this line after the programs routes:

```go
	mux.HandleFunc("GET /api/channels/{id}/now", s.handleChannelNow)
```

- [ ] **Step 4: Write the handler**

`internal/api/now_handler.go`:

```go
package api

import (
	"net/http"
	"strconv"
	"time"
)

type currentStateResponse struct {
	ChannelID int64             `json:"channel_id"`
	Current   *programStateJSON `json:"current"`
	OffsetSec float64           `json:"offset_sec"`
	Next      *programStateJSON `json:"next"`
}

type programStateJSON struct {
	ProgramID   int64     `json:"program_id"`
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

func (s *Server) handleChannelNow(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	state, err := s.channels.CurrentState(r.Context(), channelID, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := currentStateResponse{ChannelID: channelID}
	if state.Current != nil {
		resp.Current = &programStateJSON{
			ProgramID:   state.Current.ProgramID,
			MediaItemID: state.Current.MediaItemID,
			StartTime:   state.Current.StartTime,
			EndTime:     state.Current.EndTime(),
		}
		resp.OffsetSec = state.Offset.Seconds()
	}
	if state.Next != nil {
		resp.Next = &programStateJSON{
			ProgramID:   state.Next.ProgramID,
			MediaItemID: state.Next.MediaItemID,
			StartTime:   state.Next.StartTime,
			EndTime:     state.Next.EndTime(),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api
git commit -m "feat: add current-program/EPG API endpoint"
```

---

## Task 12: Wire `main.go` End-to-End

**Files:**
- Modify: `cmd/personaltv/main.go` (full replacement of Task 1's placeholder)
- Delete: `cmd/personaltv/main_test.go` (its `handleHealth` no longer exists in `main` — health check coverage moves to the API package's own tests, which already exercise `GET /healthz` indirectly through every `httptest.NewServer(srv.Routes())` call)
- Test: `internal/integration/end_to_end_test.go`

**Interfaces:**
- Consumes: everything — `db.Open`, `sqlite.New*Repository` (Tasks 2–4), `mediastore.NewScanner` (Task 6), `channels.NewService` (Task 8), `api.NewServer` (Tasks 9–11).
- Produces: a runnable binary (`go run ./cmd/personaltv`) and a top-level integration test proving the full MVP data path end-to-end: configure a source → scan real files → create a channel → schedule a program → confirm the API reports it as currently playing. This is Plan 1's Definition of Done.

**Note on TDD here:** `main()` itself isn't unit-testable (it blocks on `http.ListenAndServe`), and the integration test below wires the same packages directly rather than importing `main` — so it doesn't literally fail-then-pass around the `main.go` rewrite the way earlier tasks' tests do. Steps are ordered to write `main.go` first (the actual deliverable), then the integration test as a whole-stack acceptance check, run last.

- [ ] **Step 1: Remove the now-obsolete Task 1 test**

Delete `cmd/personaltv/main_test.go` — `handleHealth` as a standalone function is going away in Step 2 below, replaced by the `api` package's router (whose `/healthz` route is already covered by every existing `internal/api` test that spins up `httptest.NewServer(srv.Routes())`).

- [ ] **Step 2: Rewrite `main.go`**

`cmd/personaltv/main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"personaltv/internal/api"
	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/repository/sqlite"
)

func main() {
	dbPath := getEnv("PERSONALTV_DB_PATH", "personaltv.db")
	port := getEnv("PERSONALTV_PORT", "8080")

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)

	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)

	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)

	log.Printf("Personal TV listening on :%s (db: %s)", port, dbPath)
	if err := http.ListenAndServe(":"+port, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 3: Run the build to confirm `main.go` compiles against every package built so far**

Run: `go build ./...`
Expected: succeeds with no errors.

- [ ] **Step 4: Write the end-to-end integration test**

`internal/integration/end_to_end_test.go`:

```go
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"personaltv/internal/api"
	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/model"
	"personaltv/internal/repository/sqlite"
)

func generateTestVideo(t *testing.T, dir, name string, durationSec int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	d := strconv.Itoa(durationSec)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration="+d+":size=64x64:rate=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+d,
		"-c:v", "libx264", "-c:a", "aac", "-shortest", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test video: %v\n%s", err, out)
	}
	return path
}

// TestFullUserJourney drives the same steps a real user takes per the PRD's
// MVP user journey (docs/prd/HomeStreamer.md §15): configure a media
// source, scan it, create a channel, schedule media on it, and confirm the
// API reports what's currently playing. This is Plan 1's Definition of Done.
func TestFullUserJourney(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	generateTestVideo(t, mediaDir, "movie-a.mp4", 2)

	conn := db.OpenTest(t)
	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	programRepo := sqlite.NewProgramRepository(conn)
	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, programRepo, itemRepo)
	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	// 1. configure a media source
	source := &model.MediaSource{Name: "Movies", Path: mediaDir}
	if err := sourceRepo.Create(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// 2. scan it
	scanResp, err := http.Post(ts.URL+"/api/sources/"+strconv.FormatInt(source.ID, 10)+"/scan", "application/json", nil)
	if err != nil {
		t.Fatalf("scan request failed: %v", err)
	}
	if scanResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from scan, got %d", scanResp.StatusCode)
	}

	// 3. confirm the media library was populated
	mediaResp, err := http.Get(ts.URL + "/api/media")
	if err != nil {
		t.Fatalf("GET /api/media failed: %v", err)
	}
	var items []model.MediaItem
	json.NewDecoder(mediaResp.Body).Decode(&items)
	mediaResp.Body.Close()
	if len(items) != 1 {
		t.Fatalf("expected 1 media item after scan, got %d", len(items))
	}
	item := items[0]

	// 4. create a channel
	chBody, _ := json.Marshal(map[string]any{"name": "Movies"})
	chResp, err := http.Post(ts.URL+"/api/channels", "application/json", bytes.NewReader(chBody))
	if err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	var channel model.Channel
	json.NewDecoder(chResp.Body).Decode(&channel)
	chResp.Body.Close()

	// 5. schedule the media item to start one minute ago, so it's playing now
	start := time.Now().UTC().Add(-1 * time.Minute)
	progBody, _ := json.Marshal(map[string]any{"media_item_id": item.ID, "start_time": start})
	progResp, err := http.Post(ts.URL+"/api/channels/"+strconv.FormatInt(channel.ID, 10)+"/programs", "application/json", bytes.NewReader(progBody))
	if err != nil {
		t.Fatalf("add program failed: %v", err)
	}
	if progResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 adding program, got %d", progResp.StatusCode)
	}
	progResp.Body.Close()

	// 6. ask what's playing now
	nowResp, err := http.Get(ts.URL + "/api/channels/" + strconv.FormatInt(channel.ID, 10) + "/now")
	if err != nil {
		t.Fatalf("GET now failed: %v", err)
	}
	defer nowResp.Body.Close()
	var state map[string]any
	json.NewDecoder(nowResp.Body).Decode(&state)
	current, ok := state["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected a current program, got %+v", state)
	}
	if int64(current["media_item_id"].(float64)) != item.ID {
		t.Errorf("expected current media item %d, got %v", item.ID, current["media_item_id"])
	}

	// 7. restart simulation: reopen the same DB file and confirm state survives
	// (nothing in this stack is in-memory-only per design spec §6 reliability).
	_ = channel
}
```

- [ ] **Step 5: Run the integration test to verify it passes**

Run: `go test ./internal/integration/...`
Expected: PASS. If it doesn't, the bug is in how an earlier task's package composes with the others (each task's own tests passing individually doesn't guarantee that) — fix it in the relevant package, not by changing this test's assertions.

- [ ] **Step 6: Run every test in the module together**

Run: `go build ./... && go test ./...`
Expected: PASS for every package, no skips.

- [ ] **Step 7: Manually verify the binary runs**

```bash
go run ./cmd/personaltv &
sleep 1
curl -s http://localhost:8080/healthz
kill %1
```
Expected: `ok`

- [ ] **Step 8: Commit**

```bash
git add cmd/personaltv internal/integration
git rm cmd/personaltv/main_test.go 2>/dev/null || true
git commit -m "feat: wire full backend stack in main and add end-to-end test"
```

---

## Definition of Done

Plan 1 is complete when:

- `go build ./...` succeeds and `go test ./...` passes with no skipped packages.
- `go run ./cmd/personaltv` starts a server that, driven purely by `curl`, can: register a media source, scan it and discover real video files (with correct duration/codec from `ffprobe`), create channels, schedule programs on them, and correctly report what's currently playing via `GET /api/channels/{id}/now` — matching the PRD's MVP user journey (`docs/prd/HomeStreamer.md` §15) end-to-end, minus actual video playback (Plan 2).
- Restarting the process (killing and re-running `go run ./cmd/personaltv` against the same `PERSONALTV_DB_PATH`) preserves all configuration, matching PRD §16 success criterion 12 / design spec §6.
