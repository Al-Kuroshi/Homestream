package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"personaltv/internal/channels"
	"personaltv/internal/model"
)

type slotRequest struct {
	Kind           string     `json:"kind"`
	MediaItemID    *int64     `json:"media_item_id,omitempty"`
	GapDurationSec *float64   `json:"gap_duration_sec,omitempty"`
	GapLabel       string     `json:"gap_label"`
	Recurring      bool       `json:"recurring"`
	DayOfWeek      *int       `json:"day_of_week,omitempty"`
	Position       *int       `json:"position,omitempty"`
	StartTime      *time.Time `json:"start_time,omitempty"`
}

func (req slotRequest) toSlot(channelID int64) (*model.Slot, error) {
	if req.Kind != model.SlotKindMedia && req.Kind != model.SlotKindGap {
		return nil, errRequiredFields("kind")
	}
	if req.Kind == model.SlotKindMedia && req.MediaItemID == nil {
		return nil, errRequiredFields("media_item_id")
	}
	if req.Kind == model.SlotKindGap && (req.GapDurationSec == nil || *req.GapDurationSec <= 0) {
		return nil, errRequiredFields("gap_duration_sec")
	}
	if req.Recurring && (req.DayOfWeek == nil || req.Position == nil) {
		return nil, errRequiredFields("day_of_week", "position")
	}
	if !req.Recurring && req.StartTime == nil {
		return nil, errRequiredFields("start_time")
	}
	return &model.Slot{
		ChannelID:      channelID,
		Kind:           req.Kind,
		MediaItemID:    req.MediaItemID,
		GapDurationSec: req.GapDurationSec,
		GapLabel:       req.GapLabel,
		Recurring:      req.Recurring,
		DayOfWeek:      req.DayOfWeek,
		Position:       req.Position,
		StartTime:      req.StartTime,
	}, nil
}

func (s *Server) handleListSlots(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), channelID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	slots, err := s.channels.ListSlots(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

func (s *Server) handleAddSlot(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot, err := req.toSlot(channelID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.AddSlot(r.Context(), slot); err != nil {
		writeSlotError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, slot)
}

func (s *Server) handleUpdateSlot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := s.channels.GetSlot(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req slotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot, err := req.toSlot(existing.ChannelID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot.ID = id
	if err := s.channels.UpdateSlot(r.Context(), slot); err != nil {
		writeSlotError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, slot)
}

func (s *Server) handleDeleteSlot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.channels.RemoveSlot(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSlotError(w http.ResponseWriter, err error) {
	var verr *channels.ValidationError
	if errors.As(err, &verr) {
		writeError(w, http.StatusBadRequest, verr)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

// resolvedSlotResponse carries Kind/GapLabel alongside the resolved times so
// a client can tell a deliberate gap/break apart from a media occurrence.
// Without them a gap resolves to MediaItemID 0, which every consumer would
// otherwise render as a broken media lookup ("Media #0") rather than the
// scheduled break it actually is.
type resolvedSlotResponse struct {
	ProgramID   int64     `json:"program_id"`
	MediaItemID int64     `json:"media_item_id"`
	Kind        string    `json:"kind"`
	GapLabel    string    `json:"gap_label,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

func (s *Server) handleResolvedSlots(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errRequiredFields("from"))
		return
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errRequiredFields("to"))
		return
	}
	if _, err := s.channels.GetChannel(r.Context(), channelID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	resolved, err := s.channels.ResolvedWindow(r.Context(), channelID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	slotByID, err := s.slotsByID(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]resolvedSlotResponse, 0, len(resolved))
	for _, p := range resolved {
		item := resolvedSlotResponse{ProgramID: p.ProgramID, MediaItemID: p.MediaItemID, StartTime: p.StartTime, EndTime: p.EndTime()}
		if slot, ok := slotByID[p.ProgramID]; ok {
			item.Kind = slot.Kind
			item.GapLabel = slot.GapLabel
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// slotsByID indexes a channel's slots by ID. A resolved occurrence only
// carries its originating slot's ID (as ProgramID), so this is how a handler
// joins back to the slot's kind/gap_label.
func (s *Server) slotsByID(ctx context.Context, channelID int64) (map[int64]*model.Slot, error) {
	slots, err := s.channels.ListSlots(ctx, channelID)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*model.Slot, len(slots))
	for _, slot := range slots {
		byID[slot.ID] = slot
	}
	return byID, nil
}
