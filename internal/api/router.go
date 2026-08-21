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
