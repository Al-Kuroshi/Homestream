import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { useMediaItems } from "./media";

describe("useMediaItems", () => {
  it("fetches and returns the media list", async () => {
    const item = {
      id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3725,
      video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
      mod_time: "2026-01-01T00:00:00Z", invalid: false,
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    server.use(http.get("/api/media", () => HttpResponse.json([item])));
    const client = createTestQueryClient();
    const { result } = renderHook(() => useMediaItems(), { wrapper: wrapWithQueryClient(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([item]);
  });
});
