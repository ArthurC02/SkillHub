import { afterEach, expect, test, vi } from "vitest";
import { ApiError, apiFetch } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("apiFetch rejects with the server's error string when the JSON body has one", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "資料庫連線中斷" }), {
          status: 500,
          statusText: "Internal Server Error",
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );

  await expect(apiFetch("/api/skills")).rejects.toMatchObject({
    status: 500,
    message: "資料庫連線中斷",
  } satisfies Partial<ApiError>);
});

test("apiFetch falls back to statusText when the JSON body has no error string", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ detail: "ignored" }), {
          status: 400,
          statusText: "Bad Request",
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );

  await expect(apiFetch("/api/skills")).rejects.toMatchObject({
    status: 400,
    message: "Bad Request",
  } satisfies Partial<ApiError>);
});

test("apiFetch falls back to statusText when the body is not JSON", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("not json", { status: 503, statusText: "Service Unavailable" })),
  );

  await expect(apiFetch("/api/skills")).rejects.toMatchObject({
    status: 503,
    message: "Service Unavailable",
  } satisfies Partial<ApiError>);
});
