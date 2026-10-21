import { act, createElement, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { useSessionBoundary, useSignOut } from "./me.service";
import { queryKeys } from "../api/queryKeys";
import { ApiError } from "../api/client";
import { ME } from "../../testing/fixtures/platform";
import { pollUntil } from "../../testing/poll";

for (const boundary of [false, true]) {
  test(`logout schedules one reload with identity boundary=${boundary}`, async () => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity, gcTime: Infinity } },
    });
    client.setQueryData(queryKeys.me, ME);
    client.setQueryData(["private-session-data"], "private data");
    const clear = vi.spyOn(client, "clear");
    let release!: () => void;
    const reloaded = new Promise<void>((resolve) => (release = resolve));
    const reload = vi.fn(() => release());
    const originalWindow = window;
    const location = {
      href: originalWindow.location.href,
      origin: originalWindow.location.origin,
      reload,
    };
    vi.stubGlobal(
      "window",
      new Proxy(originalWindow, {
        get: (target, key) => (key === "location" ? location : Reflect.get(target, key, target)),
      }),
    );
    const requests: string[] = [];
    vi.stubGlobal("fetch", (url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      requests.push(`${method} ${url}`);
      return Promise.resolve(
        new Response(
          JSON.stringify({ error: method === "POST" ? "internal error" : "unauthorized" }),
          { status: method === "POST" ? 500 : 401 },
        ),
      );
    });
    const originalFetchQuery = client.fetchQuery.bind(client);
    if (boundary)
      vi.spyOn(client, "fetchQuery").mockImplementation(((options) =>
        originalFetchQuery(options).catch(async (error) => {
          await reloaded;
          throw error;
        })) as typeof client.fetchQuery);
    let trigger!: () => Promise<unknown>;
    let operation: Promise<unknown> | undefined;
    let mutationError: unknown;
    function Boundary() {
      useSessionBoundary();
      return null;
    }
    function Harness() {
      const logout = useSignOut();
      useEffect(() => {
        trigger = logout.mutateAsync;
      }, [logout.mutateAsync]);
      return boundary ? createElement(Boundary) : null;
    }
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    try {
      await act(async () =>
        root.render(createElement(QueryClientProvider, { client }, createElement(Harness))),
      );
      await act(async () => {
        operation = trigger().catch((error) => {
          mutationError = error;
        });
      });
      await pollUntil(
        () => reload.mock.calls.length > 0,
        () => JSON.stringify(requests),
        10000,
        { flushBeforeFirstCheck: true },
      );
      await act(async () => {
        await operation;
      });
      const observation = {
        boundary,
        reloads: reload.mock.calls.length,
        clears: clear.mock.calls.length,
        requests,
        privateCacheRemoved: client.getQueryData(["private-session-data"]) === undefined,
      };
      expect(requests.filter((request) => request === "POST /auth/logout")).toHaveLength(1);
      expect(mutationError).toBeInstanceOf(ApiError);
      expect(mutationError).toMatchObject({ status: 500 });
      expect(observation.privateCacheRemoved).toBe(true);
      expect(reload).toHaveBeenCalledTimes(1);
      expect(clear).toHaveBeenCalledTimes(1);
    } finally {
      release();
      await act(async () => root.unmount());
      client.clear();
      container.remove();
      vi.restoreAllMocks();
      vi.unstubAllGlobals();
    }
  });
}
