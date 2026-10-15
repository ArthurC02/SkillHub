import { afterEach, expect, test, vi } from "vitest";
import { ApiError, apiFetch, isTransientReadFailure, serverReason } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

test.each([
  ["no error", undefined, false],
  ["an absent error", null, false],
  ["a network error", new TypeError("Failed to fetch"), true],
  ["a 400", new ApiError(400, "bad request"), false],
  ["a 401", new ApiError(401, "not authenticated"), false],
  ["a 403", new ApiError(403, "forbidden"), false],
  ["a 404", new ApiError(404, "not found"), false],
  ["a 408", new ApiError(408, "request timeout"), true],
  ["a 429", new ApiError(429, "too many requests"), true],
  ["a 499", new ApiError(499, "request cancelled"), false],
  ["a 500", new ApiError(500, "server error"), true],
  ["a 503", new ApiError(503, "service unavailable"), true],
])("isTransientReadFailure of %s", (_, error, expected) => {
  expect(isTransientReadFailure(error)).toBe(expected);
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

test.each([
  ["a 400 with a reason", new ApiError(400, "x", { error: "名稱不能空白" }), "名稱不能空白"],
  ["a 499 with a reason", new ApiError(499, "x", { error: "請求已取消" }), "請求已取消"],
  ["a 413 with a reason", new ApiError(413, "x", { error: "超過上限" }), "超過上限"],
  [
    "a 401, which belongs to sign-in",
    new ApiError(401, "x", { error: "not authenticated" }),
    undefined,
  ],
  ["a 399", new ApiError(399, "x", { error: "不是錯誤" }), undefined],
  ["a 500", new ApiError(500, "x", { error: "資料庫連線中斷" }), undefined],
  ["a 400 whose body names no reason", new ApiError(400, "Bad Request", {}), undefined],
  ["a 400 with an empty reason", new ApiError(400, "Bad Request", { error: "" }), undefined],
  ["something that is not an API error", new Error("名稱不能空白"), undefined],
])("serverReason of %s", (_, error, want) => {
  expect(serverReason(error)).toBe(want);
});
