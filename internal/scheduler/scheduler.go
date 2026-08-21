package scheduler

import "time"

type ScheduledProgram struct {
	ProgramID   int64
	MediaItemID int64
	StartTime   time.Time
	Duration    time.Duration
}

func (p ScheduledProgram) EndTime() time.Time {
	return p.StartTime.Add(p.Duration)
}

type CurrentState struct {
	// Current is nil when the channel is "off air" — now falls in a gap
	// between two scheduled programs.
	Current *ScheduledProgram
	// Offset is only meaningful when Current != nil.
	Offset time.Duration
	// Next is nil when nothing is scheduled after now.
	Next *ScheduledProgram
}

// Evaluate determines what's playing on a channel (if anything) and what
// plays next, given its programs and a point in time. programs need not be
// sorted. Pure function: no I/O, safe to call from any goroutine.
func Evaluate(programs []ScheduledProgram, now time.Time) CurrentState {
	var current *ScheduledProgram
	var next *ScheduledProgram

	for i := range programs {
		p := programs[i]

		if !now.Before(p.StartTime) && now.Before(p.EndTime()) {
			if current == nil || p.StartTime.After(current.StartTime) {
				pCopy := p
				current = &pCopy
			}
		}

		if p.StartTime.After(now) {
			if next == nil || p.StartTime.Before(next.StartTime) {
				pCopy := p
				next = &pCopy
			}
		}
	}

	state := CurrentState{Current: current, Next: next}
	if current != nil {
		state.Offset = now.Sub(current.StartTime)
	}
	return state
}
