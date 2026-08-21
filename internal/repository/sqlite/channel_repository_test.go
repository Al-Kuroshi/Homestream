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
