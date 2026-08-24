import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NowPlayingOverlay } from "./NowPlayingOverlay";

describe("NowPlayingOverlay", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows the title, progress, and next-up text", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle="Movie B" />);
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Next: Movie B")).toBeInTheDocument();
  });

  it("omits the next-up line when there is no next program", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    expect(screen.queryByText(/^Next:/)).not.toBeInTheDocument();
  });

  it("is visible on mount and hides after a period of inactivity", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).not.toHaveClass(
      "now-playing-overlay-hidden"
    );

    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).toHaveClass("now-playing-overlay-hidden");
  });

  it("re-shows and resets the hide timer on mouse movement", () => {
    render(<NowPlayingOverlay title="Movie A" currentTimeSec={30} durationSec={120} nextTitle={null} />);
    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).toHaveClass("now-playing-overlay-hidden");

    act(() => {
      fireEvent.mouseMove(window);
    });
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).not.toHaveClass(
      "now-playing-overlay-hidden"
    );
  });
});
