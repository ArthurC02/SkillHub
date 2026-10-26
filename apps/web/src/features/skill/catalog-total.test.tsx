import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { useCatalogTotal } from "./skills.service";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  queryClient.clear();
  vi.unstubAllGlobals();
});

function Count() {
  const query = useCatalogTotal({});
  return <output data-state={query.status}>{JSON.stringify(query.data)}</output>;
}

async function readCount(body: unknown) {
  const fetch = vi.fn(
    async (_input: RequestInfo | URL) => new Response(JSON.stringify(body), { status: 200 }),
  );
  vi.stubGlobal("fetch", fetch);
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <Count />
      </QueryClientProvider>,
    );
  });
  await pollUntil(
    () => {
      const state = container.querySelector("output")?.getAttribute("data-state");
      return state === "success" || state === "error";
    },
    () => container.innerHTML,
    DEFAULT_WAIT_MS,
    { flushBeforeFirstCheck: true },
  );
  expect(fetch.mock.calls).toHaveLength(1);
  expect(String(fetch.mock.calls[0][0])).toBe("/api/skills/catalog?limit=1");
  return container.querySelector("output")!;
}

test.each([
  ["zero", 0, "0"],
  ["largest safe integer", Number.MAX_SAFE_INTEGER, "9007199254740991"],
] as const)("catalog count accepts %s", async (_name, total, expected) => {
  const output = await readCount({ total });
  expect(output.getAttribute("data-state")).toBe("success");
  expect(output.textContent).toBe(expected);
});

test.each([
  ["null response", null],
  ["missing", {}],
  ["null", { total: null }],
  ["text", { total: "0" }],
  ["negative", { total: -1 }],
  ["fractional", { total: 0.5 }],
  ["unsafe integer", { total: 9007199254740992 }],
] as const)("catalog count rejects %s instead of reporting zero", async (_name, body) => {
  const output = await readCount(body);
  expect(output.getAttribute("data-state")).toBe("error");
  expect(output.textContent).toBe("");
});
