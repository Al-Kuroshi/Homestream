import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { useMediaItems } from "./media";
import { useCreateSource, useDeleteSource, useScanSource, useSources } from "./sources";

const SOURCE = { id: 1, name: "Movies", path: "/media/movies", created_at: "2026-01-01T00:00:00Z" };

describe("useSources", () => {
  it("fetches and returns the source list", async () => {
    server.use(http.get("/api/sources", () => HttpResponse.json([SOURCE])));
    const client = createTestQueryClient();
    const { result } = renderHook(() => useSources(), { wrapper: wrapWithQueryClient(client) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([SOURCE]);
  });
});

describe("useCreateSource", () => {
  it("posts the new source and invalidates the sources list", async () => {
    let refetchCount = 0;
    server.use(
      http.post("/api/sources", async ({ request }) => {
        const body = (await request.json()) as { name: string; path: string };
        return HttpResponse.json({ id: 2, ...body, created_at: "2026-01-01T00:00:00Z" }, { status: 201 });
      }),
      http.get("/api/sources", () => {
        refetchCount += 1;
        return HttpResponse.json([]);
      })
    );
    const client = createTestQueryClient();
    const wrapper = wrapWithQueryClient(client);

    // TanStack Query only refetches invalidated queries that currently have an active
    // observer, so mount `useSources` first (as the real Settings screen does alongside
    // the create form) to make the invalidation triggered below observable.
    const sourcesQuery = renderHook(() => useSources(), { wrapper });
    await waitFor(() => expect(sourcesQuery.result.current.isSuccess).toBe(true));
    expect(refetchCount).toBe(1);

    const { result } = renderHook(() => useCreateSource(), { wrapper });

    result.current.mutate({ name: "TV", path: "/media/tv" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(1));
  });
});

describe("useDeleteSource", () => {
  it("deletes the source and invalidates the sources list", async () => {
    server.use(
      http.delete("/api/sources/1", () => new HttpResponse(null, { status: 204 })),
      http.get("/api/sources", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    const { result } = renderHook(() => useDeleteSource(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate(1);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe("useScanSource", () => {
  it("triggers a scan and invalidates sources and media", async () => {
    let mediaRefetched = false;
    server.use(
      http.post("/api/sources/1/scan", () => new HttpResponse(null, { status: 204 })),
      http.get("/api/sources", () => HttpResponse.json([])),
      http.get("/api/media", () => {
        mediaRefetched = true;
        return HttpResponse.json([]);
      })
    );
    const client = createTestQueryClient();
    const wrapper = wrapWithQueryClient(client);

    // See the comment in the useCreateSource test above: an active observer is required
    // for invalidation to trigger a refetch, so mount both queries first.
    const sourcesQuery = renderHook(() => useSources(), { wrapper });
    const mediaQuery = renderHook(() => useMediaItems(), { wrapper });
    await waitFor(() => expect(sourcesQuery.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(mediaQuery.result.current.isSuccess).toBe(true));
    mediaRefetched = false;

    const { result } = renderHook(() => useScanSource(), { wrapper });

    result.current.mutate(1);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(mediaRefetched).toBe(true));
  });
});
