import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { CreationSession } from "./components/CreationSession";
import { CreateHub } from "./components/CreateHub";
import type { CreationSession as Session } from "./api/creation";
import { useCreationEntryPoint } from "./api/creation";
vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    search,
  }: {
    children: ReactNode;
    to: string;
    search?: Record<string, unknown>;
  }) => (
    <a href={to} data-search={search ? JSON.stringify(search) : undefined}>
      {children}
    </a>
  ),
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
    acceptance_criteria: [],
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
  deadline: "2026-09-05T01:00:00Z",
  ...patch,
});
const response = (v: unknown, status = 200) =>
  Promise.resolve(
    new Response(JSON.stringify(v), { status, headers: { "Content-Type": "application/json" } }),
  );
const LIMITS = {
  min_budget_usd: 0.1,
  max_budget_usd: 5,
  max_steps: 20,
  max_tool_calls: 10,
  call_timeout_seconds: 120,
  session_timeout_seconds: 3600,
  retention_seconds: 604800,
};
/** Every GET fires against one of three routes; `/limits` is checked first
 * since it also ends in neither of the other two suffixes. */
function routeGet(url: string, list: unknown, single: unknown) {
  if (url.endsWith("/creation-sessions/limits")) return response(LIMITS);
  return response(url.endsWith("/creation-sessions") ? list : single);
}
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
      return routeGet(url, [], sample());
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
      return routeGet(url, [], sample());
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
test("structured diagram understanding renders all four sections", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.diagram_understanding = JSON.stringify({
    nodes: ["開始", "整理"],
    conditions: [],
    branches: ["完成"],
    uncertainties: [],
  });
  v.snapshot.pending_action = "confirm_diagram";
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(v);
      }
      return routeGet(url, [v], v);
    }),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("節點");
  expect(box.textContent).toContain("條件");
  expect(box.textContent).toContain("分支");
  expect(box.textContent).toContain("不確定處");
  expect(box.textContent).toContain("未列出");
  expect(box.textContent).toContain("開始");
  expect(box.textContent).toContain("完成");
  await click("確認流程圖理解");
  expect(posts[0]).toMatchObject({ kind: "confirm_diagram", expected_revision: 7 });
});
test("legacy diagram understanding stays visible with refresh notice", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.diagram_understanding = "開始 → 摘要";
  v.snapshot.pending_action = "confirm_diagram";
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("開始 → 摘要");
  expect(box.textContent).toContain("請在對話要求重新整理，或重新上傳後確認");
  expect(box.textContent).not.toContain("節點");
  expect(button("確認流程圖理解").disabled).toBe(true);
});
test("catalog references can start a session and require confirmation", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample();
  v.snapshot.pending_action = "confirm_references";
  v.snapshot.references = [
    {
      skill_id: "ref-1",
      version_id: "v1",
      name: "摘要 Skill",
      available: true,
      confirmed: false,
      description: "整理輸入並輸出摘要",
      compatibility: "需要文字輸入",
      allowed_tools: "Bash",
    },
  ];
  v.snapshot.model = "secret-model";
  v.snapshot.prompt_version = "secret-prompt";
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(posts.length === 1 ? sample({ revision: 1 }) : v);
      }
      return routeGet(url, [], v);
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
  expect(box.textContent).toContain("整理輸入並輸出摘要");
  expect(box.textContent).toContain("需要文字輸入");
  expect(box.textContent).toContain("Bash");
  expect(box.textContent).toContain("固定版本");
  expect(box.textContent).not.toContain("secret-model");
  expect(box.textContent).not.toContain("secret-prompt");
  const urls = (fetch as unknown as { mock: { calls: [string, RequestInit?][] } }).mock.calls.map(
    ([url]) => url,
  );
  expect(urls.some((url) => /latest|detail|files/.test(url))).toBe(false);
  await click("確認參考 Skill");
  await waitFor(() => posts.length === 3);
  expect(posts[2]).toMatchObject({ kind: "confirm_references", expected_revision: 7 });
});
test("resume shows unknown costs and confirms the displayed diagram revision", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.diagram_understanding = JSON.stringify({
    nodes: ["開始", "摘要"],
    conditions: [],
    branches: [],
    uncertainties: [],
  });
  v.snapshot.pending_action = "confirm_diagram";
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(v);
      }
      return routeGet(url, [v], v);
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
      return routeGet(url, [current], current);
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
      return routeGet(url, [v], v);
    }),
  );
  await render();
  await resume();
  await input("想完成的任務", "重試同一個修改");
  await click("送出素材");
  await waitFor(() => !!box.querySelector('[role="alert"]'));
  expect(box.textContent).toContain("網路連線失敗");
  await click("送出素材");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toEqual(posts[0]);
});
test("budget band is shown and an out-of-band amount is refused locally", async () => {
  const posts: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(sample({ state: "queued" }));
      }
      return routeGet(url, [], sample());
    }),
  );
  await render();
  await waitFor(() => box.textContent!.includes("之間"));
  expect(box.textContent).toContain("介於 $ 0.1 與 $ 5 之間");
  await input("這次預算上限（美元）", "50");
  await input("想完成的任務", "超出預算的任務");
  await click("開始互動創作");
  await waitFor(() => !!box.querySelector('[role="alert"]'));
  expect(box.textContent).toContain("介於 $0.1 與 $5 之間");
  expect(posts).toHaveLength(0);
});
test("an open session shows its deadline and retention", async () => {
  const v = sample();
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  const deadline = box.querySelector(`time[datetime="${v.deadline}"]`);
  const expires = box.querySelector(`time[datetime="${v.expires_at}"]`);
  expect(deadline).toBeTruthy();
  expect(expires).toBeTruthy();
});
test("a failed or unevaluated run warns before saving instead of claiming no run happened", async () => {
  const v = sample({
    state: "candidate_ready",
    revision: 9,
  });
  v.snapshot.draft = {
    revision: 1,
    content_hash: "a".repeat(64),
    skill: {
      name: "摘要",
      description: "",
      compatibility: "",
      allowed_tools: "",
      body: "",
      files: [],
    },
    validation: JSON.stringify({ findings: [], blocked: false }),
    blocked: false,
  };
  v.snapshot.candidate = { skill_id: "sk-1", version_id: "v1", run_id: "run-1" };
  v.snapshot.messages = [
    {
      role: "tool",
      content: JSON.stringify({
        run_id: "run-1",
        execution_status: "failed",
        evaluation: { evaluation_available: false },
      }),
    },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("試跑未通過或未評估");
  expect(box.textContent).not.toContain("這份草稿尚未試跑");
});
test("acceptance criteria render under the brief and the confirm button names both", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.pending_action = "confirm_brief";
  v.snapshot.acceptance_criteria = ["輸入摘要後輸出重點條列", "字數不超過 200 字"];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("驗收條件");
  expect(box.textContent).toContain("輸入摘要後輸出重點條列");
  expect(box.textContent).toContain("字數不超過 200 字");
  expect(box.textContent).toContain("確認需求摘要與驗收條件");
});
test("a failed session shows the raise form, refuses an out-of-band amount locally, and posts a valid one", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample({ state: "failed" });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(v);
      }
      return routeGet(url, [v], v);
    }),
  );
  await render();
  await resume();
  await waitFor(() => !!box.querySelector('[aria-label="提高這次預算上限（美元）"]'));
  await input("提高這次預算上限（美元）", "50");
  await click("提高預算後繼續");
  await waitFor(() => !!box.querySelector('[role="alert"]'));
  expect(box.textContent).toContain("不超過 $5");
  expect(posts).toHaveLength(0);
  await input("提高這次預算上限（美元）", "2");
  await click("提高預算後繼續");
  await waitFor(() => posts.length === 1);
  expect(posts[0]).toMatchObject({ kind: "raise_budget", budget_usd: 2 });
});
test("a candidate with a test_case_id renders the Test Case sentence and the run link carries it", async () => {
  const v = sample({ state: "candidate_ready" });
  v.snapshot.draft = {
    revision: 1,
    content_hash: "a".repeat(64),
    skill: {
      name: "摘要",
      description: "",
      compatibility: "",
      allowed_tools: "",
      body: "",
      files: [],
    },
    validation: JSON.stringify({ findings: [], blocked: false }),
    blocked: false,
  };
  v.snapshot.candidate = { skill_id: "sk-1", version_id: "v1", test_case_id: "tc-1" };
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("已依確認的驗收條件建立 Test Case");
  const link = [...box.querySelectorAll("a")].find(
    (a) => a.textContent === "檢查權限與費用後試跑此版本",
  );
  expect(JSON.parse(link?.getAttribute("data-search") ?? "{}")).toMatchObject({
    test_case: "tc-1",
  });
});
function EntryPointProbe() {
  return <>{String(useCreationEntryPoint())}</>;
}
function stubMe(features: Record<string, boolean>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => response({ features })),
  );
}
test("⛔ useCreationEntryPoint is false unless /me says creation_skill is exactly true — generate_skill alone", async () => {
  stubMe({ generate_skill: true });
  await render(<EntryPointProbe />);
  expect(box.textContent).toBe("false");
});
test("⛔ useCreationEntryPoint is false unless /me says creation_skill is exactly true — no features", async () => {
  stubMe({});
  await render(<EntryPointProbe />);
  expect(box.textContent).toBe("false");
});
test("⛔ useCreationEntryPoint is false unless /me says creation_skill is exactly true — both on", async () => {
  stubMe({ generate_skill: true, creation_skill: true });
  await render(<EntryPointProbe />);
  expect(box.textContent).toBe("true");
});
test("flag off never mounts creation or fetches its private sessions", async () => {
  const fetch = vi.fn(() => response([]));
  vi.stubGlobal("fetch", fetch);
  await render(<CreateHub generateExposed={false} creationExposed={true} />);
  expect(box.textContent).not.toContain("和 Agent 一起創作 Skill");
  expect(fetch).not.toHaveBeenCalled();
});
