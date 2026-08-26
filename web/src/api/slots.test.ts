import { QueryClient } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { addSlot, listResolvedSlots, listSlots, useAddSlot, useResolvedSlots, useSlotsForChannel } from "./slots";

describe("slots API client", () => {
  it("listSlots fetches a channel's slots", async () => {
    server.use(http.get("/api/channels/1/slots", () => HttpResponse.json([{ id: 1 }])));
    const result = await listSlots(1);
    expect(result).toEqual([{ id: 1 }]);
  });

  it("listResolvedSlots fetches the resolved window with from/to query params", async () => {
    server.use(
      http.get("/api/channels/1/slots/resolved", ({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get("from")).toBe("2026-08-31T00:00:00.000Z");
        expect(url.searchParams.get("to")).toBe("2026-09-07T00:00:00.000Z");
        return HttpResponse.json([{ program_id: 1, media_item_id: 5, start_time: "2026-08-31T00:00:00Z", end_time: "2026-08-31T01:00:00Z" }]);
      })
    );
    const result = await listResolvedSlots(1, "2026-08-31T00:00:00.000Z", "2026-09-07T00:00:00.000Z");
    expect(result).toHaveLength(1);
  });

  it("addSlot posts the slot body without the channelId wrapper field", async () => {
    server.use(
      http.post("/api/channels/1/slots", async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>;
        expect(body).not.toHaveProperty("channelId");
        expect(body).toMatchObject({ kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
        return HttpResponse.json({ id: 1 }, { status: 201 });
      })
    );
    await addSlot({ channelId: 1, kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
  });

  it("useSlotsForChannel exposes the fetched slots", async () => {
    server.use(http.get("/api/channels/1/slots", () => HttpResponse.json([{ id: 1, channel_id: 1 }])));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useSlotsForChannel(1), { wrapper: wrapWithQueryClient(client) });
    await waitFor(() => expect(result.current.data).toEqual([{ id: 1, channel_id: 1 }]));
  });

  it("useResolvedSlots exposes the fetched resolved window", async () => {
    server.use(http.get("/api/channels/1/slots/resolved", () => HttpResponse.json([{ program_id: 1 }])));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useResolvedSlots(1, "2026-08-31T00:00:00.000Z", "2026-09-07T00:00:00.000Z"), {
      wrapper: wrapWithQueryClient(client),
    });
    await waitFor(() => expect(result.current.data).toEqual([{ program_id: 1 }]));
  });

  it("useAddSlot invalidates the channel's slots and resolved queries on success", async () => {
    server.use(http.post("/api/channels/1/slots", () => HttpResponse.json({ id: 1 }, { status: 201 })));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useAddSlot(1), { wrapper: wrapWithQueryClient(client) });
    result.current.mutate({ channelId: 1, kind: "media", media_item_id: 5, recurring: true, day_of_week: 1, position: 1000 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalled();
  });
});
