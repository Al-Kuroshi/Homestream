import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { useChannel, useChannels, useCreateChannel, useDeleteChannel, useUpdateChannel } from "./channels";

const CHANNEL = {
  id: 1, name: "Movies", description: "", enabled: true, position: 0,
  created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
};

describe("useChannels", () => {
  it("fetches and returns the channel list", async () => {
    server.use(http.get("/api/channels", () => HttpResponse.json([CHANNEL])));
    const client = createTestQueryClient();
    const { result } = renderHook(() => useChannels(), { wrapper: wrapWithQueryClient(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([CHANNEL]);
  });
});

describe("useChannel", () => {
  it("fetches a single channel by id", async () => {
    server.use(http.get("/api/channels/1", () => HttpResponse.json(CHANNEL)));
    const client = createTestQueryClient();
    const { result } = renderHook(() => useChannel(1), { wrapper: wrapWithQueryClient(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(CHANNEL);
  });

  it("does not fetch when id is 0", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useChannel(0), { wrapper: wrapWithQueryClient(client) });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useCreateChannel", () => {
  it("posts the new channel and invalidates the channel list", async () => {
    let refetchCount = 0;
    server.use(
      http.post("/api/channels", async ({ request }) => {
        const body = (await request.json()) as { name: string };
        return HttpResponse.json({ ...CHANNEL, id: 2, name: body.name }, { status: 201 });
      }),
      http.get("/api/channels", () => {
        refetchCount += 1;
        return HttpResponse.json([]);
      })
    );
    const client = createTestQueryClient();
    const wrapper = wrapWithQueryClient(client);

    // TanStack Query only refetches invalidated queries that currently have an active
    // observer, so mount `useChannels` first (as the real Channels screen does alongside
    // the create form) to make the invalidation triggered below observable.
    const channelsQuery = renderHook(() => useChannels(), { wrapper });
    await waitFor(() => expect(channelsQuery.result.current.isSuccess).toBe(true));
    expect(refetchCount).toBe(1);

    const { result } = renderHook(() => useCreateChannel(), { wrapper });

    result.current.mutate({ name: "Sitcoms" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(1));
  });
});

describe("useUpdateChannel", () => {
  it("puts the updated channel", async () => {
    server.use(
      http.put("/api/channels/1", async ({ request }) => {
        const body = await request.json();
        return HttpResponse.json({ ...CHANNEL, ...(body as object) });
      }),
      http.get("/api/channels", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    const { result } = renderHook(() => useUpdateChannel(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ id: 1, name: "Movies HD", description: "", enabled: false, position: 0 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe("useDeleteChannel", () => {
  it("deletes the channel", async () => {
    server.use(
      http.delete("/api/channels/1", () => new HttpResponse(null, { status: 204 })),
      http.get("/api/channels", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    const { result } = renderHook(() => useDeleteChannel(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate(1);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
