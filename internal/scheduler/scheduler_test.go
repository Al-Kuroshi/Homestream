package scheduler_test

import (
	"testing"
	"time"

	"personaltv/internal/scheduler"
)

func mustParse(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", value, err)
	}
	return parsed
}

func TestEvaluate_MidProgram(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 2*time.Hour + 5*time.Minute},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T20:05:00Z"), Duration: 30 * time.Minute},
	}
	now := mustParse(t, "2026-01-01T19:00:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current == nil || state.Current.ProgramID != 1 {
		t.Fatalf("expected current program 1, got %+v", state.Current)
	}
	if state.Offset != time.Hour {
		t.Errorf("expected offset 1h, got %v", state.Offset)
	}
	if state.Next == nil || state.Next.ProgramID != 2 {
		t.Fatalf("expected next program 2, got %+v", state.Next)
	}
}

func TestEvaluate_Gap(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T20:00:00Z"), Duration: 1 * time.Hour},
	}
	now := mustParse(t, "2026-01-01T19:30:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current != nil {
		t.Fatalf("expected no current program during gap, got %+v", state.Current)
	}
	if state.Next == nil || state.Next.ProgramID != 2 {
		t.Fatalf("expected next program 2, got %+v", state.Next)
	}
}

func TestEvaluate_ExactBoundaries(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
		{ProgramID: 2, StartTime: mustParse(t, "2026-01-01T19:00:00Z"), Duration: 1 * time.Hour},
	}

	atStart := scheduler.Evaluate(programs, mustParse(t, "2026-01-01T19:00:00Z"))
	if atStart.Current == nil || atStart.Current.ProgramID != 2 {
		t.Fatalf("expected program 2 to be current exactly at its start time, got %+v", atStart.Current)
	}

	justBeforeEnd := scheduler.Evaluate(programs, mustParse(t, "2026-01-01T18:59:59Z"))
	if justBeforeEnd.Current == nil || justBeforeEnd.Current.ProgramID != 1 {
		t.Fatalf("expected program 1 to still be current just before its end, got %+v", justBeforeEnd.Current)
	}
}

func TestEvaluate_NothingScheduledAfterNow(t *testing.T) {
	programs := []scheduler.ScheduledProgram{
		{ProgramID: 1, StartTime: mustParse(t, "2026-01-01T18:00:00Z"), Duration: 1 * time.Hour},
	}
	now := mustParse(t, "2026-01-01T19:30:00Z")

	state := scheduler.Evaluate(programs, now)

	if state.Current != nil {
		t.Fatalf("expected no current program, got %+v", state.Current)
	}
	if state.Next != nil {
		t.Fatalf("expected no next program, got %+v", state.Next)
	}
}
