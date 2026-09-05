import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { CreationSession } from "./components/CreationSession";
import { CreateHub } from "./components/CreateHub";
import type { CreationSession as Session } from "./api/creation";
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));
vi.mock("./components/GenerateSkill", () => ({
  GenerateSkill: () => <div>舊生成入口</div>,
  ReferencePicker: ({ onToggle }: { onToggle: (id: string, name: string) => void }) => (
    <button onClick={() => onToggle("ref-1", "摘要 Skill")}>選擇摘要參考</button>
  ),
}));
let box: HTMLDivElement, root: Root, q: QueryClient;
const sample = (patch: Partial<Session> = {}): Session => ({
  id: "s1",
  revision: 7,
  state: "waiting_input",
  snapshot: {
    messages: [],
    brief: "摘要任務",
    brief_confirmed: false,
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_usd: 1,
    reserved_usd: 0.1,
    usage_unknown: true,
    steps: 1,
    tool_calls: 0,
  },
  created_at: "2026-09-05T00:00:00Z",
  updated_at: "2026-09-05T00:00:00Z",
  expires_at: "2026-09-06T00:00:00Z",
  ...patch,
});
const response = (v: unknown, status = 200) =>
  Promise.resolve(
    new Response(JSON.stringify(v), { status, headers: { "Content-Type": "application/json" } }),
  );
beforeEach(() => {
  box = document.createElement("div");
  document.body.appendChild(box);
  q = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});
afterEach(async () => {
  await act(async () => root?.unmount());
  q.clear();
  box.remove();
  vi.unstubAllGlobals();
});
async function waitFor(fn: () => boolean) {
  const until = Date.now() + 2500;
  while (!fn()) {
    if (Date.now() > until) throw Error(box.textContent ?? "timeout");
    await act(async () => new Promise((r) => setTimeout(r, 5)));
  }
}
async function render(node: ReactNode = <CreationSession />) {
  await act(async () => {
    root = createRoot(box);
    root.render(<QueryClientProvider client={q}>{node}</QueryClientProvider>);
  });
  await act(async () => new Promise((r) => setTimeout(r, 20)));
}
function button(text: string) {
  const found = [...box.querySelectorAll("button")].find((b) => b.textContent === text);
  if (!found) throw Error("button missing " + text);
  return found;
}
async function click(text: string) {
  await act(async () => button(text).click());
}
async function input(label: string, value: string) {
  const el = box.querySelector('[aria-label="' + label + '"]') as
    HTMLInputElement | HTMLTextAreaElement;
  const prototype =
    el.tagName === "TEXTAREA" ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function mode(index: number) {
  await act(async () => {
    (box.querySelectorAll('input[type="radio"]')[index] as HTMLInputElement).click();
  });
}
async function resume() {
  await waitFor(() => !!box.querySelector("select"));
  await act(async () => {
    const select = box.querySelector("select")!;
    select.value = "s1";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => box.textContent!.includes("創作狀態"));
}
test("natural language creates one budgeted session", async () => {
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(sample({ state: "queued" }));
      }
      return response(url.endsWith("/creation-sessions") ? [] : sample());
    }),
  );
  await render();
  await input("這次預算上限（美元）", ".5");
  await input("想完成的任務", "建立摘要 Skill");
  await click("開始互動創作");
  await waitFor(() => posts.length === 1);
  expect(posts[0]).toMatchObject({ message: "建立摘要 Skill", budget_usd: 0.5 });
  expect(posts[0].id).toBeTruthy();
});
test("diagram starts with an unbilled empty session then sends transient input", async () => {
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(sample({ revision: posts.length }));
      }
      return response(url.endsWith("/creation-sessions") ? [] : sample());
    }),
  );
  await render();
  await input("這次預算上限（美元）", ".5");
  await mode(1);
  const file = box.querySelector('input[type="file"]') as HTMLInputElement;
  await act(async () => {
    Object.defineProperty(file, "files", {
      value: [new File(["private diagram"], "flow.png", { type: "image/png" })],
    });
    file.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await click("開始互動創作");
  await waitFor(() => posts.length === 2);
  expect(posts[0].message).toBe("");
  expect(posts[1]).toMatchObject({
    kind: "diagram",
    expected_revision: 1,
    diagram: { media_type: "image/png", data: btoa("private diagram") },
  });
});
test("catalog references can start a session and require confirmation", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample();
  v.snapshot.pending_action = "confirm_references";
  v.snapshot.references = [
    { skill_id: "ref-1", version_id: "v1", name: "摘要 Skill", available: true, confirmed: false },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(posts.length === 1 ? sample({ revision: 1 }) : v);
      }
      return response(url.endsWith("/creation-sessions") ? [] : v);
    }),
  );
  await render();
  await input("這次預算上限（美元）", ".5");
  await mode(2);
  await click("選擇摘要參考");
  await click("開始互動創作");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toMatchObject({ kind: "select_references", reference_skill_ids: ["ref-1"] });
  await waitFor(() => box.textContent!.includes("確認參考 Skill"));
  await click("確認參考 Skill");
  await waitFor(() => posts.length === 3);
  expect(posts[2]).toMatchObject({ kind: "confirm_references", expected_revision: 7 });
});
test("resume shows unknown costs and confirms the displayed diagram revision", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.diagram_understanding = "開始 → 摘要";
  v.snapshot.pending_action = "confirm_diagram";
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(v);
      }
      return response(url.endsWith("/creation-sessions") ? [v] : v);
    }),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("已知費用 未知");
  expect(box.textContent).toContain("不能當作零");
  await click("確認流程圖理解");
  expect(posts[0]).toMatchObject({ kind: "confirm_diagram", expected_revision: 7 });
});
test("409 preserves input and needs an explicit action with the refreshed revision", async () => {
  const posts: Record<string, unknown>[] = [];
  let current = sample();
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        if (posts.length === 1) {
          current = sample({ revision: 8 });
          return response({ error: "stale" }, 409);
        }
        return response(current);
      }
      return response(url.endsWith("/creation-sessions") ? [current] : current);
    }),
  );
  await render();
  await resume();
  await input("想完成的任務", "保留這個修訂");
  await click("送出素材");
  await waitFor(() => box.textContent!.includes("輸入仍保留"));
  expect((box.querySelector("textarea") as HTMLTextAreaElement).value).toBe("保留這個修訂");
  expect(posts).toHaveLength(1);
  await click("送出素材");
  await waitFor(() => posts.length === 2);
  expect(posts[1].expected_revision).toBe(8);
  expect(posts[1].command_id).not.toBe(posts[0].command_id);
});
test("network retry reuses the command ID and payload", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample();
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return posts.length === 1
          ? Promise.reject(new TypeError("network unavailable"))
          : response(v);
      }
      return response(url.endsWith("/creation-sessions") ? [v] : v);
    }),
  );
  await render();
  await resume();
  await input("想完成的任務", "重試同一個修改");
  await click("送出素材");
  await waitFor(() => !!box.querySelector('[role="alert"]'));
  await click("送出素材");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toEqual(posts[0]);
});
test("flag off never mounts creation or fetches its private sessions", async () => {
  const fetch = vi.fn(() => response([]));
  vi.stubGlobal("fetch", fetch);
  await render(<CreateHub generateExposed={false} creationExposed={true} />);
  expect(box.textContent).not.toContain("和 Agent 一起創作 Skill");
  expect(fetch).not.toHaveBeenCalled();
});
