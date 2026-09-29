import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { createAppRouter } from "../../app/router";

let container: HTMLDivElement;
let root: Root;
let router: ReturnType<typeof createAppRouter>;

const SESSION_ID = "11111111-1111-4111-8111-111111111111";
const SESSION = {
  id: SESSION_ID,
  revision: 7,
  state: "waiting_input",
  snapshot: {
    messages: [],
    brief: "把每週報告整理成摘要",
    brief_confirmed: true,
    acceptance_criteria: ["保留決策與待辦"],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 500,
    reserved_credits: 0,
    spent_credits: 130,
    usage_unknown: false,
    steps: 2,
    tool_calls: 0,
  },
  created_at: "2026-09-28T00:00:00Z",
  updated_at: "2026-09-28T01:00:00Z",
  expires_at: "2026-10-05T00:00:00Z",
  deadline: "2026-09-28T02:00:00Z",
};
const LIMITS = {
  min_budget_credits: 130,
  max_budget_credits: 6500,
  max_steps: 20,
  max_tool_calls: 10,
  call_timeout_seconds: 120,
  session_timeout_seconds: 3600,
  retention_seconds: 604800,
};

beforeEach(() => {
  queryClient.clear();
  router = createAppRouter();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubMe(
  features?: Record<string, boolean>,
  route?: (path: string, init?: RequestInit) => Response | undefined,
) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me")
      return Promise.resolve(
        json({
          user_id: "u-1",
          email: "t@example.com",
          display_name: "tester",
          workspace_id: "ws-1",
          deletion_requested_at: null,
          purge_after: null,
          deletion_scope: null,
          ...(features ? { features } : {}),
        }),
      );
    return Promise.resolve(route?.(path, init) ?? json({}));
  });
}

function creationRoute(seen: string[], sessions = [SESSION]) {
  return (path: string, init?: RequestInit) => {
    seen.push(path);
    if (path === "/creation-sessions/limits") return json(LIMITS);
    if (path === "/me/credits") return json({ error: "not found" }, 404);
    if (path === "/creation-sessions") {
      return json(init?.method === "POST" ? SESSION : sessions);
    }
    if (path === `/creation-sessions/${SESSION_ID}`) return json(SESSION);
  };
}

async function visit(rendered: () => boolean, entry = "/workspace/creations") {
  router.update({ history: createMemoryHistory({ initialEntries: [entry] }) });
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
  const seen: string[] = [];
  stubMe(undefined, creationRoute(seen));
  await visit(
    () => text().includes("這一頁現在不存在"),
    `/workspace/creations?session=${SESSION_ID}`,
  );

  expect(text()).toContain("這一頁現在不存在");
  expect(container.querySelector("#generate-task"), "旗標關著卻渲染了生成表單").toBeNull();
  expect(text(), "旗標關著卻渲染了互動創作").not.toContain("和 Agent 一起創作");
  expect(
    seen.filter((path) => path.startsWith("/creation-sessions")),
    "旗標關著卻讀取了創作會話",
  ).toEqual([]);
  const hrefs = Array.from(container.querySelectorAll("a")).map((a) => a.getAttribute("href"));
  expect(hrefs, "沒有給一條回得去的路").toContain("/library");
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
  await visit(() => container.querySelector("main a[href='/library']") !== null);

  const back = Array.from(container.querySelectorAll("main a")).filter(
    (a) => a.getAttribute("href") === "/library",
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

test("a valid session URL reopens that exact server session", async () => {
  const seen: string[] = [];
  stubMe({ generate_skill: true, creation_skill: true }, creationRoute(seen));

  await visit(() => text().includes("等待你的補充"), `/workspace/creations?session=${SESSION_ID}`);

  expect(seen).toContain(`/creation-sessions/${SESSION_ID}`);
  expect(router.state.location.search.session).toBe(SESSION_ID);
  expect(text()).toContain("把每週報告整理成摘要");
});

test("an invalid session value stays on the session list and never requests a detail", async () => {
  const seen: string[] = [];
  stubMe({ generate_skill: true, creation_skill: true }, creationRoute(seen));

  await visit(() => text().includes("對話紀錄"), "/workspace/creations?session=not-a-uuid");

  expect(router.state.location.search.session).toBeUndefined();
  expect(seen).not.toContain("/creation-sessions/not-a-uuid");
  expect(text()).toContain("說出任務，一步步做成你的 Skill");
});

test("choosing a saved conversation sets its URL and starting new clears it", async () => {
  const seen: string[] = [];
  stubMe({ generate_skill: true, creation_skill: true }, creationRoute(seen));
  await visit(() => container.querySelector(`[data-session="${SESSION_ID}"]`) !== null);

  await act(async () => {
    container.querySelector<HTMLButtonElement>(`[data-session="${SESSION_ID}"]`)!.click();
  });
  await waitFor(() => router.state.location.search.session === SESSION_ID);

  expect(router.state.location.search.session).toBe(SESSION_ID);
  expect(seen).toContain(`/creation-sessions/${SESSION_ID}`);

  await act(async () => {
    [...container.querySelectorAll("button")]
      .find((button) => button.textContent === "＋ 開始新的創作")!
      .click();
  });
  await waitFor(() => router.state.location.search.session === undefined);

  expect(router.state.location.search.session).toBeUndefined();
  expect(text()).toContain("說出任務，一步步做成你的 Skill");
});

test("creating a session makes the returned session addressable", async () => {
  const seen: string[] = [];
  stubMe({ generate_skill: true, creation_skill: true }, creationRoute(seen, []));
  await visit(() => container.querySelector('input[name="creation-budget"]') !== null);

  await act(async () => {
    container
      .querySelector<HTMLInputElement>('input[name="creation-budget"][value="500"]')!
      .click();
    const task = container.querySelector<HTMLTextAreaElement>('[aria-label="想完成的任務"]')!;
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(
      task,
      "整理每週報告",
    );
    task.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    [...container.querySelectorAll("button")]
      .find((button) => button.textContent === "開始創作")!
      .click();
  });
  await waitFor(() => router.state.location.search.session === SESSION_ID);

  expect(router.state.location.search.session).toBe(SESSION_ID);
});
