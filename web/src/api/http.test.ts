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
