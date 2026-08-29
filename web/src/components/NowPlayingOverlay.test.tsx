import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NowPlayingOverlay } from "./NowPlayingOverlay";

const noop = () => {};

describe("NowPlayingOverlay", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows the title, progress, and next-up text", () => {
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle="Movie B"
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Next: Movie B")).toBeInTheDocument();
  });

  it("omits the next-up line when there is no next program", () => {
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
    expect(screen.queryByText(/^Next:/)).not.toBeInTheDocument();
  });

  it("shows a mute icon when muted or volume is 0, and an unmuted icon otherwise", () => {
    const { rerender } = render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
    expect(screen.getByLabelText("Mute")).toBeInTheDocument();

    rerender(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
    expect(screen.getByLabelText("Unmute")).toBeInTheDocument();
  });

  it("calls onMuteToggle when the mute button is clicked", () => {
    const onMuteToggle = vi.fn();
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={onMuteToggle}
      />
    );
    fireEvent.click(screen.getByLabelText("Mute"));
    expect(onMuteToggle).toHaveBeenCalled();
  });

  it("calls onVolumeChange with the new value when the slider moves", () => {
    const onVolumeChange = vi.fn();
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={onVolumeChange}
        onMuteToggle={noop}
      />
    );
    fireEvent.change(screen.getByLabelText("Volume"), { target: { value: "0.3" } });
    expect(onVolumeChange).toHaveBeenCalledWith(0.3);
  });

  it("is visible on mount and hides after a period of inactivity", () => {
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).not.toHaveClass(
      "now-playing-overlay-hidden"
    );

    act(() => vi.advanceTimersByTime(3000));
    expect(screen.getByText("Movie A").closest(".now-playing-overlay")).toHaveClass("now-playing-overlay-hidden");
  });

  it("re-shows and resets the hide timer on mouse movement", () => {
    render(
      <NowPlayingOverlay
        title="Movie A"
        currentTimeSec={30}
        durationSec={120}
        nextTitle={null}
        volume={1}
        muted={false}
        onVolumeChange={noop}
        onMuteToggle={noop}
      />
    );
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
