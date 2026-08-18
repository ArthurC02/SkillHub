import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { DataPolicy } from "./pages/DataPolicy";

// 02:O11Y-004 / 04 丙-25② — the data policy page. The one thing worth a test here
// is that it never invents a retention window: ADR-029's proposed 180 days is not
// ratified, and a page that printed it while the deployment collected nothing
// would be the 04 乙-2 mistake in a new place.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children?: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    if (settled()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`timed out; DOM was: ${container.textContent}`);
}

const text = () => container.textContent ?? "";

const EVENTS = [
  {
    name: "search_performed",
    when: "a search is submitted",
    attributes: ["query_length", "query_language"],
    not_recorded: "not one word of the query itself",
  },
];

test("O11Y-004 an unconfigured deployment says 目前不收集 rather than a proposed number", async () => {
  vi.stubGlobal("fetch", () =>
    json({ collecting: false, retention_days: 0, events: EVENTS, note: "no free-text column" }),
  );
  await render(<DataPolicy />, () => text().includes("目前不收集"));

  // NFR-002 made visible: no retention value, no collection.
  expect(text()).toContain("一列都不寫");
  expect(text()).not.toContain("180");
  // No window is stated at all, not even a zero-day one: the copy that names a
  // retention period belongs to the other branch and must not leak into this one.
  expect(text()).not.toContain("到期後刪除");
  // The four events are still disclosed — "we collect nothing" is a disclosure.
  expect(text()).toContain("search_performed");
  expect(text()).toContain("query_length");
});

test("O11Y-004 a collecting deployment states the window it actually applies", async () => {
  vi.stubGlobal("fetch", () =>
    json({ collecting: true, retention_days: 180, events: EVENTS, note: "no free-text column" }),
  );
  await render(<DataPolicy />, () => text().includes("180"));

  expect(text()).toContain("180 天");
  expect(text()).not.toContain("目前不收集");
  // The disclosure the whitelist alone would hide.
  expect(text()).toContain("not one word of the query itself");
});

test("O11Y-004 a failed read is a failed read, never an implied 'nothing is collected'", async () => {
  vi.stubGlobal("fetch", () => json({ error: "boom" }, 500));
  await render(<DataPolicy />, () => text().includes("無法讀取分析事件政策"));

  expect(text()).toContain("讀不到不等於沒有收集");
  expect(text()).not.toContain("目前不收集");
});
