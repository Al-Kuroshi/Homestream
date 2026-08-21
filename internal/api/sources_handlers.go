package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	if err := validateSourcePath(req.Path); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	source := &model.MediaSource{Name: req.Name, Path: req.Path}
	if err := s.sources.Create(r.Context(), source); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, source)
}

// validateSourcePath applies the minimum sanity checks on a media source
// directory before it is persisted and later handed to filepath.WalkDir.
// The MVP has no authentication by design, so an unvalidated path lets
// anyone who can reach the port point the scanner at, say, "/". This is not
// a confinement mechanism — it just refuses paths that are obviously not a
// media directory.
func validateSourcePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path is not readable: %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %q", path)
	}
	return nil
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
