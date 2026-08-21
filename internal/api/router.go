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

	mux.HandleFunc("GET /api/channels/{id}/now", s.handleChannelNow)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
