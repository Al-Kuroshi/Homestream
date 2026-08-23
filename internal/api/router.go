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
	static   http.Handler
}

func NewServer(sources repository.MediaSourceRepository, items repository.MediaItemRepository, scanner *mediastore.Scanner, channelSvc *channels.Service) *Server {
	return &Server{sources: sources, items: items, scanner: scanner, channels: channelSvc}
}

// SetStaticHandler registers the handler used for any request that doesn't
// match /healthz or /api/*, e.g. the embedded frontend SPA (see
// cmd/personaltv/main.go). If never called, unmatched paths 404 as before
// — every existing test and NewServer call site is unaffected.
func (s *Server) SetStaticHandler(h http.Handler) {
	s.static = h
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

	if s.static != nil {
		// Registered without this, any unmatched /api/* path (a typo'd
		// endpoint, a wrong method, /api/ itself) would fall through to the
		// SPA catch-all below and return 200 text/html instead of 404 —
		// which web/src/api/http.ts's apiGet silently swallows into
		// `undefined` rather than throwing ApiError, since res.ok is true.
		// Go's ServeMux picks the most specific registered pattern, so the
		// exact "GET /api/channels"-style routes above still win over this.
		mux.Handle("/api/", http.NotFoundHandler())
		mux.Handle("/", s.static)
	}

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
