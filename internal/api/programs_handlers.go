package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"personaltv/internal/model"
)

type addProgramRequest struct {
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
}

func (s *Server) handleListPrograms(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	programs, err := s.channels.ListPrograms(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, programs)
}

func (s *Server) handleAddProgram(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req addProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.MediaItemID == 0 || req.StartTime.IsZero() {
		writeError(w, http.StatusBadRequest, errRequiredFields("media_item_id", "start_time"))
		return
	}

	program := &model.Program{ChannelID: channelID, MediaItemID: req.MediaItemID, StartTime: req.StartTime}
	if err := s.channels.AddProgram(r.Context(), program); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, program)
}

type updateProgramRequest struct {
	MediaItemID int64     `json:"media_item_id"`
	StartTime   time.Time `json:"start_time"`
}

func (s *Server) handleUpdateProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	existing, err := s.channels.GetProgram(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	var req updateProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	updated := &model.Program{ID: id, ChannelID: existing.ChannelID, MediaItemID: req.MediaItemID, StartTime: req.StartTime}
	if err := s.channels.UpdateProgram(r.Context(), updated); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteProgram(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.RemoveProgram(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
