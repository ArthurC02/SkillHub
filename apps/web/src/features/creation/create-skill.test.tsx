import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { router } from "../../app/router";

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

function stubMe(features?: Record<string, boolean>) {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me")
      return Promise.resolve(
        new Response(
          JSON.stringify({
            user_id: "u-1",
            email: "t@example.com",
            display_name: "tester",
            workspace_id: "ws-1",
            deletion_requested_at: null,
            purge_after: null,
            deletion_scope: null,
            ...(features ? { features } : {}),
          }),
          { status: 200 },
        ),
      );
    return Promise.resolve(new Response(JSON.stringify({}), { status: 200 }));
  });
}

async function visit(rendered: () => boolean) {
  router.update({ history: createMemoryHistory({ initialEntries: ["/workspace/creations"] }) });
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
  });
  await waitFor(() => queryClient.getQueryState(queryKeys.me)?.status === "success" && rendered());
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const text = () => container.textContent ?? "";

test("⛔ with the flag off, /workspace/creations is not a workbench and says so", async () => {
  stubMe();
  await visit(() => text().includes("這一頁現在不存在"));

  expect(text()).toContain("這一頁現在不存在");
  expect(container.querySelector("#generate-task"), "旗標關著卻渲染了生成表單").toBeNull();
  expect(text(), "旗標關著卻渲染了互動創作").not.toContain("和 Agent 一起創作");
  const hrefs = Array.from(container.querySelectorAll("a")).map((a) => a.getAttribute("href"));
  expect(hrefs, "沒有給一條回得去的路").toContain("/workspace/skills");
});

test("with generate_skill on, the page is the generation workbench", async () => {
  stubMe({ generate_skill: true });
  await visit(() => container.querySelector("#generate-task") !== null);

  expect(container.querySelector("#generate-task")).not.toBeNull();
  expect(text()).not.toContain("這一頁現在不存在");
});

test.each([
  ["旗標開著", { generate_skill: true }],
  ["旗標關著", undefined],
] as const)("%s 時這一頁都有一條回得去的路", async (_label, features) => {
  stubMe(features);
  await visit(() => container.querySelector("main a[href='/workspace/skills']") !== null);

  const back = Array.from(container.querySelectorAll("main a")).filter(
    (a) => a.getAttribute("href") === "/workspace/skills",
  );
  expect(back.length, "這一頁沒有出口").toBeGreaterThan(0);
  expect(back.length, "回去的路出現了兩次").toBe(1);
});

test("with creation_skill on as well, the page is the conversation", async () => {
  stubMe({ generate_skill: true, creation_skill: true });
  await visit(() => text().includes("和 Agent 一起創作"));

  expect(text()).toContain("和 Agent 一起創作");
  expect(container.querySelector("#generate-task"), "兩個工作台同時掛上了").toBeNull();
});
