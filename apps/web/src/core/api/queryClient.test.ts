import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { QueryState } from "@tanstack/react-query";
import { platformResponse } from "../../testing/fixtures/platform";
import { devLogin, logout } from "../session/me.service";
import { ApiError, SessionNotReadyError, apiFetch } from "./client";
import { queryClient } from "./queryClient";
import { queryKeys } from "./queryKeys";
import type { Me } from "./types";

const profile = platformResponse("/me").body as Me;
type SessionState = Partial<QueryState<Me, Error>>;

function session(state: SessionState) {
  queryClient
    .getQueryCache()
    .build<Me, Error>(queryClient, { queryKey: queryKeys.me })
    .setState(state);
}

beforeEach(() => {
  queryClient.clear();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(null, { status: 204 })),
  );
});
afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

for (const request of [
  { name: "a private deletion", path: "/skills/example", method: "DELETE" },
  { name: "an imperative creation", path: "/creation-sessions", method: "POST" },
]) {
  test.each<[string, SessionState]>([
    ["initial identity pending", { status: "pending", fetchStatus: "fetching" }],
    ["previous identity refreshing", { data: profile, status: "success", fetchStatus: "fetching" }],
    ["identity read paused", { data: profile, status: "success", fetchStatus: "paused" }],
    ["initial identity unavailable", { status: "error", error: new ApiError(503, "unavailable") }],
    [
      "previous identity unavailable",
      { data: profile, status: "error", error: new ApiError(503, "unavailable") },
    ],
    [
      "identity response lost",
      { data: profile, status: "error", error: new TypeError("Failed to fetch") },
    ],
    [
      "identity forbidden",
      { data: profile, status: "error", error: new ApiError(403, "forbidden") },
    ],
    [
      "previous identity signed out",
      { data: profile, status: "error", error: new ApiError(401, "unauthorized") },
    ],
  ])(`%s rejects ${request.name} before HTTP is sent`, async (_name, state) => {
    session(state);
    await expect(apiFetch(request.path, { method: request.method })).rejects.toBeInstanceOf(
      SessionNotReadyError,
    );
    expect(fetch).not.toHaveBeenCalled();
  });
}

test.each<[string, SessionState | undefined]>([
  ["confirmed identity", { data: profile, status: "success", fetchStatus: "idle" }],
  [
    "confirmed anonymous reader",
    { status: "error", error: new ApiError(401, "unauthorized"), fetchStatus: "idle" },
  ],
  ["client without a session query", undefined],
])("%s preserves existing write behavior", async (_name, state) => {
  if (state) session(state);
  await expect(apiFetch("/skills/example", { method: "DELETE" })).resolves.toBeUndefined();
  expect(fetch).toHaveBeenCalledTimes(1);
});

test("explicit logout remains available while identity is being checked", async () => {
  session({ data: profile, status: "success", fetchStatus: "fetching" });
  const request = vi.fn(async () => new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", request);
  await logout();
  expect(request).toHaveBeenCalledWith("/auth/logout", expect.objectContaining({ method: "POST" }));
});

test("explicit login remains available while identity is being checked", async () => {
  session({ data: profile, status: "success", fetchStatus: "fetching" });
  const request = vi.fn(async () => new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", request);
  await devLogin("second");
  expect(request).toHaveBeenCalledWith(
    "/auth/dev/login",
    expect.objectContaining({ method: "POST", body: '{"user":"second"}' }),
  );
});

test.each(["GET", "get", "HEAD", "OPTIONS"])(
  "%s reads remain usable while identity is being checked",
  async (method) => {
    session({ data: profile, status: "success", fetchStatus: "fetching" });
    await expect(apiFetch("/metadata", { method })).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

test.each(["POST", "post"])(
  "the public corrected search using %s remains usable while identity is unreadable",
  async (method) => {
    session({ data: profile, status: "error", error: new ApiError(503, "unavailable") });
    await expect(apiFetch("/api/skills/search", { method })).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

test("a default GET remains usable while identity is being checked", async () => {
  session({ data: profile, status: "success", fetchStatus: "fetching" });
  await expect(apiFetch("/metadata")).resolves.toBeUndefined();
  expect(fetch).toHaveBeenCalledTimes(1);
});

test("a lowercase write method still requires a confirmed session", async () => {
  session({ data: profile, status: "success", fetchStatus: "fetching" });
  await expect(apiFetch("/skills/import/url", { method: "post" })).rejects.toBeInstanceOf(
    SessionNotReadyError,
  );
  expect(fetch).not.toHaveBeenCalled();
});

test("a sessionless path does not exempt a different write method", async () => {
  session({ data: profile, status: "success", fetchStatus: "fetching" });
  await expect(apiFetch("/auth/logout", { method: "DELETE" })).rejects.toBeInstanceOf(
    SessionNotReadyError,
  );
  expect(fetch).not.toHaveBeenCalled();
});
