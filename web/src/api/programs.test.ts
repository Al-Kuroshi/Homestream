import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { useAddProgram, useDeleteProgram, useProgramsForChannel, useUpdateProgram } from "./programs";

const PROGRAM = {
  id: 1, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z",
  created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
};

describe("useProgramsForChannel", () => {
  it("fetches and returns a channel's programs", async () => {
    server.use(http.get("/api/channels/1/programs", () => HttpResponse.json([PROGRAM])));
    const client = createTestQueryClient();
    const { result } = renderHook(() => useProgramsForChannel(1), { wrapper: wrapWithQueryClient(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([PROGRAM]);
  });

  it("does not fetch when channelId is 0", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useProgramsForChannel(0), { wrapper: wrapWithQueryClient(client) });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useAddProgram", () => {
  it("posts a new program and invalidates that channel's programs", async () => {
    let refetchCount = 0;
    server.use(
      http.post("/api/channels/1/programs", async ({ request }) => {
        const body = (await request.json()) as { media_item_id: number; start_time: string };
        return HttpResponse.json({ ...PROGRAM, ...body }, { status: 201 });
      }),
      http.get("/api/channels/1/programs", () => {
        refetchCount += 1;
        return HttpResponse.json([]);
      })
    );
    const client = createTestQueryClient();
    const wrapper = wrapWithQueryClient(client);

    // TanStack Query only refetches invalidated queries that currently have an active
    // observer, so mount `useProgramsForChannel(1)` first (as the real channel schedule
    // screen does alongside the add-program form) to make the invalidation triggered
    // below observable.
    const programsQuery = renderHook(() => useProgramsForChannel(1), { wrapper });
    await waitFor(() => expect(programsQuery.result.current.isSuccess).toBe(true));
    expect(refetchCount).toBe(1);

    const { result } = renderHook(() => useAddProgram(1), { wrapper });

    result.current.mutate({ channelId: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(1));
  });
});

describe("useUpdateProgram", () => {
  it("puts the updated program", async () => {
    server.use(
      http.put("/api/programs/1", async ({ request }) => {
        const body = await request.json();
        return HttpResponse.json({ ...PROGRAM, ...(body as object) });
      }),
      http.get("/api/channels/1/programs", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    const { result } = renderHook(() => useUpdateProgram(1), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ id: 1, channelId: 1, media_item_id: 2, start_time: "2026-01-01T19:00:00Z" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe("useDeleteProgram", () => {
  it("deletes the program", async () => {
    server.use(
      http.delete("/api/programs/1", () => new HttpResponse(null, { status: 204 })),
      http.get("/api/channels/1/programs", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    const { result } = renderHook(() => useDeleteProgram(1), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ id: 1, channelId: 1 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
