package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

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

func (s *Server) handleSessionFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")

	sess, ok := s.playback.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such playback session"))
		return
	}
	if failed, ferr := sess.Failed(); failed {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("playback session failed: %w", ferr))
		return
	}

	// file must be a plain filename within the session's own directory —
	// no path traversal (e.g. "../../etc/passwd") outside it.
	if file != filepath.Base(file) {
		writeError(w, http.StatusBadRequest, errors.New("invalid file name"))
		return
	}
	path := filepath.Join(sess.Dir, file)

	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()

	s.playback.TouchSession(id)

	switch filepath.Ext(file) {
	case ".m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	}

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeContent(w, r, file, info.ModTime(), f)
}

func (s *Server) handleChannelWatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	result, err := s.playback.TuneIn(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, watchResponse{
		Status:      result.Status,
		Mode:        result.Mode,
		MediaItemID: result.MediaItemID,
		OffsetSec:   result.OffsetSec,
		SessionID:   result.SessionID,
	})
}

type watchResponse struct {
	Status      string  `json:"status"`
	Mode        string  `json:"mode,omitempty"`
	MediaItemID int64   `json:"media_item_id,omitempty"`
	OffsetSec   float64 `json:"offset_sec,omitempty"`
	SessionID   string  `json:"session_id,omitempty"`
}
