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
