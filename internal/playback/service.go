package playback

import (
	"personaltv/internal/channels"
	"personaltv/internal/repository"
)

// Service ties together the schedule (channels.Service), the repositories
// needed to resolve a media item to a file, and the SessionManager that
// owns transcode sessions. Task 4 adds session-lookup delegating methods;
// Task 5 adds TuneIn, its primary operation. This constructor's signature
// is fixed here and does not change in either later task.
type Service struct {
	channels *channels.Service
	sources  repository.MediaSourceRepository
	items    repository.MediaItemRepository
	sessions *SessionManager
}

func NewService(channelSvc *channels.Service, sources repository.MediaSourceRepository, items repository.MediaItemRepository, sessions *SessionManager) *Service {
	return &Service{channels: channelSvc, sources: sources, items: items, sessions: sessions}
}
