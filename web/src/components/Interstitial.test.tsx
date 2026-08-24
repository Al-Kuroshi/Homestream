import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Interstitial } from "./Interstitial";

describe("Interstitial", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("shows the off_air heading and a next-up countdown", () => {
    render(
      <Interstitial reason="off_air" next={{ title: "Movie B", startTime: new Date("2026-01-01T18:00:30Z") }} />
    );
    expect(screen.getByText("Nothing scheduled right now")).toBeInTheDocument();
    expect(screen.getByText(/Up next: Movie B/)).toBeInTheDocument();
    expect(screen.getByText(/starts in 0:00:30/)).toBeInTheDocument();
  });

  it("shows the unavailable heading and no-next fallback", () => {
    render(<Interstitial reason="unavailable" next={null} />);
    expect(screen.getByText("Currently unavailable")).toBeInTheDocument();
    expect(screen.getByText("Nothing else scheduled on this channel.")).toBeInTheDocument();
  });

  it("ticks the countdown down every second", () => {
    render(
      <Interstitial reason="off_air" next={{ title: "Movie B", startTime: new Date("2026-01-01T18:00:30Z") }} />
    );
    expect(screen.getByText(/starts in 0:00:30/)).toBeInTheDocument();

    act(() => vi.advanceTimersByTime(5000));
    expect(screen.getByText(/starts in 0:00:25/)).toBeInTheDocument();
  });
});
