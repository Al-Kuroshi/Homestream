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

type programStateJSON struct {
	ProgramID   int64     `json:"program_id"`
	MediaItemID int64     `json:"media_item_id"`
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

	resp := currentStateResponse{ChannelID: channelID}
	if state.Current != nil {
		resp.Current = &programStateJSON{
			ProgramID:   state.Current.ProgramID,
			MediaItemID: state.Current.MediaItemID,
			StartTime:   state.Current.StartTime,
			EndTime:     state.Current.EndTime(),
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
	}

	writeJSON(w, http.StatusOK, resp)
}
