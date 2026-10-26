import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { emptySearchIntent } from "../../core/api/searchIntent";
import { CATALOG, SEARCH } from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { useCatalog, useCorrectedSkillSearch } from "./skills.service";

let container: HTMLDivElement;
let root: Root;
const correction = JSON.stringify({ intent: emptySearchIntent(), keywords: ["PDF"] });
type Mode = "catalog" | "search" | "corrected";

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  queryClient.clear();
  vi.unstubAllGlobals();
});

function Probe({ mode }: { mode: Mode }) {
  const catalog = useCatalog({}, mode === "catalog");
  const search = useCorrectedSkillSearch(
    "pdf",
    {},
    mode !== "catalog",
    mode === "corrected" ? correction : undefined,
  );
  const query = mode === "catalog" ? catalog : search;
  return (
    <>
      <output data-state={query.status}>{JSON.stringify(query.data)}</output>
      <button onClick={() => void query.refetch()}>retry</button>
    </>
  );
}

test.each(["catalog", "search", "corrected"] as const)(
  "%s rejects malformed success before caching and recovers on the same request",
  async (mode) => {
    let recovered = false;
    const calls: { url: string; method: string; body: unknown }[] = [];
    const valid = mode === "catalog" ? CATALOG : SEARCH;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: init?.body ? JSON.parse(String(init.body)) : null,
        });
        return new Response(JSON.stringify(recovered ? valid : null), { status: 200 });
      }),
    );
    await act(async () =>
      root.render(
        <QueryClientProvider client={queryClient}>
          <Probe mode={mode} />
        </QueryClientProvider>,
      ),
    );
    const output = container.querySelector("output")!;
    await pollUntil(
      () => output.getAttribute("data-state") === "error",
      () => container.innerHTML,
      DEFAULT_WAIT_MS,
      { flushBeforeFirstCheck: true },
    );
    expect(output.textContent).toBe("");
    expect(
      queryClient
        .getQueryCache()
        .getAll()
        .every((query) => query.state.data === undefined),
    ).toBe(true);
    expect(calls).toHaveLength(1);
    expect(calls[0]).toEqual(
      mode === "catalog"
        ? { url: "/api/skills/catalog?limit=100", method: "GET", body: null }
        : mode === "search"
          ? { url: "/api/skills/search?q=pdf&limit=20", method: "GET", body: null }
          : {
              url: "/api/skills/search",
              method: "POST",
              body: {
                query: "pdf",
                intent: { input: null, output: null, tools: null, data: null, environment: null },
                keywords: ["PDF"],
                filters: {},
                limit: 20,
              },
            },
    );
    recovered = true;
    await act(async () => container.querySelector("button")!.click());
    await pollUntil(
      () => output.getAttribute("data-state") === "success",
      () => container.innerHTML,
      DEFAULT_WAIT_MS,
      { flushBeforeFirstCheck: true },
    );
    expect(JSON.parse(output.textContent!)).toEqual(valid);
    expect(calls).toHaveLength(2);
    expect(calls[1]).toEqual(calls[0]);
  },
);
