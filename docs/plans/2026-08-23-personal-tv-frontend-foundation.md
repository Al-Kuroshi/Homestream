# Personal TV — Frontend Foundation & Management Screens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the React + TypeScript frontend SPA — app shell/navigation, Guide (EPG timeline grid), Media Library, Channels (list + schedule editor), and Settings (media sources) — fully working against the existing Go REST API, plus the `go:embed` wiring so the production Go binary serves the built SPA.

**Architecture:** A new `web/` directory (Vite + React + TypeScript) sits alongside the existing Go backend. A typed API client (`web/src/api/*`) wraps every REST endpoint this plan uses; TanStack Query owns all server state (fetching, caching, polling, mutation-triggered invalidation) on top of that client. React Router provides real per-screen URLs under a persistent sidebar shell. In production, `web/dist` (the Vite build output) is embedded into the Go binary via `go:embed` and served for any request that isn't `/api/*` or `/healthz`; in development, the Vite dev server proxies `/api/*` to the Go backend so both run side by side with no CORS configuration needed.

**Tech Stack:** Node.js 20+/npm, Vite, React 18+, TypeScript (strict), React Router, TanStack Query (React Query), plain CSS (no UI framework), Vitest + React Testing Library + `@testing-library/user-event`, MSW (Mock Service Worker) for network mocking in tests, `jest-dom` matchers.

**Spec:** `docs/design/2026-08-23-personal-tv-frontend-foundation-design.md` (and `docs/prd/HomeStreamer.md` for product requirements, `docs/design/2026-08-21-personal-tv-design.md` for the overall system design). This plan implements spec §2–§7. TV/player (spec §1, deferred) and any backend API changes (spec §8, explicitly out of scope) are not part of this plan — the REST API is used exactly as it exists today on `main` (verified against `internal/model/model.go` and `internal/api/*.go`).

## Global Constraints

- Node.js 20+ and npm are required to build/test this plan's code (parallel to the backend plan's `ffmpeg`/`ffprobe`-on-`PATH` requirement).
- All JSON field names are `snake_case`, exactly as tagged in `internal/model/model.go` (e.g. `source_id`, `duration_sec`, `media_item_id`, `start_time`, `created_at`) — never assume Go's default PascalCase field names. Timestamps are RFC3339 strings (backend writes them via `db.FormatTime`, always UTC).
- Every screen/component talks to the backend only through `web/src/api/*` — never a raw `fetch` call inside a screen or component. This is the frontend's enforcement of the "UI consumes the API" principle already stated in `CLAUDE.md`.
- No new backend endpoints and no changes to any file under `internal/` except `internal/api/router.go` (Task 11 only, additive — see that task).
- TypeScript strict mode stays on (the default from Vite's `react-ts` template — do not weaken it).
- Tests are colocated as `*.test.ts`/`*.test.tsx` next to the code they test, black-box where practical, network mocked via MSW — no test hits a real backend process. This matches `CLAUDE.md`'s Testing Conventions section, extended to the frontend.
- Off-air gap semantics (spec §4.1): a channel can have non-contiguous programs; the gap between `CurrentState.Current == nil` moments is a first-class "off air" state already implemented server-side (`internal/scheduler/scheduler.go`). The frontend never needs to call any scheduler-internal code — it derives gaps itself from each program's `start_time`/computed `end_time`, client-side, exactly as spec'd.
- Default Guide time window: 1 hour before now to 5 hours after now (spec §4.1) — a UI parameter, adjustable without revisiting the spec.

---

## Task 1: Project Scaffolding

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/tsconfig.app.json`, `web/tsconfig.node.json`, `web/vite.config.ts`, `web/index.html` (all via `npm create vite`, then adjusted)
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/App.css`
- Create: `web/src/index.css`
- Create: `web/src/test/setup.ts`
- Create: `web/src/test/server.ts`
- Test: `web/src/App.test.tsx`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: a working `npm run dev` / `npm run build` / `npm test` toolchain in `web/`, and the MSW test server (`web/src/test/server.ts`, a `setupServer()` instance with no default handlers) that **every later task's tests import and add handlers to via `server.use(...)`**. `App.tsx` here is a placeholder — **Task 4 replaces it wholesale** once routing/sidebar exist.

- [ ] **Step 1: Scaffold the Vite project**

```bash
cd /home/daslaptop/HomeStreamProject
npm create vite@latest web -- --template react-ts
cd web
npm install
```

- [ ] **Step 2: Install the extra dependencies this plan needs**

```bash
npm install --save-dev vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom msw
```

(`react-router-dom` and `@tanstack/react-query` are installed in the tasks that first use them — Task 4 and Task 2, respectively — not here, per YAGNI: don't install a dependency before the task that needs it.)

- [ ] **Step 3: Configure Vitest**

Replace `web/vite.config.ts` with:

```ts
/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: false,
  },
});
```

- [ ] **Step 4: Add the test scripts**

In `web/package.json`, add to `"scripts"`:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 5: Create the MSW test server and Vitest setup file**

`web/src/test/server.ts`:

```ts
import { setupServer } from "msw/node";

// No default handlers: every test adds exactly the handlers it needs via
// server.use(...), so a test can never accidentally pass because of another
// test's leftover mock.
export const server = setupServer();
```

`web/src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, beforeAll } from "vitest";
import { server } from "./server";

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

- [ ] **Step 6: Write the failing smoke test**

Replace `web/src/App.test.tsx` (delete the Vite template's default one if present) with:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { App } from "./App";

describe("App", () => {
  it("renders the app heading", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: "Personal TV" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 7: Run the test to verify it fails**

Run: `cd web && npm test`
Expected: FAIL — `App` either doesn't export the right thing or doesn't render "Personal TV" (the Vite template's default `App.tsx` renders a counter demo, not this heading).

- [ ] **Step 8: Replace the template's App/main/CSS with a minimal placeholder**

`web/src/App.tsx`:

```tsx
export function App() {
  return <h1>Personal TV</h1>;
}
```

`web/src/main.tsx`:

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

`web/src/App.css`: delete the Vite template's default content — leave the file empty (Task 4 populates it with real shell layout rules).

`web/src/index.css`: leave the Vite template's default (basic resets) in place — no changes needed.

Delete `web/src/assets/react.svg` and any other Vite-template demo assets that are no longer referenced (`App.tsx` no longer imports them).

- [ ] **Step 9: Run the test to verify it passes**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 10: Verify the dev server and production build both work**

```bash
cd web
npm run build
```

Expected: exits 0, produces `web/dist/`. (`npm run dev` is a long-running process — verify manually by starting it and confirming `http://localhost:5173` shows "Personal TV"; do not leave it running for the next step.)

- [ ] **Step 11: Add `web/dist`'s local nested ignore and commit**

`web/dist` is already covered by the repo root `.gitignore` (`/web/dist`) — no change needed there. (Task 11 introduces a tracked placeholder inside `web/dist` for `go:embed`; nothing to do here.)

```bash
cd /home/daslaptop/HomeStreamProject
git add web/package.json web/package-lock.json web/tsconfig*.json web/vite.config.ts web/index.html web/src web/public 2>/dev/null
git commit -m "chore: scaffold Vite + React + TypeScript frontend with Vitest/MSW"
```

---

## Task 2: API Client — HTTP Core, Sources, and Media

**Files:**
- Create: `web/src/api/types.ts`
- Create: `web/src/api/http.ts`
- Create: `web/src/api/sources.ts`
- Create: `web/src/api/media.ts`
- Create: `web/src/test/queryClient.tsx`
- Test: `web/src/api/http.test.ts`
- Test: `web/src/api/sources.test.ts`
- Test: `web/src/api/media.test.ts`

**Interfaces:**
- Consumes: `server` (MSW test server, Task 1).
- Produces:
  - `apiGet<T>(path)`, `apiPost<T>(path, body?)`, `apiPut<T>(path, body)`, `apiDelete(path)` in `web/src/api/http.ts`, and the `ApiError` class (`.status: number`, `.message: string`). **Every other API module (this task's `sources.ts`/`media.ts`, and Task 3's `channels.ts`/`programs.ts`) is built on these four functions — no module calls `fetch` directly.**
  - Types `MediaSource`, `MediaItem` in `web/src/api/types.ts` (Task 3 adds `Channel`/`Program` to the same file).
  - `listSources()`, `createSource(input)`, `deleteSource(id)`, `scanSource(id)` and hooks `useSources()`, `useCreateSource()`, `useDeleteSource()`, `useScanSource()` in `web/src/api/sources.ts`. **Task 6 (Settings screen) is the consumer.**
  - `listMedia()` and `useMediaItems()` in `web/src/api/media.ts`. **Tasks 5, 8, 9, 10 all consume `useMediaItems`.**
  - `createTestQueryClient()` and `wrapWithQueryClient(client)` in `web/src/test/queryClient.tsx` — **every task from here on that tests a TanStack Query hook or a component that uses one reuses this helper.**

- [ ] **Step 1: Install TanStack Query**

```bash
cd web
npm install @tanstack/react-query
```

- [ ] **Step 2: Write the failing tests for the HTTP core**

`web/src/api/http.test.ts`:

```ts
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../test/server";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";

describe("apiGet", () => {
  it("returns parsed JSON on success", async () => {
    server.use(http.get("/api/widgets", () => HttpResponse.json([{ id: 1 }])));
    const result = await apiGet<{ id: number }[]>("/widgets");
    expect(result).toEqual([{ id: 1 }]);
  });

  it("throws with the server's error message on failure", async () => {
    server.use(http.get("/api/widgets", () => HttpResponse.json({ error: "boom" }, { status: 500 })));
    await expect(apiGet("/widgets")).rejects.toMatchObject({ status: 500, message: "boom" });
  });
});

describe("apiPost", () => {
  it("sends a JSON body and returns the created resource", async () => {
    server.use(
      http.post("/api/widgets", async ({ request }) => {
        const body = await request.json();
        return HttpResponse.json({ id: 2, ...(body as object) }, { status: 201 });
      })
    );
    const result = await apiPost<{ id: number; name: string }>("/widgets", { name: "gadget" });
    expect(result).toEqual({ id: 2, name: "gadget" });
  });

  it("posts with no body when none is given", async () => {
    server.use(
      http.post("/api/widgets/1/scan", () => new HttpResponse(null, { status: 204 }))
    );
    await expect(apiPost<void>("/widgets/1/scan")).resolves.toBeUndefined();
  });
});

describe("apiPut", () => {
  it("sends a JSON body and returns the updated resource", async () => {
    server.use(
      http.put("/api/widgets/1", async ({ request }) => {
        const body = await request.json();
        return HttpResponse.json({ id: 1, ...(body as object) });
      })
    );
    const result = await apiPut<{ id: number; name: string }>("/widgets/1", { name: "renamed" });
    expect(result).toEqual({ id: 1, name: "renamed" });
  });
});

describe("apiDelete", () => {
  it("resolves with no value on a 204", async () => {
    server.use(http.delete("/api/widgets/1", () => new HttpResponse(null, { status: 204 })));
    await expect(apiDelete("/widgets/1")).resolves.toBeUndefined();
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd web && npm test -- src/api/http.test.ts`
Expected: FAIL — `./http` doesn't exist yet.

- [ ] **Step 4: Write the HTTP core implementation**

`web/src/api/http.ts`:

```ts
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const BASE = "/api";

async function handle<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return undefined as T;
  }
  const body = await res.json().catch(() => undefined);
  if (!res.ok) {
    const message = (body as { error?: string } | undefined)?.error ?? res.statusText;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  return handle<T>(res);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return handle<T>(res);
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return handle<T>(res);
}

export async function apiDelete(path: string): Promise<void> {
  const res = await fetch(`${BASE}${path}`, { method: "DELETE" });
  await handle<void>(res);
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npm test -- src/api/http.test.ts`
Expected: PASS

- [ ] **Step 6: Write the shared types (Sources/Media half)**

`web/src/api/types.ts`:

```ts
export interface MediaSource {
  id: number;
  name: string;
  path: string;
  created_at: string;
}

export interface MediaItem {
  id: number;
  source_id: number;
  rel_path: string;
  title: string;
  duration_sec: number;
  video_codec: string;
  audio_codec: string;
  container: string;
  size_bytes: number;
  mod_time: string;
  invalid: boolean;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 7: Write the shared TanStack Query test helper**

`web/src/test/queryClient.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

export function wrapWithQueryClient(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}
```

- [ ] **Step 8: Write the failing tests for Sources and Media**

`web/src/api/sources.test.ts`:

```ts
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
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
    const { result } = renderHook(() => useCreateSource(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ name: "TV", path: "/media/tv" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(0));
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
    const { result } = renderHook(() => useScanSource(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate(1);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(mediaRefetched).toBe(true));
  });
});
```

`web/src/api/media.test.ts`:

```ts
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
```

- [ ] **Step 9: Run the tests to verify they fail**

Run: `cd web && npm test -- src/api/sources.test.ts src/api/media.test.ts`
Expected: FAIL — `./sources` and `./media` don't exist yet.

- [ ] **Step 10: Write the Sources and Media API modules**

`web/src/api/sources.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost } from "./http";
import type { MediaSource } from "./types";

export function listSources(): Promise<MediaSource[]> {
  return apiGet<MediaSource[]>("/sources");
}

export interface CreateSourceInput {
  name: string;
  path: string;
}

export function createSource(input: CreateSourceInput): Promise<MediaSource> {
  return apiPost<MediaSource>("/sources", input);
}

export function deleteSource(id: number): Promise<void> {
  return apiDelete(`/sources/${id}`);
}

export function scanSource(id: number): Promise<void> {
  return apiPost<void>(`/sources/${id}/scan`);
}

const sourcesKey = ["sources"] as const;
const mediaKey = ["media"] as const;

export function useSources() {
  return useQuery({ queryKey: sourcesKey, queryFn: listSources });
}

export function useCreateSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createSource,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function useDeleteSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteSource,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function useScanSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: scanSource,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sourcesKey });
      queryClient.invalidateQueries({ queryKey: mediaKey });
    },
  });
}
```

`web/src/api/media.ts`:

```ts
import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import type { MediaItem } from "./types";

export function listMedia(): Promise<MediaItem[]> {
  return apiGet<MediaItem[]>("/media");
}

export function useMediaItems() {
  return useQuery({ queryKey: ["media"], queryFn: listMedia });
}
```

- [ ] **Step 11: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS (all tests, including Task 1's)

- [ ] **Step 12: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/package.json web/package-lock.json web/src/api web/src/test
git commit -m "feat: add API client HTTP core plus sources/media modules"
```

---

## Task 3: API Client — Channels and Programs

**Files:**
- Modify: `web/src/api/types.ts` (add `Channel`, `Program`)
- Create: `web/src/api/channels.ts`
- Create: `web/src/api/programs.ts`
- Test: `web/src/api/channels.test.ts`
- Test: `web/src/api/programs.test.ts`

**Interfaces:**
- Consumes: `apiGet`/`apiPost`/`apiPut`/`apiDelete` (Task 2's `http.ts`), `createTestQueryClient`/`wrapWithQueryClient` (Task 2's `test/queryClient.tsx`), `server` (Task 1).
- Produces:
  - Types `Channel`, `Program` in `web/src/api/types.ts`.
  - `listChannels()`, `getChannel(id)`, `createChannel(input)`, `updateChannel(input)`, `deleteChannel(id)` and hooks `useChannels()`, `useChannel(id)`, `useCreateChannel()`, `useUpdateChannel()`, `useDeleteChannel()` in `web/src/api/channels.ts`. **Tasks 7, 8, 9, 10 consume these.**
  - `listPrograms(channelId)`, `addProgram(input)`, `updateProgram(input)`, `deleteProgram(input)` and hooks `useProgramsForChannel(channelId)`, `useAddProgram(channelId)`, `useUpdateProgram(channelId)`, `useDeleteProgram(channelId)` in `web/src/api/programs.ts`. **Tasks 8, 9, 10 consume these.**

- [ ] **Step 1: Add the Channel/Program types**

Append to `web/src/api/types.ts`:

```ts
export interface Channel {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface Program {
  id: number;
  channel_id: number;
  media_item_id: number;
  start_time: string;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 2: Write the failing tests for Channels**

`web/src/api/channels.test.ts`:

```ts
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
    const { result } = renderHook(() => useCreateChannel(), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ name: "Sitcoms" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(0));
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
```

- [ ] **Step 3: Write the failing tests for Programs**

`web/src/api/programs.test.ts`:

```ts
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
    const { result } = renderHook(() => useAddProgram(1), { wrapper: wrapWithQueryClient(client) });

    result.current.mutate({ channelId: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(refetchCount).toBeGreaterThan(0));
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
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `cd web && npm test -- src/api/channels.test.ts src/api/programs.test.ts`
Expected: FAIL — `./channels` and `./programs` don't exist yet.

- [ ] **Step 5: Write the Channels API module**

`web/src/api/channels.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { Channel } from "./types";

export function listChannels(): Promise<Channel[]> {
  return apiGet<Channel[]>("/channels");
}

export function getChannel(id: number): Promise<Channel> {
  return apiGet<Channel>(`/channels/${id}`);
}

export interface CreateChannelInput {
  name: string;
  description?: string;
  position?: number;
}

export function createChannel(input: CreateChannelInput): Promise<Channel> {
  return apiPost<Channel>("/channels", input);
}

export interface UpdateChannelInput {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  position: number;
}

export function updateChannel(input: UpdateChannelInput): Promise<Channel> {
  return apiPut<Channel>(`/channels/${input.id}`, {
    name: input.name,
    description: input.description,
    enabled: input.enabled,
    position: input.position,
  });
}

export function deleteChannel(id: number): Promise<void> {
  return apiDelete(`/channels/${id}`);
}

const channelsKey = ["channels"] as const;

export function useChannels() {
  return useQuery({ queryKey: channelsKey, queryFn: listChannels });
}

export function useChannel(id: number) {
  return useQuery({
    queryKey: [...channelsKey, id],
    queryFn: () => getChannel(id),
    enabled: id > 0,
  });
}

export function useCreateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}

export function useUpdateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}
```

- [ ] **Step 6: Write the Programs API module**

`web/src/api/programs.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { Program } from "./types";

export function listPrograms(channelId: number): Promise<Program[]> {
  return apiGet<Program[]>(`/channels/${channelId}/programs`);
}

export interface AddProgramInput {
  channelId: number;
  media_item_id: number;
  start_time: string;
}

export function addProgram(input: AddProgramInput): Promise<Program> {
  return apiPost<Program>(`/channels/${input.channelId}/programs`, {
    media_item_id: input.media_item_id,
    start_time: input.start_time,
  });
}

export interface UpdateProgramInput {
  id: number;
  channelId: number;
  media_item_id: number;
  start_time: string;
}

export function updateProgram(input: UpdateProgramInput): Promise<Program> {
  return apiPut<Program>(`/programs/${input.id}`, {
    media_item_id: input.media_item_id,
    start_time: input.start_time,
  });
}

export interface DeleteProgramInput {
  id: number;
  channelId: number;
}

export function deleteProgram(input: DeleteProgramInput): Promise<void> {
  return apiDelete(`/programs/${input.id}`);
}

function programsKey(channelId: number) {
  return ["channels", channelId, "programs"] as const;
}

export function useProgramsForChannel(channelId: number) {
  return useQuery({
    queryKey: programsKey(channelId),
    queryFn: () => listPrograms(channelId),
    enabled: channelId > 0,
  });
}

export function useAddProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: addProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}

export function useUpdateProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}

export function useDeleteProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/api
git commit -m "feat: add API client channels/programs modules"
```

---

## Task 4: App Shell — Sidebar Navigation and Routing

**Files:**
- Modify: `web/src/App.tsx` (Task 1's placeholder, replaced wholesale)
- Modify: `web/src/App.test.tsx` (replaced wholesale)
- Modify: `web/src/App.css`
- Create: `web/src/AppRoutes.tsx`
- Test: `web/src/AppRoutes.test.tsx`
- Create: `web/src/components/Sidebar.tsx`
- Create: `web/src/components/Sidebar.css`
- Test: `web/src/components/Sidebar.test.tsx`
- Create: `web/src/components/ErrorBoundary.tsx`
- Test: `web/src/components/ErrorBoundary.test.tsx`
- Create: `web/src/screens/GuideScreen.tsx` (placeholder — **Task 11 replaces wholesale**)
- Create: `web/src/screens/LibraryScreen.tsx` (placeholder — **Task 5 replaces wholesale**)
- Create: `web/src/screens/ChannelsListScreen.tsx` (placeholder — **Task 7 replaces wholesale**)
- Create: `web/src/screens/ChannelScheduleScreen.tsx` (placeholder — **Task 9 replaces wholesale**)
- Create: `web/src/screens/SettingsScreen.tsx` (placeholder — **Task 6 replaces wholesale**)

**Interfaces:**
- Consumes: nothing from the API client yet (this task is pure shell/routing; the screens it mounts are placeholders).
- Produces: `AppRoutes` (router-agnostic route tree, importable directly by tests via `MemoryRouter`), `Sidebar` (nav component), and `ErrorBoundary` (spec §6's route-level last-resort safety net — wraps `AppRoutes` inside `App`; no other task needs to import it directly). Every placeholder screen exports a component named exactly `GuideScreen`, `LibraryScreen`, `ChannelsListScreen`, `ChannelScheduleScreen`, `SettingsScreen` respectively — **Tasks 5, 6, 7, 9, and 11 keep these exact export names when they replace the file bodies**, and each keeps a top-level heading with the same visible text (`"Guide"`, `"Library"`, `"Channels"`, `"Settings"`) so this task's `AppRoutes` test keeps passing after those replacements. `ChannelScheduleScreen`'s placeholder heading is not asserted by this task's tests (its real heading becomes dynamic — the channel's name — once Task 9 replaces it).

- [ ] **Step 1: Install React Router**

```bash
cd web
npm install react-router-dom
```

- [ ] **Step 2: Write the failing placeholder screens**

`web/src/screens/GuideScreen.tsx`:

```tsx
export function GuideScreen() {
  return <h1>Guide</h1>;
}
```

`web/src/screens/LibraryScreen.tsx`:

```tsx
export function LibraryScreen() {
  return <h1>Library</h1>;
}
```

`web/src/screens/ChannelsListScreen.tsx`:

```tsx
export function ChannelsListScreen() {
  return <h1>Channels</h1>;
}
```

`web/src/screens/ChannelScheduleScreen.tsx`:

```tsx
export function ChannelScheduleScreen() {
  return <h1>Channel Schedule</h1>;
}
```

`web/src/screens/SettingsScreen.tsx`:

```tsx
export function SettingsScreen() {
  return <h1>Settings</h1>;
}
```

(These aren't TDD'd individually — they're one-line placeholders with no behavior to test yet; Tasks 5–8/10 TDD their real implementations.)

- [ ] **Step 3: Write the failing tests for AppRoutes**

`web/src/AppRoutes.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppRoutes } from "./AppRoutes";

describe("AppRoutes", () => {
  it("redirects / to the Guide screen", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Guide" })).toBeInTheDocument();
  });

  it("renders the Library screen at /library", () => {
    render(
      <MemoryRouter initialEntries={["/library"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Library" })).toBeInTheDocument();
  });

  it("renders the Channels screen at /channels", () => {
    render(
      <MemoryRouter initialEntries={["/channels"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("renders the Settings screen at /settings", () => {
    render(
      <MemoryRouter initialEntries={["/settings"]}>
        <AppRoutes />
      </MemoryRouter>
    );
    expect(screen.getByRole("heading", { name: "Settings" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 4: Write the failing tests for Sidebar**

`web/src/components/Sidebar.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { Sidebar } from "./Sidebar";

describe("Sidebar", () => {
  it("renders a link for each of the four screens", () => {
    render(
      <MemoryRouter initialEntries={["/guide"]}>
        <Sidebar />
      </MemoryRouter>
    );
    for (const label of ["Guide", "Library", "Channels", "Settings"]) {
      expect(screen.getByRole("link", { name: label })).toBeInTheDocument();
    }
  });

  it("marks the link matching the current route as active", () => {
    render(
      <MemoryRouter initialEntries={["/library"]}>
        <Sidebar />
      </MemoryRouter>
    );
    expect(screen.getByRole("link", { name: "Library" })).toHaveClass("active");
    expect(screen.getByRole("link", { name: "Guide" })).not.toHaveClass("active");
  });
});
```

- [ ] **Step 5: Write the failing tests for ErrorBoundary**

`web/src/components/ErrorBoundary.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ErrorBoundary } from "./ErrorBoundary";

function Boom(): never {
  throw new Error("boom");
}

describe("ErrorBoundary", () => {
  it("renders its children when nothing throws", () => {
    render(
      <ErrorBoundary>
        <p>All good</p>
      </ErrorBoundary>
    );
    expect(screen.getByText("All good")).toBeInTheDocument();
  });

  it("catches a render error and shows a fallback instead of crashing the app", () => {
    // React logs caught errors to console.error even when a boundary
    // handles them; silence that expected noise for this one test.
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong: boom");
    consoleError.mockRestore();
  });
});
```

- [ ] **Step 6: Write the failing test for App**

Replace `web/src/App.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { App } from "./App";

describe("App", () => {
  it("renders the sidebar navigation and the routed content area", () => {
    render(<App />);
    expect(screen.getByRole("navigation", { name: "Main navigation" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Guide" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 7: Run the tests to verify they fail**

Run: `cd web && npm test`
Expected: FAIL — `./AppRoutes`, `./components/Sidebar`, and `./components/ErrorBoundary` don't exist yet; `App.test.tsx`'s new assertions fail against Task 1's placeholder `App`.

- [ ] **Step 8: Write the ErrorBoundary component**

`web/src/components/ErrorBoundary.tsx`:

```tsx
import { Component, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// The route-level last-resort safety net (spec §6): an unexpected render
// error anywhere under this boundary shows a fallback message instead of
// blanking the whole app. This is not the primary error-handling path —
// TanStack Query's isError/error states (handled per-screen) are — this
// only catches what those can't (a bug in render logic itself).
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    if (this.state.error) {
      return <p role="alert">Something went wrong: {this.state.error.message}</p>;
    }
    return this.props.children;
  }
}
```

- [ ] **Step 9: Write the Sidebar component**

`web/src/components/Sidebar.tsx`:

```tsx
import { NavLink } from "react-router-dom";
import "./Sidebar.css";

const NAV_ITEMS = [
  { to: "/guide", label: "Guide" },
  { to: "/library", label: "Library" },
  { to: "/channels", label: "Channels" },
  { to: "/settings", label: "Settings" },
] as const;

export function Sidebar() {
  return (
    <nav className="sidebar" aria-label="Main navigation">
      <div className="sidebar-title">Personal TV</div>
      <ul>
        {NAV_ITEMS.map((item) => (
          <li key={item.to}>
            <NavLink to={item.to} className={({ isActive }) => (isActive ? "active" : undefined)}>
              {item.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
```

`web/src/components/Sidebar.css`:

```css
.sidebar {
  width: 180px;
  flex-shrink: 0;
  background: #1a1a1a;
  color: #f5f5f5;
  padding: 16px;
}

.sidebar-title {
  font-weight: 600;
  margin-bottom: 16px;
}

.sidebar ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sidebar a {
  display: block;
  padding: 8px 10px;
  border-radius: 4px;
  color: inherit;
  text-decoration: none;
}

.sidebar a.active {
  background: rgba(255, 255, 255, 0.15);
}
```

- [ ] **Step 10: Write AppRoutes**

`web/src/AppRoutes.tsx`:

```tsx
import { Navigate, Route, Routes } from "react-router-dom";
import { ChannelScheduleScreen } from "./screens/ChannelScheduleScreen";
import { ChannelsListScreen } from "./screens/ChannelsListScreen";
import { GuideScreen } from "./screens/GuideScreen";
import { LibraryScreen } from "./screens/LibraryScreen";
import { SettingsScreen } from "./screens/SettingsScreen";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/guide" replace />} />
      <Route path="/guide" element={<GuideScreen />} />
      <Route path="/library" element={<LibraryScreen />} />
      <Route path="/channels" element={<ChannelsListScreen />} />
      <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      <Route path="/settings" element={<SettingsScreen />} />
    </Routes>
  );
}
```

- [ ] **Step 11: Write the new App**

Replace `web/src/App.tsx`:

```tsx
import { BrowserRouter } from "react-router-dom";
import { AppRoutes } from "./AppRoutes";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Sidebar } from "./components/Sidebar";
import "./App.css";

export function App() {
  return (
    <BrowserRouter>
      <div className="app-shell">
        <Sidebar />
        <main className="app-content">
          <ErrorBoundary>
            <AppRoutes />
          </ErrorBoundary>
        </main>
      </div>
    </BrowserRouter>
  );
}
```

Replace `web/src/App.css`:

```css
.app-shell {
  display: flex;
  min-height: 100vh;
}

.app-content {
  flex: 1;
  padding: 24px;
  overflow-x: auto;
}
```

- [ ] **Step 12: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 13: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/package.json web/package-lock.json web/src/App.tsx web/src/App.test.tsx web/src/App.css web/src/AppRoutes.tsx web/src/AppRoutes.test.tsx web/src/components web/src/screens
git commit -m "feat: add app shell with sidebar navigation, routing, and an error boundary"
```

---

## Task 5: Media Library Screen

**Files:**
- Modify: `web/src/screens/LibraryScreen.tsx` (Task 4's placeholder, replaced wholesale)
- Create: `web/src/screens/LibraryScreen.css`
- Test: `web/src/screens/LibraryScreen.test.tsx`

**Interfaces:**
- Consumes: `useMediaItems()` (Task 2's `api/media.ts`), `useSources()` (Task 2's `api/sources.ts`), `createTestQueryClient`/`wrapWithQueryClient` (Task 2), `server` (Task 1).
- Produces: `LibraryScreen` (same export name and `<h1>Library</h1>` heading as Task 4's placeholder — `AppRoutes.test.tsx` keeps passing unmodified).

- [ ] **Step 1: Write the failing tests**

`web/src/screens/LibraryScreen.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { LibraryScreen } from "./LibraryScreen";

const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3725,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: 2, source_id: 1, rel_path: "b.mp4", title: "Broken B", duration_sec: 0,
    video_codec: "", audio_codec: "", container: "", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: true,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
const SOURCES = [{ id: 1, name: "Movies", path: "/media/movies", created_at: "2026-01-01T00:00:00Z" }];

function renderScreen() {
  server.use(
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/sources", () => HttpResponse.json(SOURCES))
  );
  const client = createTestQueryClient();
  render(<LibraryScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("LibraryScreen", () => {
  it("renders every media item as a row, with duration formatted h:mm:ss", async () => {
    renderScreen();
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Broken B")).toBeInTheDocument();
    expect(screen.getByText("1:02:05")).toBeInTheDocument();
  });

  it("shows the owning source's name and an Invalid status for broken items", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    expect(screen.getAllByText("Movies")).toHaveLength(2);
    expect(screen.getByText("Invalid")).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("filters by search text", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.type(screen.getByLabelText("Search titles"), "Movie A");
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.queryByText("Broken B")).not.toBeInTheDocument();
  });

  it("filters to invalid-only items", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.click(screen.getByLabelText("Invalid only"));
    expect(screen.queryByText("Movie A")).not.toBeInTheDocument();
    expect(screen.getByText("Broken B")).toBeInTheDocument();
  });

  it("shows an empty-state message when no media matches the filters", async () => {
    renderScreen();
    await screen.findByText("Movie A");
    await userEvent.type(screen.getByLabelText("Search titles"), "nothing matches this");
    expect(screen.getByText("No media matches the current filters.")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/screens/LibraryScreen.test.tsx`
Expected: FAIL — the placeholder `LibraryScreen` renders none of this.

- [ ] **Step 3: Write the implementation**

`web/src/screens/LibraryScreen.tsx`:

```tsx
import { useMemo, useState } from "react";
import { useMediaItems } from "../api/media";
import { useSources } from "../api/sources";
import "./LibraryScreen.css";

export function LibraryScreen() {
  const { data: items, isLoading, isError } = useMediaItems();
  const { data: sources } = useSources();
  const [search, setSearch] = useState("");
  const [sourceFilter, setSourceFilter] = useState<number | "all">("all");
  const [invalidOnly, setInvalidOnly] = useState(false);

  const sourceNameById = useMemo(() => {
    const map = new Map<number, string>();
    for (const s of sources ?? []) map.set(s.id, s.name);
    return map;
  }, [sources]);

  const filtered = useMemo(() => {
    return (items ?? []).filter((item) => {
      if (search && !item.title.toLowerCase().includes(search.toLowerCase())) return false;
      if (sourceFilter !== "all" && item.source_id !== sourceFilter) return false;
      if (invalidOnly && !item.invalid) return false;
      return true;
    });
  }, [items, search, sourceFilter, invalidOnly]);

  if (isLoading) return <p>Loading media…</p>;
  if (isError) return <p role="alert">Failed to load media library.</p>;

  return (
    <section>
      <h1>Library</h1>
      <div className="library-filters">
        <input
          type="search"
          placeholder="Search titles…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search titles"
        />
        <select
          value={sourceFilter}
          onChange={(e) => setSourceFilter(e.target.value === "all" ? "all" : Number(e.target.value))}
          aria-label="Filter by source"
        >
          <option value="all">All sources</option>
          {(sources ?? []).map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
          ))}
        </select>
        <label>
          <input type="checkbox" checked={invalidOnly} onChange={(e) => setInvalidOnly(e.target.checked)} />
          Invalid only
        </label>
      </div>
      <table>
        <thead>
          <tr>
            <th>Title</th><th>Duration</th><th>Source</th><th>Codec</th><th>Container</th><th>Status</th>
          </tr>
        </thead>
        <tbody>
          {filtered.map((item) => (
            <tr key={item.id}>
              <td>{item.title}</td>
              <td>{formatDuration(item.duration_sec)}</td>
              <td>{sourceNameById.get(item.source_id) ?? item.source_id}</td>
              <td>{item.video_codec || "—"}/{item.audio_codec || "—"}</td>
              <td>{item.container || "—"}</td>
              <td>{item.invalid ? "Invalid" : "OK"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {filtered.length === 0 && <p>No media matches the current filters.</p>}
    </section>
  );
}

function formatDuration(seconds: number): string {
  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return h > 0
    ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
    : `${m}:${String(s).padStart(2, "0")}`;
}
```

`web/src/screens/LibraryScreen.css`:

```css
.library-filters {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 16px;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th, td {
  text-align: left;
  padding: 6px 10px;
  border-bottom: 1px solid #333;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/LibraryScreen.tsx web/src/screens/LibraryScreen.css web/src/screens/LibraryScreen.test.tsx
git commit -m "feat: add Media Library screen"
```

---

## Task 6: Settings Screen (Media Sources)

**Files:**
- Modify: `web/src/screens/SettingsScreen.tsx` (Task 4's placeholder, replaced wholesale)
- Create: `web/src/screens/SettingsScreen.css`
- Test: `web/src/screens/SettingsScreen.test.tsx`

**Interfaces:**
- Consumes: `useSources`, `useCreateSource`, `useDeleteSource`, `useScanSource` (Task 2's `api/sources.ts`), `useMediaItems` (Task 2's `api/media.ts`), `createTestQueryClient`/`wrapWithQueryClient` (Task 2), `server` (Task 1).
- Produces: `SettingsScreen` (same export name and `<h1>Settings</h1>` heading as Task 4's placeholder). Per spec §4.4, this screen covers media sources only — no playback/app config sections.

- [ ] **Step 1: Write the failing tests**

`web/src/screens/SettingsScreen.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { SettingsScreen } from "./SettingsScreen";

const SOURCES = [{ id: 1, name: "Movies", path: "/media/movies", created_at: "2026-01-01T00:00:00Z" }];
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 100,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];

function renderScreen(sources: typeof SOURCES = SOURCES, media: typeof MEDIA = MEDIA) {
  server.use(
    http.get("/api/sources", () => HttpResponse.json(sources)),
    http.get("/api/media", () => HttpResponse.json(media))
  );
  const client = createTestQueryClient();
  render(<SettingsScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("SettingsScreen", () => {
  it("lists each source with its path and item count", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    expect(screen.getByText("/media/movies — 1 item(s)")).toBeInTheDocument();
  });

  it("shows an empty state with no sources configured", async () => {
    renderScreen([]);
    expect(await screen.findByText("No media sources configured yet.")).toBeInTheDocument();
  });

  it("adds a new source via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/sources", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json({ id: 2, ...(created as object), created_at: "2026-01-01T00:00:00Z" }, { status: 201 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.type(screen.getByLabelText("Name"), "TV");
    await userEvent.type(screen.getByLabelText("Path"), "/media/tv");
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));

    await waitFor(() => expect(created).toEqual({ name: "TV", path: "/media/tv" }));
  });

  it("shows a scanning state while a rescan is in flight", async () => {
    server.use(
      http.post("/api/sources/1/scan", async () => {
        await delay(50);
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.click(screen.getByRole("button", { name: "Rescan" }));
    expect(await screen.findByRole("button", { name: "Scanning…" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Rescan" })).toBeInTheDocument());
  });

  it("requires a confirmation click before removing a source", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/sources/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByText("Movies");

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(deleted).toBe(false);
    expect(screen.getByText("Remove this source and all its media/programs?")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Confirm remove" }));
    await waitFor(() => expect(deleted).toBe(true));
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/screens/SettingsScreen.test.tsx`
Expected: FAIL — the placeholder `SettingsScreen` renders none of this.

- [ ] **Step 3: Write the implementation**

`web/src/screens/SettingsScreen.tsx`:

```tsx
import { useState, type FormEvent } from "react";
import { useMediaItems } from "../api/media";
import { useCreateSource, useDeleteSource, useScanSource, useSources } from "../api/sources";
import "./SettingsScreen.css";

export function SettingsScreen() {
  const { data: sources, isLoading, isError } = useSources();
  const { data: media } = useMediaItems();
  const createSource = useCreateSource();
  const deleteSource = useDeleteSource();
  const scanSource = useScanSource();
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<number | null>(null);

  function itemCount(sourceId: number): number {
    return (media ?? []).filter((m) => m.source_id === sourceId).length;
  }

  function handleAdd(e: FormEvent) {
    e.preventDefault();
    createSource.mutate(
      { name, path },
      { onSuccess: () => { setName(""); setPath(""); } }
    );
  }

  if (isLoading) return <p>Loading sources…</p>;
  if (isError) return <p role="alert">Failed to load media sources.</p>;

  return (
    <section>
      <h1>Settings</h1>
      <h2>Media Sources</h2>
      <ul className="source-list">
        {(sources ?? []).map((source) => (
          <li key={source.id}>
            <div>
              <strong>{source.name}</strong>
              <div className="source-path">{source.path} — {itemCount(source.id)} item(s)</div>
            </div>
            <div className="source-actions">
              <button
                onClick={() => scanSource.mutate(source.id)}
                disabled={scanSource.isPending && scanSource.variables === source.id}
              >
                {scanSource.isPending && scanSource.variables === source.id ? "Scanning…" : "Rescan"}
              </button>
              {confirmingDeleteId === source.id ? (
                <>
                  <span>Remove this source and all its media/programs?</span>
                  <button
                    onClick={() =>
                      deleteSource.mutate(source.id, { onSettled: () => setConfirmingDeleteId(null) })
                    }
                  >
                    Confirm remove
                  </button>
                  <button onClick={() => setConfirmingDeleteId(null)}>Cancel</button>
                </>
              ) : (
                <button onClick={() => setConfirmingDeleteId(source.id)}>Remove</button>
              )}
            </div>
          </li>
        ))}
      </ul>
      {(sources ?? []).length === 0 && <p>No media sources configured yet.</p>}

      <h2>Add a source</h2>
      <form onSubmit={handleAdd}>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Path
          <input value={path} onChange={(e) => setPath(e.target.value)} required />
        </label>
        <button type="submit" disabled={createSource.isPending}>Add source</button>
        {createSource.isError && <p role="alert">{createSource.error.message}</p>}
      </form>
    </section>
  );
}
```

`web/src/screens/SettingsScreen.css`:

```css
.source-list {
  list-style: none;
  padding: 0;
}

.source-list li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid #333;
  gap: 12px;
}

.source-path {
  font-size: 0.85em;
  opacity: 0.75;
}

.source-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

form label {
  display: block;
  margin-bottom: 8px;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/SettingsScreen.tsx web/src/screens/SettingsScreen.css web/src/screens/SettingsScreen.test.tsx
git commit -m "feat: add Settings screen for media source management"
```

---

## Task 7: Channels List Screen

**Files:**
- Modify: `web/src/screens/ChannelsListScreen.tsx` (Task 4's placeholder, replaced wholesale)
- Create: `web/src/screens/ChannelsListScreen.css`
- Test: `web/src/screens/ChannelsListScreen.test.tsx`

**Interfaces:**
- Consumes: `useChannels`, `useCreateChannel`, `useUpdateChannel`, `useDeleteChannel` (Task 3's `api/channels.ts`), `Channel` type (Task 3's `api/types.ts`), `createTestQueryClient`/`wrapWithQueryClient` (Task 2), `server` (Task 1), `Link` (`react-router-dom`, installed Task 4).
- Produces: `ChannelsListScreen` (same export name and `<h1>Channels</h1>` heading as Task 4's placeholder). Each channel links to `/channels/{id}` — **Task 9's `ChannelScheduleScreen` is what that route renders.**

- [ ] **Step 1: Write the failing tests**

`web/src/screens/ChannelsListScreen.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelsListScreen } from "./ChannelsListScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Sitcoms", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderScreen(channels: typeof CHANNELS = CHANNELS) {
  server.use(http.get("/api/channels", () => HttpResponse.json(channels)));
  const client = createTestQueryClient();
  render(<ChannelsListScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("ChannelsListScreen", () => {
  it("lists channels ordered by position, each linking to its schedule editor", async () => {
    renderScreen();
    expect(await screen.findByRole("link", { name: "Movies" })).toHaveAttribute("href", "/channels/1");
    expect(screen.getByRole("link", { name: "Sitcoms" })).toHaveAttribute("href", "/channels/2");
  });

  it("shows an empty state with no channels", async () => {
    renderScreen([]);
    expect(await screen.findByText("No channels yet.")).toBeInTheDocument();
  });

  it("creates a channel via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/channels", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(
          { id: 3, description: "", enabled: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", ...(created as object) },
          { status: 201 }
        );
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.type(screen.getByLabelText("Name"), "News");
    await userEvent.click(screen.getByRole("button", { name: "Create channel" }));

    await waitFor(() => expect(created).toEqual({ name: "News", position: 2 }));
  });

  it("toggles a channel's enabled state", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/channels/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...CHANNELS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("checkbox", { name: "Enabled" })[0]);

    await waitFor(() => expect(putBody).toMatchObject({ enabled: false }));
  });

  it("renames a channel", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/channels/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...CHANNELS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("button", { name: "Rename" })[0]);
    const input = screen.getByLabelText("Rename Movies");
    await userEvent.clear(input);
    await userEvent.type(input, "Movies HD");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(putBody).toMatchObject({ name: "Movies HD" }));
  });

  it("swaps positions when moving a channel down", async () => {
    const putBodies: unknown[] = [];
    server.use(
      http.put("/api/channels/:id", async ({ request }) => {
        putBodies.push(await request.json());
        return HttpResponse.json({});
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getByRole("button", { name: "Move Movies down" }));

    await waitFor(() => expect(putBodies).toHaveLength(2));
    expect(putBodies).toEqual(
      expect.arrayContaining([expect.objectContaining({ position: 1 }), expect.objectContaining({ position: 0 })])
    );
  });

  it("deletes a channel", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/channels/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByRole("link", { name: "Movies" });

    await userEvent.click(screen.getAllByRole("button", { name: "Delete" })[0]);

    await waitFor(() => expect(deleted).toBe(true));
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/screens/ChannelsListScreen.test.tsx`
Expected: FAIL — the placeholder `ChannelsListScreen` renders none of this.

- [ ] **Step 3: Write the implementation**

`web/src/screens/ChannelsListScreen.tsx`:

```tsx
import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useChannels, useCreateChannel, useDeleteChannel, useUpdateChannel } from "../api/channels";
import type { Channel } from "../api/types";
import "./ChannelsListScreen.css";

export function ChannelsListScreen() {
  const { data: channels, isLoading, isError } = useChannels();
  const createChannel = useCreateChannel();
  const updateChannel = useUpdateChannel();
  const deleteChannel = useDeleteChannel();
  const [name, setName] = useState("");
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renameValue, setRenameValue] = useState("");

  const sorted = [...(channels ?? [])].sort((a, b) => a.position - b.position);

  function handleCreate(e: FormEvent) {
    e.preventDefault();
    createChannel.mutate({ name, position: sorted.length }, { onSuccess: () => setName("") });
  }

  function startRename(channel: Channel) {
    setRenamingId(channel.id);
    setRenameValue(channel.name);
  }

  function commitRename(channel: Channel) {
    updateChannel.mutate({
      id: channel.id, name: renameValue, description: channel.description,
      enabled: channel.enabled, position: channel.position,
    });
    setRenamingId(null);
  }

  function toggleEnabled(channel: Channel) {
    updateChannel.mutate({
      id: channel.id, name: channel.name, description: channel.description,
      enabled: !channel.enabled, position: channel.position,
    });
  }

  function move(channel: Channel, direction: -1 | 1) {
    const index = sorted.findIndex((c) => c.id === channel.id);
    const other = sorted[index + direction];
    if (!other) return;
    updateChannel.mutate({
      id: channel.id, name: channel.name, description: channel.description,
      enabled: channel.enabled, position: other.position,
    });
    updateChannel.mutate({
      id: other.id, name: other.name, description: other.description,
      enabled: other.enabled, position: channel.position,
    });
  }

  if (isLoading) return <p>Loading channels…</p>;
  if (isError) return <p role="alert">Failed to load channels.</p>;

  return (
    <section>
      <h1>Channels</h1>
      <ul className="channel-list">
        {sorted.map((channel, index) => (
          <li key={channel.id}>
            <div className="channel-reorder">
              <button aria-label={`Move ${channel.name} up`} onClick={() => move(channel, -1)} disabled={index === 0}>↑</button>
              <button aria-label={`Move ${channel.name} down`} onClick={() => move(channel, 1)} disabled={index === sorted.length - 1}>↓</button>
            </div>
            {renamingId === channel.id ? (
              <>
                <input
                  value={renameValue}
                  onChange={(e) => setRenameValue(e.target.value)}
                  aria-label={`Rename ${channel.name}`}
                />
                <button onClick={() => commitRename(channel)}>Save</button>
                <button onClick={() => setRenamingId(null)}>Cancel</button>
              </>
            ) : (
              <Link to={`/channels/${channel.id}`}>{channel.name}</Link>
            )}
            <label>
              <input type="checkbox" checked={channel.enabled} onChange={() => toggleEnabled(channel)} />
              Enabled
            </label>
            {renamingId !== channel.id && <button onClick={() => startRename(channel)}>Rename</button>}
            <button onClick={() => deleteChannel.mutate(channel.id)}>Delete</button>
          </li>
        ))}
      </ul>
      {sorted.length === 0 && <p>No channels yet.</p>}

      <h2>Create a channel</h2>
      <form onSubmit={handleCreate}>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <button type="submit" disabled={createChannel.isPending}>Create channel</button>
      </form>
    </section>
  );
}
```

`web/src/screens/ChannelsListScreen.css`:

```css
.channel-list {
  list-style: none;
  padding: 0;
}

.channel-list li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid #333;
}

.channel-reorder {
  display: flex;
  flex-direction: column;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/ChannelsListScreen.tsx web/src/screens/ChannelsListScreen.css web/src/screens/ChannelsListScreen.test.tsx
git commit -m "feat: add Channels list screen"
```

---

## Task 8: Scheduling Time Utilities

**Files:**
- Create: `web/src/scheduling/time.ts`
- Test: `web/src/scheduling/time.test.ts`

**Interfaces:**
- Consumes: nothing (pure functions, no React, no API client).
- Produces: `computeEndTime(startTimeIso: string, durationSec: number): Date`, `formatTimeRange(start: Date, end: Date): string`, `toDatetimeLocalValue(iso: string): string`. **Task 9 (Channel Schedule editor) and Task 10 (Guide data layer) both import `computeEndTime`, which is the one place "end time = start time + media duration" (PRD's "end time computed from duration" rule) is implemented client-side — no other file recomputes it.** `formatTimeRange` deliberately formats in UTC (see Step 3 below — a documented, deliberate MVP simplification, not an oversight) so both this task's tests and later consumers get a fully deterministic, locale/timezone-independent display string.

- [ ] **Step 1: Write the failing tests**

`web/src/scheduling/time.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { computeEndTime, formatTimeRange, toDatetimeLocalValue } from "./time";

describe("computeEndTime", () => {
  it("adds the media duration (in seconds) to the start time", () => {
    const end = computeEndTime("2026-01-01T18:00:00Z", 5400); // 1.5 hours
    expect(end.toISOString()).toBe("2026-01-01T19:30:00.000Z");
  });

  it("returns the start time unchanged when duration is zero", () => {
    const end = computeEndTime("2026-01-01T18:00:00Z", 0);
    expect(end.toISOString()).toBe("2026-01-01T18:00:00.000Z");
  });
});

describe("formatTimeRange", () => {
  it("formats a start/end pair as UTC hour:minute", () => {
    const start = new Date("2026-01-01T18:00:00Z");
    const end = new Date("2026-01-01T19:30:00Z");
    expect(formatTimeRange(start, end)).toBe("06:00 PM – 07:30 PM");
  });
});

describe("toDatetimeLocalValue", () => {
  it("round-trips an ISO string to a datetime-local input value in local time", () => {
    // toDatetimeLocalValue feeds an <input type="datetime-local">, which is
    // always local-time by spec, so the expected value is reconstructed
    // from the same Date object's local getters rather than a fixed string
    // — that stays correct regardless of the test machine's timezone.
    const iso = "2026-01-01T18:00:00Z";
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, "0");
    const expected = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    expect(toDatetimeLocalValue(iso)).toBe(expected);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/scheduling/time.test.ts`
Expected: FAIL — `./time` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`web/src/scheduling/time.ts`:

```ts
// computeEndTime is the one place "end time = start time + media duration"
// (the PRD's end-time-from-duration rule, already implemented server-side
// in internal/scheduler's ScheduledProgram.EndTime()) is computed
// client-side. Every screen that needs a program's end time imports this
// instead of recomputing it.
export function computeEndTime(startTimeIso: string, durationSec: number): Date {
  return new Date(new Date(startTimeIso).getTime() + durationSec * 1000);
}

// Deliberately UTC: the MVP has no per-user timezone setting anywhere in
// the backend or this plan, and formatting in the viewer's local timezone
// here would make this function's output timezone-dependent and untestable
// without mocking the system clock. Displaying local time is a reasonable
// future enhancement, not built in this plan.
export function formatTimeRange(start: Date, end: Date): string {
  const fmt = new Intl.DateTimeFormat("en-US", { hour: "2-digit", minute: "2-digit", timeZone: "UTC" });
  return `${fmt.format(start)} – ${fmt.format(end)}`;
}

// Unlike formatTimeRange, this feeds an <input type="datetime-local">,
// which the HTML spec defines as always local time — so this one
// intentionally uses the browser's local timezone via the Date object's
// local getters.
export function toDatetimeLocalValue(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/scheduling
git commit -m "feat: add scheduling time utilities (end time, formatting)"
```

---

## Task 9: Channel Schedule Editor Screen

**Files:**
- Modify: `web/src/screens/ChannelScheduleScreen.tsx` (Task 4's placeholder, replaced wholesale)
- Create: `web/src/screens/ChannelScheduleScreen.css`
- Test: `web/src/screens/ChannelScheduleScreen.test.tsx`

**Interfaces:**
- Consumes: `useChannel` (Task 3's `api/channels.ts`), `useMediaItems` (Task 2's `api/media.ts`), `useProgramsForChannel`/`useAddProgram`/`useUpdateProgram`/`useDeleteProgram` (Task 3's `api/programs.ts`), `computeEndTime`/`formatTimeRange`/`toDatetimeLocalValue` (Task 8's `scheduling/time.ts`), `useParams`/`MemoryRouter`/`Route`/`Routes` (`react-router-dom`), `createTestQueryClient`/`wrapWithQueryClient` (Task 2), `server` (Task 1).
- Produces: `ChannelScheduleScreen` (same export name as Task 4's placeholder; its heading is now the dynamic channel name, replacing the placeholder's static `<h1>Channel Schedule</h1>` — this is the one placeholder heading Task 4's own tests don't assert against, exactly as noted in Task 4).

Per spec §4.3: ordering is purely derived from each program's `start_time` (no drag-drop, no separate position field for programs), and the start-time input is free-form — nothing prevents a gap between two programs, which is intentional (spec §4.1/§4.3, off-air gaps are a first-class state).

- [ ] **Step 1: Write the failing tests**

`web/src/screens/ChannelScheduleScreen.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelScheduleScreen } from "./ChannelScheduleScreen";

const CHANNEL = {
  id: 1, name: "Movies", description: "", enabled: true, position: 0,
  created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
};
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 5400,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
const PROGRAMS = [
  { id: 1, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderScreen(programs: typeof PROGRAMS = PROGRAMS) {
  server.use(
    http.get("/api/channels/1", () => HttpResponse.json(CHANNEL)),
    http.get("/api/channels/1/programs", () => HttpResponse.json(programs)),
    http.get("/api/media", () => HttpResponse.json(MEDIA))
  );
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={["/channels/1"]}>
      <Routes>
        <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("ChannelScheduleScreen", () => {
  it("renders the channel name and each program with its computed end time", async () => {
    renderScreen();
    expect(await screen.findByRole("heading", { name: "Movies" })).toBeInTheDocument();
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("06:00 PM – 07:30 PM")).toBeInTheDocument();
  });

  it("shows an empty state with no programs scheduled", async () => {
    renderScreen([]);
    expect(await screen.findByText("No programs scheduled yet.")).toBeInTheDocument();
  });

  it("adds a program via the form", async () => {
    let created: unknown = null;
    server.use(
      http.post("/api/channels/1/programs", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(
          { id: 2, channel_id: 1, ...(created as object), created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
          { status: 201 }
        );
      })
    );
    renderScreen();
    await screen.findByText("Movie A");

    await userEvent.selectOptions(screen.getByLabelText("Media"), "1");
    fireEvent.change(screen.getByLabelText("Start time"), { target: { value: "2026-01-02T10:00" } });
    await userEvent.click(screen.getByRole("button", { name: "Add program" }));

    await waitFor(() =>
      expect(created).toEqual({ media_item_id: 1, start_time: new Date("2026-01-02T10:00").toISOString() })
    );
  });

  it("edits a program's start time", async () => {
    let putBody: unknown = null;
    server.use(
      http.put("/api/programs/1", async ({ request }) => {
        putBody = await request.json();
        return HttpResponse.json({ ...PROGRAMS[0], ...(putBody as object) });
      })
    );
    renderScreen();
    await screen.findByText("Movie A");

    await userEvent.click(screen.getByRole("button", { name: "Edit start time" }));
    fireEvent.change(screen.getByLabelText("Edit start time"), { target: { value: "2026-01-01T20:00" } });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(putBody).toEqual({ media_item_id: 1, start_time: new Date("2026-01-01T20:00").toISOString() })
    );
  });

  it("removes a program", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/programs/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      })
    );
    renderScreen();
    await screen.findByText("Movie A");

    await userEvent.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(deleted).toBe(true));
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/screens/ChannelScheduleScreen.test.tsx`
Expected: FAIL — the placeholder `ChannelScheduleScreen` renders none of this.

- [ ] **Step 3: Write the implementation**

`web/src/screens/ChannelScheduleScreen.tsx`:

```tsx
import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";
import { useChannel } from "../api/channels";
import { useMediaItems } from "../api/media";
import { useAddProgram, useDeleteProgram, useProgramsForChannel, useUpdateProgram } from "../api/programs";
import { computeEndTime, formatTimeRange, toDatetimeLocalValue } from "../scheduling/time";
import "./ChannelScheduleScreen.css";

export function ChannelScheduleScreen() {
  const params = useParams<{ id: string }>();
  const channelId = Number(params.id);
  const { data: channel, isLoading: channelLoading, isError: channelError } = useChannel(channelId);
  const { data: programs, isLoading: programsLoading, isError: programsError } = useProgramsForChannel(channelId);
  const { data: media } = useMediaItems();
  const addProgram = useAddProgram(channelId);
  const updateProgram = useUpdateProgram(channelId);
  const deleteProgram = useDeleteProgram(channelId);

  const [mediaItemId, setMediaItemId] = useState<number | "">("");
  const [startTime, setStartTime] = useState("");
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editStartTime, setEditStartTime] = useState("");

  const mediaById = new Map((media ?? []).map((m) => [m.id, m]));
  const sortedPrograms = [...(programs ?? [])].sort(
    (a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
  );

  function handleAdd(e: FormEvent) {
    e.preventDefault();
    if (mediaItemId === "" || !startTime) return;
    addProgram.mutate(
      { channelId, media_item_id: mediaItemId, start_time: new Date(startTime).toISOString() },
      { onSuccess: () => { setMediaItemId(""); setStartTime(""); } }
    );
  }

  function startEdit(programId: number, currentStart: string) {
    setEditingId(programId);
    setEditStartTime(toDatetimeLocalValue(currentStart));
  }

  function commitEdit(programId: number, mediaItemIdForProgram: number) {
    updateProgram.mutate({
      id: programId, channelId, media_item_id: mediaItemIdForProgram,
      start_time: new Date(editStartTime).toISOString(),
    });
    setEditingId(null);
  }

  if (channelLoading || programsLoading) return <p>Loading schedule…</p>;
  if (channelError || programsError || !channel) return <p role="alert">Failed to load this channel's schedule.</p>;

  return (
    <section>
      <h1>{channel.name}</h1>
      <ul className="program-list">
        {sortedPrograms.map((program) => {
          const item = mediaById.get(program.media_item_id);
          const end = computeEndTime(program.start_time, item?.duration_sec ?? 0);
          return (
            <li key={program.id}>
              <span>{item?.title ?? `Media #${program.media_item_id}`}</span>
              {editingId === program.id ? (
                <>
                  <input
                    type="datetime-local"
                    value={editStartTime}
                    onChange={(e) => setEditStartTime(e.target.value)}
                    aria-label="Edit start time"
                  />
                  <button onClick={() => commitEdit(program.id, program.media_item_id)}>Save</button>
                  <button onClick={() => setEditingId(null)}>Cancel</button>
                </>
              ) : (
                <>
                  <span>{formatTimeRange(new Date(program.start_time), end)}</span>
                  <button onClick={() => startEdit(program.id, program.start_time)}>Edit start time</button>
                </>
              )}
              <button onClick={() => deleteProgram.mutate({ id: program.id, channelId })}>Remove</button>
            </li>
          );
        })}
      </ul>
      {sortedPrograms.length === 0 && <p>No programs scheduled yet.</p>}

      <h2>Add a program</h2>
      <form onSubmit={handleAdd}>
        <label>
          Media
          <select
            value={mediaItemId}
            onChange={(e) => setMediaItemId(e.target.value === "" ? "" : Number(e.target.value))}
            aria-label="Media"
            required
          >
            <option value="">Select media…</option>
            {(media ?? []).map((m) => (
              <option key={m.id} value={m.id}>{m.title}</option>
            ))}
          </select>
        </label>
        <label>
          Start time
          <input
            type="datetime-local"
            value={startTime}
            onChange={(e) => setStartTime(e.target.value)}
            aria-label="Start time"
            required
          />
        </label>
        <button type="submit" disabled={addProgram.isPending}>Add program</button>
      </form>
    </section>
  );
}
```

`web/src/screens/ChannelScheduleScreen.css`:

```css
.program-list {
  list-style: none;
  padding: 0;
}

.program-list li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid #333;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/ChannelScheduleScreen.tsx web/src/screens/ChannelScheduleScreen.css web/src/screens/ChannelScheduleScreen.test.tsx
git commit -m "feat: add Channel schedule editor screen"
```

---

## Task 10: Guide Data Layer (Pure Functions)

**Files:**
- Create: `web/src/scheduling/guide.ts`
- Test: `web/src/scheduling/guide.test.ts`

**Interfaces:**
- Consumes: `computeEndTime` (Task 8's `scheduling/time.ts`), `MediaItem`/`Program` types (Task 2/3's `api/types.ts`).
- Produces: `joinProgramsWithMedia(programs, mediaById): GuideProgram[]` (sorted by start time — **`buildTimeline` requires this precondition and does not sort its input itself**), `buildTimeline(programs: GuideProgram[], windowStart, windowEnd): TimelineBlock[]`, `defaultGuideWindow(now: Date): { start: Date; end: Date }`, and the `GuideProgram`/`TimelineBlock` types. **Task 11 (Guide screen UI) is the only consumer**, and renders exactly the blocks this task produces — it contains no scheduling/gap logic of its own.

This is the module spec §4.1's off-air rendering rule (gaps before the first program, between programs, after the last, or spanning the whole row when a channel has no programs at all in the window) is actually implemented — no React, no API calls, pure and independently testable, the same way `internal/scheduler` is pure on the backend.

- [ ] **Step 1: Write the failing tests**

`web/src/scheduling/guide.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import type { MediaItem, Program } from "../api/types";
import { buildTimeline, defaultGuideWindow, joinProgramsWithMedia } from "./guide";

const WINDOW_START = new Date("2026-01-01T17:00:00Z");
const WINDOW_END = new Date("2026-01-01T23:00:00Z");

function guideProgram(id: number, startIso: string, endIso: string) {
  return { programId: id, mediaItemId: id, title: `Program ${id}`, start: new Date(startIso), end: new Date(endIso) };
}

describe("buildTimeline", () => {
  it("returns a single off-air block spanning the window when there are no programs", () => {
    expect(buildTimeline([], WINDOW_START, WINDOW_END)).toEqual([
      { type: "off-air", start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("returns just the program block when it exactly fills the window", () => {
    const p = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T23:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("adds an off-air block before a program that starts after the window start", () => {
    const p = guideProgram(1, "2026-01-01T18:00:00Z", "2026-01-01T19:00:00Z");
    const blocks = buildTimeline([p], WINDOW_START, WINDOW_END);
    expect(blocks[0]).toEqual({ type: "off-air", start: WINDOW_START, end: p.start });
    expect(blocks[1]).toEqual({ type: "program", program: p, start: p.start, end: p.end });
  });

  it("adds an off-air block after a program that ends before the window end", () => {
    const p = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T18:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: p.end },
      { type: "off-air", start: p.end, end: WINDOW_END },
    ]);
  });

  it("adds an off-air block between two non-contiguous programs", () => {
    const a = guideProgram(1, "2026-01-01T17:00:00Z", "2026-01-01T18:00:00Z");
    const b = guideProgram(2, "2026-01-01T19:00:00Z", "2026-01-01T20:00:00Z");
    const blocks = buildTimeline([a, b], WINDOW_START, WINDOW_END);
    expect(blocks[0]).toMatchObject({ type: "program", program: a });
    expect(blocks[1]).toEqual({ type: "off-air", start: a.end, end: b.start });
    expect(blocks[2]).toMatchObject({ type: "program", program: b });
  });

  it("clips a program that runs across both window edges to the window bounds", () => {
    const p = guideProgram(1, "2026-01-01T10:00:00Z", "2026-01-02T00:00:00Z");
    expect(buildTimeline([p], WINDOW_START, WINDOW_END)).toEqual([
      { type: "program", program: p, start: WINDOW_START, end: WINDOW_END },
    ]);
  });

  it("excludes programs entirely outside the window", () => {
    const before = guideProgram(1, "2026-01-01T10:00:00Z", "2026-01-01T11:00:00Z");
    const inWindow = guideProgram(2, "2026-01-01T18:00:00Z", "2026-01-01T19:00:00Z");
    const blocks = buildTimeline([before, inWindow], WINDOW_START, WINDOW_END);
    expect(blocks.some((b) => b.type === "program" && b.program.programId === 1)).toBe(false);
  });
});

describe("joinProgramsWithMedia", () => {
  const mediaItem: MediaItem = {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  };

  it("joins each program with its media title and computed end time, sorted by start time", () => {
    const media = new Map([[1, mediaItem]]);
    const programs: Program[] = [
      { id: 2, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T20:00:00Z", created_at: "", updated_at: "" },
      { id: 1, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T18:00:00Z", created_at: "", updated_at: "" },
    ];
    const joined = joinProgramsWithMedia(programs, media);
    expect(joined.map((p) => p.programId)).toEqual([1, 2]);
    expect(joined[0]).toMatchObject({
      title: "Movie A",
      start: new Date("2026-01-01T18:00:00Z"),
      end: new Date("2026-01-01T19:00:00Z"),
    });
  });

  it("falls back to a placeholder title when the media item is missing", () => {
    const programs: Program[] = [
      { id: 1, channel_id: 1, media_item_id: 99, start_time: "2026-01-01T18:00:00Z", created_at: "", updated_at: "" },
    ];
    const joined = joinProgramsWithMedia(programs, new Map());
    expect(joined[0].title).toBe("Media #99");
  });
});

describe("defaultGuideWindow", () => {
  it("returns 1 hour before now to 5 hours after now", () => {
    const now = new Date("2026-01-01T18:00:00Z");
    expect(defaultGuideWindow(now)).toEqual({
      start: new Date("2026-01-01T17:00:00Z"),
      end: new Date("2026-01-01T23:00:00Z"),
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/scheduling/guide.test.ts`
Expected: FAIL — `./guide` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`web/src/scheduling/guide.ts`:

```ts
import type { MediaItem, Program } from "../api/types";
import { computeEndTime } from "./time";

export interface GuideProgram {
  programId: number;
  mediaItemId: number;
  title: string;
  start: Date;
  end: Date;
}

export type TimelineBlock =
  | { type: "program"; program: GuideProgram; start: Date; end: Date }
  | { type: "off-air"; start: Date; end: Date };

export function joinProgramsWithMedia(programs: Program[], mediaById: Map<number, MediaItem>): GuideProgram[] {
  return programs
    .map((p) => {
      const item = mediaById.get(p.media_item_id);
      return {
        programId: p.id,
        mediaItemId: p.media_item_id,
        title: item?.title ?? `Media #${p.media_item_id}`,
        start: new Date(p.start_time),
        end: computeEndTime(p.start_time, item?.duration_sec ?? 0),
      };
    })
    .sort((a, b) => a.start.getTime() - b.start.getTime());
}

// buildTimeline turns a channel's programs (already sorted by start time —
// see joinProgramsWithMedia) into a contiguous sequence of blocks spanning
// [windowStart, windowEnd) with no gaps: every moment in the window is
// covered by exactly one program block or one off-air block. Off-air is a
// first-class state (spec §4.1), mirroring the backend scheduler's
// CurrentState.Current == nil (internal/scheduler/scheduler.go).
export function buildTimeline(programs: GuideProgram[], windowStart: Date, windowEnd: Date): TimelineBlock[] {
  const blocks: TimelineBlock[] = [];
  const relevant = programs.filter((p) => p.end > windowStart && p.start < windowEnd);

  let cursor = windowStart;
  for (const program of relevant) {
    if (program.start > cursor) {
      blocks.push({ type: "off-air", start: cursor, end: program.start });
    }
    blocks.push({
      type: "program",
      program,
      start: program.start < windowStart ? windowStart : program.start,
      end: program.end > windowEnd ? windowEnd : program.end,
    });
    if (program.end > cursor) cursor = program.end;
  }
  if (cursor < windowEnd) {
    blocks.push({ type: "off-air", start: cursor, end: windowEnd });
  }
  return blocks;
}

export function defaultGuideWindow(now: Date): { start: Date; end: Date } {
  return {
    start: new Date(now.getTime() - 60 * 60 * 1000),
    end: new Date(now.getTime() + 5 * 60 * 60 * 1000),
  };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/scheduling/guide.ts web/src/scheduling/guide.test.ts
git commit -m "feat: add Guide data layer (program/media join, off-air timeline)"
```

---

## Task 11: Guide Screen UI

**Files:**
- Modify: `web/src/screens/GuideScreen.tsx` (Task 4's placeholder, replaced wholesale)
- Create: `web/src/screens/GuideScreen.css`
- Test: `web/src/screens/GuideScreen.test.tsx`

**Interfaces:**
- Consumes: `useChannels` (Task 3), `useMediaItems` (Task 2), `listPrograms` (Task 3's `api/programs.ts` — called directly through TanStack Query's `useQueries`, not `useProgramsForChannel`, because the number of channels is dynamic and React hooks cannot be called a variable number of times per render; `useQueries` is TanStack Query's supported mechanism for exactly this), `joinProgramsWithMedia`/`buildTimeline`/`defaultGuideWindow`/`TimelineBlock` (Task 10's `scheduling/guide.ts`), `formatTimeRange` (Task 8's `scheduling/time.ts`).
- Produces: `GuideScreen` (same export name and `<h1>Guide</h1>` heading as Task 4's placeholder — `AppRoutes.test.tsx` keeps passing unmodified). This is the app's default route (`/`).

Per spec §4.1: default visible window is 1 hour before now to 5 hours after now; disabled channels are not shown; programs/media poll every 30s to catch schedule changes made elsewhere, while the "now" position itself ticks from a separate 60s client-side timer — no per-second server round-trips for either.

- [ ] **Step 1: Write the failing tests**

`web/src/screens/GuideScreen.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { GuideScreen } from "./GuideScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Off Channel", description: "", enabled: false, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];
const MEDIA = [
  {
    id: 1, source_id: 1, rel_path: "a.mp4", title: "Movie A", duration_sec: 3600,
    video_codec: "h264", audio_codec: "aac", container: "mp4", size_bytes: 1,
    mod_time: "2026-01-01T00:00:00Z", invalid: false,
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
];
const PROGRAMS_CH1 = [
  { id: 1, channel_id: 1, media_item_id: 1, start_time: "2026-01-01T19:00:00Z", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderScreen() {
  server.use(
    http.get("/api/channels", () => HttpResponse.json(CHANNELS)),
    http.get("/api/media", () => HttpResponse.json(MEDIA)),
    http.get("/api/channels/1/programs", () => HttpResponse.json(PROGRAMS_CH1)),
    http.get("/api/channels/2/programs", () => HttpResponse.json([]))
  );
  const client = createTestQueryClient();
  render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });
}

describe("GuideScreen", () => {
  beforeEach(() => {
    // Only Date is faked (not setTimeout/setInterval) so React Testing
    // Library's async findBy*/waitFor polling — which relies on real
    // timers — keeps working; only "what time is it" is under test control.
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-01-01T18:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders a row only for enabled channels", async () => {
    renderScreen();
    expect(await screen.findByText("Movies")).toBeInTheDocument();
    expect(screen.queryByText("Off Channel")).not.toBeInTheDocument();
  });

  it("renders the scheduled program and Off Air blocks for the surrounding gaps", async () => {
    renderScreen();
    await screen.findByText("Movies");
    expect(await screen.findByText("Movie A")).toBeInTheDocument();
    // Window is [17:00, 23:00); the one program runs 19:00-20:00, so there
    // are two gaps: before it (17:00-19:00) and after it (20:00-23:00).
    expect(screen.getAllByText("Off Air")).toHaveLength(2);
  });

  it("shows the now-line when the current time falls within the default window", async () => {
    renderScreen();
    await screen.findByText("Movies");
    expect(screen.getByTestId("now-line")).toBeInTheDocument();
  });

  it("hides the now-line when the current time falls outside the default window", async () => {
    vi.setSystemTime(new Date("2026-01-03T00:00:00Z"));
    renderScreen();
    await screen.findByText("Movies");
    expect(screen.queryByTestId("now-line")).not.toBeInTheDocument();
  });

  it("shows an empty-state message when there are no enabled channels", async () => {
    server.use(
      http.get("/api/channels", () => HttpResponse.json([CHANNELS[1]])),
      http.get("/api/media", () => HttpResponse.json([])),
      http.get("/api/channels/2/programs", () => HttpResponse.json([]))
    );
    const client = createTestQueryClient();
    render(<GuideScreen />, { wrapper: wrapWithQueryClient(client) });
    expect(await screen.findByText("No enabled channels to show.")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- src/screens/GuideScreen.test.tsx`
Expected: FAIL — the placeholder `GuideScreen` renders none of this.

- [ ] **Step 3: Write the implementation**

`web/src/screens/GuideScreen.tsx`:

```tsx
import { useQueries } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useChannels } from "../api/channels";
import { useMediaItems } from "../api/media";
import { listPrograms } from "../api/programs";
import { buildTimeline, defaultGuideWindow, joinProgramsWithMedia, type TimelineBlock } from "../scheduling/guide";
import { formatTimeRange } from "../scheduling/time";
import "./GuideScreen.css";

const POLL_INTERVAL_MS = 30_000;
const NOW_TICK_MS = 60_000;

export function GuideScreen() {
  const { data: channels, isLoading: channelsLoading, isError: channelsError } = useChannels();
  const { data: media, isLoading: mediaLoading, isError: mediaError } = useMediaItems();
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), NOW_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const programQueries = useQueries({
    queries: (channels ?? []).map((channel) => ({
      queryKey: ["channels", channel.id, "programs"] as const,
      queryFn: () => listPrograms(channel.id),
      refetchInterval: POLL_INTERVAL_MS,
    })),
  });

  const mediaById = useMemo(() => new Map((media ?? []).map((m) => [m.id, m])), [media]);
  const { start: windowStart, end: windowEnd } = defaultGuideWindow(now);
  const totalMs = windowEnd.getTime() - windowStart.getTime();

  if (channelsLoading || mediaLoading) return <p>Loading guide…</p>;
  if (channelsError || mediaError) return <p role="alert">Failed to load the guide.</p>;

  const rows = (channels ?? [])
    .map((channel, index) => ({ channel, programs: programQueries[index]?.data ?? [] }))
    .filter((row) => row.channel.enabled);

  const showNowLine = now >= windowStart && now < windowEnd;
  const nowPercent = ((now.getTime() - windowStart.getTime()) / totalMs) * 100;

  return (
    <section>
      <h1>Guide</h1>
      <div className="guide-grid">
        {showNowLine && (
          <div className="guide-now-line" data-testid="now-line" style={{ left: `${nowPercent}%` }} />
        )}
        {rows.map(({ channel, programs }) => {
          const joined = joinProgramsWithMedia(programs, mediaById);
          const timeline = buildTimeline(joined, windowStart, windowEnd);
          return (
            <div className="guide-row" key={channel.id}>
              <div className="guide-channel-name">{channel.name}</div>
              <div className="guide-timeline">
                {timeline.map((block, i) => (
                  <TimelineBlockView key={i} block={block} totalMs={totalMs} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
      {rows.length === 0 && <p>No enabled channels to show.</p>}
    </section>
  );
}

function TimelineBlockView({ block, totalMs }: { block: TimelineBlock; totalMs: number }) {
  const widthPercent = ((block.end.getTime() - block.start.getTime()) / totalMs) * 100;
  if (block.type === "off-air") {
    return <div className="guide-block guide-block-offair" style={{ width: `${widthPercent}%` }}>Off Air</div>;
  }
  return (
    <div
      className="guide-block guide-block-program"
      style={{ width: `${widthPercent}%` }}
      title={formatTimeRange(block.program.start, block.program.end)}
    >
      {block.program.title}
    </div>
  );
}
```

`web/src/screens/GuideScreen.css`:

```css
.guide-grid {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.guide-row {
  display: flex;
  align-items: stretch;
}

.guide-channel-name {
  width: 120px;
  flex-shrink: 0;
  padding: 6px;
  font-weight: 600;
}

.guide-timeline {
  display: flex;
  flex: 1;
  min-width: 0;
}

.guide-block {
  padding: 4px 6px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  border-right: 1px solid #222;
}

.guide-block-program {
  background: rgba(255, 255, 255, 0.1);
}

.guide-block-offair {
  background: transparent;
  opacity: 0.5;
  font-style: italic;
}

.guide-now-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: #e74c3c;
  pointer-events: none;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test`
Expected: PASS (this is the full frontend test suite — every task's tests should now pass together)

- [ ] **Step 5: Commit**

```bash
cd /home/daslaptop/HomeStreamProject
git add web/src/screens/GuideScreen.tsx web/src/screens/GuideScreen.css web/src/screens/GuideScreen.test.tsx
git commit -m "feat: add Guide screen with timeline grid and off-air rendering"
```

---

## Task 12: `go:embed` Wiring — Production Static Serving and Dev Proxy

**Files:**
- Modify: `.gitignore` (repo root — remove the `/web/dist` line)
- Create: `web/dist/.gitkeep` (tracked, empty)
- Create: `web/dist/.gitignore` (tracked — excludes everything else under `web/dist`)
- Create: `web/embed.go`
- Test: `web/embed_test.go`
- Modify: `internal/api/router.go` (additive: a new field + method + one conditional route registration — no existing route or signature changes)
- Test: `internal/api/router_test.go`
- Modify: `cmd/personaltv/main.go` (wire the static handler in)
- Modify: `web/vite.config.ts` (add the dev-mode `/api` proxy)

**Interfaces:**
- Consumes: nothing new from earlier frontend tasks (this task only needs `web/dist`, the build output). This is the one task in this plan that touches `internal/` and `cmd/`, per the Global Constraints exception.
- Produces: `web.Handler() (http.Handler, error)` (package `web`, `web/embed.go`) — serves the embedded `web/dist` with SPA fallback (any unmatched path serves `index.html` so client-side routes like `/channels/5` resolve). `(*api.Server).SetStaticHandler(h http.Handler)` — additive; **all 13 existing `srv.Routes()` call sites across `internal/api/*_test.go` and `internal/integration/end_to_end_test.go` are untouched and keep passing unmodified**, since a `Server` with no static handler set behaves exactly as it does on `main` today (unmatched paths still 404).

**Why not change `Routes()`'s or `NewServer`'s signature:** the backend plan's own precedent (Task 9→10) shows changing `NewServer`'s signature is acceptable when every call site is updated as part of that same task — but here there is no need to touch 13 files just to thread one optional handler through, when a small additive setter accomplishes the same wiring with zero blast radius on existing tests.

- [ ] **Step 1: Stop ignoring `web/dist` at the repo root**

In `.gitignore`, remove the `/web/dist` line (keep everything else, including `/web/node_modules`). `web/dist`'s own nested `.gitignore` (Step 2) takes over from here.

- [ ] **Step 2: Add the tracked placeholder inside `web/dist`**

Create `web/dist/.gitkeep` (empty file) — this is what lets `//go:embed all:dist` (Step 3) compile even before anyone has run `npm run build` (a fresh clone, or any Go-only task in this plan run before this one).

Create `web/dist/.gitignore`:

```
*
!.gitignore
!.gitkeep
```

This ignores everything else `npm run build` produces inside `web/dist` (so build output never gets committed) while keeping the directory itself, and these two files, tracked.

- [ ] **Step 3: Write the embed handler**

`web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// all: is required, not stylistic — before `npm run build` has ever run,
// web/dist contains only the tracked .gitkeep (a dot-file), and a plain
// (non-"all:") embed pattern excludes dot/underscore-prefixed files. With
// nothing else to match, `//go:embed dist` would fail to compile with
// "pattern dist: no matching files found". `all:dist` always has at least
// .gitkeep to embed, so this package always compiles — go build/vet never
// depends on the frontend having been built first.
//
//go:embed all:dist
var distFS embed.FS

// Handler serves the built SPA (the contents of web/dist, produced by
// `npm run build`) from the embedded filesystem. Any request path that
// doesn't match a real file falls back to index.html, so client-side
// routes (e.g. /channels/5) resolve to the SPA instead of 404ing.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
```

- [ ] **Step 4: Verify Go still builds from just the placeholder**

Run: `go build ./... && go vet ./...`
Expected: succeeds, even though `npm run build` has not run yet in this task — this is the payoff of Step 2/3's `all:dist` placeholder trick.

- [ ] **Step 5: Write the failing tests for the static-handler wiring**

`internal/api/router_test.go`:

```go
package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes_WithoutStaticHandler_UnmatchedPath404s(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/some/unmatched/path")
	if err != nil {
		t.Fatalf("GET returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for an unmatched path with no static handler set, got %d", resp.StatusCode)
	}
}

func TestRoutes_WithStaticHandler_FallsBackForUnmatchedPaths(t *testing.T) {
	srv := newTestServer(t)
	srv.SetStaticHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("static: " + r.URL.Path))
	}))
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/guide")
	if err != nil {
		t.Fatalf("GET returned error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from the static handler, got %d", resp.StatusCode)
	}
	if string(body) != "static: /guide" {
		t.Errorf("expected the static handler to receive the unmatched path, got %q", string(body))
	}

	// /api/sources must still route to the real API handler, not the static fallback.
	apiResp, err := http.Get(ts.URL + "/api/sources")
	if err != nil {
		t.Fatalf("GET /api/sources returned error: %v", err)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusOK {
		t.Errorf("expected /api/sources to still be routed to the API, got status %d", apiResp.StatusCode)
	}
}
```

(`newTestServer(t)` is the existing helper already shared across this package's other test files — see `internal/api/sources_handlers_test.go`.)

- [ ] **Step 6: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run TestRoutes_`
Expected: FAIL — `srv.SetStaticHandler` doesn't exist yet.

- [ ] **Step 7: Add the static handler field and method to the Server**

In `internal/api/router.go`, add `static http.Handler` to the `Server` struct, add the method below it, and register it as the last route in `Routes()`:

```go
type Server struct {
	sources  repository.MediaSourceRepository
	items    repository.MediaItemRepository
	scanner  *mediastore.Scanner
	channels *channels.Service
	static   http.Handler
}
```

```go
// SetStaticHandler registers the handler used for any request that doesn't
// match /healthz or /api/*, e.g. the embedded frontend SPA (see
// cmd/personaltv/main.go). If never called, unmatched paths 404 as before
// — every existing test and NewServer call site is unaffected.
func (s *Server) SetStaticHandler(h http.Handler) {
	s.static = h
}
```

Inside `Routes()`, immediately before `return mux`:

```go
	if s.static != nil {
		mux.Handle("/", s.static)
	}

	return mux
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS (all of this package's tests, old and new)

- [ ] **Step 9: Wire the static handler into `main.go`**

In `cmd/personaltv/main.go`, add `"personaltv/web"` to the imports, and between constructing `server` and constructing `srv`:

```go
	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)

	webHandler, err := web.Handler()
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}
	server.SetStaticHandler(webHandler)
```

- [ ] **Step 10: Add the dev-mode API proxy to Vite**

In `web/vite.config.ts`, add a `server.proxy` entry so `npm run dev` forwards `/api/*` to the Go backend (default `PERSONALTV_PORT=8080`, per `cmd/personaltv/main.go`):

```ts
/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: false,
  },
});
```

- [ ] **Step 11: Build the real frontend and write the embed tests**

```bash
cd web
npm run build
```

`web/embed_test.go`:

```go
package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ServesIndexAtRoot(t *testing.T) {
	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `<div id="root">`) {
		t.Errorf("expected index.html to contain the SPA's root div, got: %s", w.Body.String())
	}
}

func TestHandler_FallsBackToIndexForClientRoutes(t *testing.T) {
	handler, err := Handler()
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	rootW := httptest.NewRecorder()
	handler.ServeHTTP(rootW, httptest.NewRequest("GET", "/", nil))

	routeW := httptest.NewRecorder()
	handler.ServeHTTP(routeW, httptest.NewRequest("GET", "/channels/5", nil))

	if routeW.Code != 200 {
		t.Fatalf("expected status 200 for a client-side route, got %d", routeW.Code)
	}
	if routeW.Body.String() != rootW.Body.String() {
		t.Errorf("expected /channels/5 to fall back to the same body as /, but it differed")
	}
}
```

**Note:** these two tests only pass once `npm run build` has produced a real `web/dist/index.html` — this is the one test in this plan (and the whole repo) with that prerequisite, the frontend equivalent of the backend plan's `ffmpeg`/`ffprobe`-on-`PATH` requirement.

- [ ] **Step 12: Run the tests to verify they pass**

Run: `go test ./web/...`
Expected: PASS

- [ ] **Step 13: Run the full verification suite and commit**

```bash
cd /home/daslaptop/HomeStreamProject
go build ./...
go vet ./...
gofmt -l .
go test ./...
cd web
npm test
npm run build
```

Expected: everything passes/exits 0. Then, as a manual sanity check (not automatable here): run `go run ./cmd/personaltv` from the repo root and confirm `http://localhost:8080/` serves the Guide screen and `http://localhost:8080/api/channels` still returns JSON.

```bash
cd /home/daslaptop/HomeStreamProject
git add .gitignore web/dist/.gitkeep web/dist/.gitignore web/embed.go web/embed_test.go web/vite.config.ts internal/api/router.go internal/api/router_test.go cmd/personaltv/main.go
git commit -m "feat: embed and serve the built frontend from the Go binary"
```

---

## Definition of Done

After Task 12: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` (Go side) and `npm test`, `npm run build` (frontend side) all pass from a clean checkout with only Node.js 20+/npm and Go 1.22+/`ffmpeg`/`ffprobe` on `PATH` (the union of this plan's and the core-backend plan's prerequisites). `go run ./cmd/personaltv` serves a working SPA at `/` — Guide (with off-air gaps rendered), Media Library, Channels (list, create/rename/delete/reorder/enable-toggle, and each channel's schedule editor with add/edit-start-time/remove), and Settings (media sources: list/add/rescan/remove-with-confirmation) — entirely through the existing REST API, with `npm run dev`'s proxy giving the same experience against a locally-running `go run ./cmd/personaltv` during development. TV/player is not built (spec §1/§8) — the sidebar has no fifth nav item wired up for it, only room reserved.
