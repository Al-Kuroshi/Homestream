package api

import (
	"net/http"
	"os"
	"strconv"

	"personaltv/internal/playback"
)

func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	item, err := s.items.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	source, err := s.sources.Get(r.Context(), item.SourceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	path := playback.ResolvePath(source, item)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	http.ServeContent(w, r, item.Title, info.ModTime(), f)
}
