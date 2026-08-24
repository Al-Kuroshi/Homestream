import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { VideoPlayer } from "./VideoPlayer";

const hlsInstances: Array<{
  loadSource: ReturnType<typeof vi.fn>;
  attachMedia: ReturnType<typeof vi.fn>;
  destroy: ReturnType<typeof vi.fn>;
  on: ReturnType<typeof vi.fn>;
}> = [];

vi.mock("hls.js", () => {
  class MockHls {
    static Events = { ERROR: "hlsError" };
    static isSupported = vi.fn(() => true);
    loadSource = vi.fn();
    attachMedia = vi.fn();
    destroy = vi.fn();
    on = vi.fn();
    constructor() {
      hlsInstances.push(this as unknown as (typeof hlsInstances)[number]);
    }
  }
  return { default: MockHls };
});

describe("VideoPlayer", () => {
  beforeEach(() => {
    hlsInstances.length = 0;
    window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    window.HTMLMediaElement.prototype.load = vi.fn();
    window.HTMLMediaElement.prototype.canPlayType = vi.fn().mockReturnValue("");
  });

  it("sets the video src directly and applies offsetSec on loadedmetadata for direct mode", () => {
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" offsetSec={42} onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    expect(video.src).toContain("/api/media/5/stream");

    fireEvent.loadedMetadata(video);
    expect(video.currentTime).toBe(42);
    expect(video.play).toHaveBeenCalled();
  });

  it("constructs hls.js and loads the playlist for hls mode without native support", () => {
    render(<VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" onError={vi.fn()} />);
    expect(hlsInstances).toHaveLength(1);
    expect(hlsInstances[0].loadSource).toHaveBeenCalledWith("/api/playback/sessions/abc/playlist.m3u8");
    expect(hlsInstances[0].attachMedia).toHaveBeenCalled();
  });

  it("never applies offsetSec to the video element in hls mode", () => {
    render(<VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" offsetSec={99} onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    fireEvent.loadedMetadata(video);
    expect(video.currentTime).toBe(0);
  });

  it("calls onError when the video element errors", () => {
    const onError = vi.fn();
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={onError} />);
    fireEvent.error(screen.getByTestId("video-el"));
    expect(onError).toHaveBeenCalled();
  });

  it("reports currentTime via onTimeUpdate as the video plays", () => {
    const onTimeUpdate = vi.fn();
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={vi.fn()} onTimeUpdate={onTimeUpdate} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    video.currentTime = 12.5;
    fireEvent.timeUpdate(video);
    expect(onTimeUpdate).toHaveBeenCalledWith(12.5);
  });

  it("shows a tap-to-play button when autoplay is blocked, and retries play on click", async () => {
    window.HTMLMediaElement.prototype.play = vi.fn().mockRejectedValue(new Error("blocked"));
    render(<VideoPlayer mode="direct" src="/api/media/5/stream" onError={vi.fn()} />);
    const video = screen.getByTestId("video-el") as HTMLVideoElement;
    fireEvent.loadedMetadata(video);

    const button = await screen.findByText("▶ Tap to play");
    expect(button).toBeInTheDocument();

    window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    fireEvent.click(button);
    expect(screen.queryByText("▶ Tap to play")).not.toBeInTheDocument();
  });

  it("does not reset the video src or restart hls.js when only the onError/onTimeUpdate callback identities change", () => {
    const { rerender } = render(
      <VideoPlayer mode="hls" src="/api/playback/sessions/abc/playlist.m3u8" onError={() => {}} />
    );
    expect(hlsInstances).toHaveLength(1);

    rerender(
      <VideoPlayer
        mode="hls"
        src="/api/playback/sessions/abc/playlist.m3u8"
        onError={() => {}}
        onTimeUpdate={() => {}}
      />
    );
    expect(hlsInstances).toHaveLength(1);
  });
});
