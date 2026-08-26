package api

import (
	"net/http"
	"strconv"
	"time"
)

type currentStateResponse struct {
	ChannelID int64             `json:"channel_id"`
	Current   *programStateJSON `json:"current"`
	OffsetSec float64           `json:"offset_sec"`
	Next      *programStateJSON `json:"next"`
}

// Kind/GapLabel mirror resolvedSlotResponse's: a gap occurrence has
// MediaItemID 0, so without them the TV screen's next-up line has nothing to
// display but "Unknown" for a deliberately scheduled break.
type programStateJSON struct {
	ProgramID   int64     `json:"program_id"`
	MediaItemID int64     `json:"media_item_id"`
	Kind        string    `json:"kind"`
	GapLabel    string    `json:"gap_label,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

func (s *Server) handleChannelNow(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if _, err := s.channels.GetChannel(r.Context(), channelID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	state, err := s.channels.CurrentState(r.Context(), channelID, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	slotByID, err := s.slotsByID(r.Context(), channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := currentStateResponse{ChannelID: channelID}
	if state.Current != nil {
		resp.Current = &programStateJSON{
			ProgramID:   state.Current.ProgramID,
			MediaItemID: state.Current.MediaItemID,
			StartTime:   state.Current.StartTime,
			EndTime:     state.Current.EndTime(),
		}
		if slot, ok := slotByID[state.Current.ProgramID]; ok {
			resp.Current.Kind = slot.Kind
			resp.Current.GapLabel = slot.GapLabel
		}
		resp.OffsetSec = state.Offset.Seconds()
	}
	if state.Next != nil {
		resp.Next = &programStateJSON{
			ProgramID:   state.Next.ProgramID,
			MediaItemID: state.Next.MediaItemID,
			StartTime:   state.Next.StartTime,
			EndTime:     state.Next.EndTime(),
		}
		if slot, ok := slotByID[state.Next.ProgramID]; ok {
			resp.Next.Kind = slot.Kind
			resp.Next.GapLabel = slot.GapLabel
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
