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
