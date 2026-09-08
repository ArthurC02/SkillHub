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
 * since it also ends in neither of the other two suffixes.
 *
 * CRED-001's GET /me/credits also fires on every render (CreationSession
 * calls useCredits unconditionally) and defaults to the real, current
 * production answer — 404, the route is not mounted yet (see api/credits.ts)
 * — so every existing test below is unaffected unless it opts into a credits
 * response of its own. */
function routeGet(url: string, list: unknown, single: unknown) {
  if (url.endsWith("/creation-sessions/limits")) return response(LIMITS);
  if (url.endsWith("/me/credits")) return response({ error: "not found" }, 404);
  return response(url.endsWith("/creation-sessions") ? list : single);
}
beforeEach(() => {
  box = document.createElement("div");
  document.body.appendChild(box);
  // jsdom has no object URLs. The component holds the sent `File` behind one
  // because the platform keeps the digest and refuses the bytes, so a test that
  // wants to see a thumbnail has to supply the browser half.
  URL.createObjectURL = vi.fn((blob: Blob) => "blob:" + String((blob as File).name));
  URL.revokeObjectURL = vi.fn();
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
/**
 * 2026-09-08：素材種類不再由一組 radio 先選，而是由輸入區裡放了什麼推出來
 * （`CreationSession` 的 `submit`）。文字框與「附一張流程圖」永遠在，所以只剩
 * 「參考目錄裡的 Skill」還需要先按開它的挑選器。
 */
async function openReferencePicker() {
  await click("參考目錄裡的 Skill");
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
// CRED-001 (ADR-068) gate ①: a new session may start only when the balance
// meets the estimated threshold. These two are the DB-free half of the CRED
// test coverage this task asked for; apiserver/credits_gate_test.go and
// credits_route_test.go cover the Go side (pure gating logic and the
// RequireSession/RequireOperator HTTP surface).
const creditsResponse = (patch: Record<string, unknown> = {}) => ({
  balance_credits: 100,
  debt_floor_credits: -50,
  estimated_session: { low_credits: 30, high_credits: 65, sample_size: 40, estimated: false },
  can_start: true,
  ...patch,
});
test("a balance at or above the threshold leaves the start button enabled and shows the estimate", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (url.endsWith("/me/credits")) return response(creditsResponse());
      if (init?.method === "POST") return response(sample({ state: "queued" }));
      return routeGet(url, [], sample());
    }),
  );
  await render();
  await waitFor(() => box.textContent!.includes("目前餘額"));
  expect(box.textContent).toContain("目前餘額 100 點");
  expect(box.textContent).toContain("30–65 點");
  expect(button("開始互動創作").disabled).toBe(false);
});
test("a balance below the threshold disables the start button and names the deficit", async () => {
  const reason =
    "餘額不足以開始新的創作會話：目前 10 點，這一場大約要 65 點，還差 55 點。請聯絡 operator 授予點數，或等待下次充值。";
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (url.endsWith("/me/credits")) {
        return response(
          creditsResponse({ balance_credits: 10, can_start: false, block_reason: reason }),
        );
      }
      return routeGet(url, [], sample());
    }),
  );
  await render();
  await waitFor(() => box.textContent!.includes("還差"));
  expect(box.textContent).toContain(reason);
  const submit = button("開始互動創作");
  expect(submit.disabled).toBe(true);
  expect(submit.getAttribute("aria-describedby")).toBe("creation-credits-why-disabled");
});
/** 把一張圖放進輸入區。回傳那個 `<input>`，因為有一支測試要看它的 `value`。 */
async function attachDiagram(name = "flow.png", body = "diagram") {
  const el = box.querySelector('input[type="file"]') as HTMLInputElement;
  await act(async () => {
    Object.defineProperty(el, "files", {
      configurable: true,
      value: [new File([body], name, { type: "image/png" })],
    });
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  return el;
}
/**
 * 2026-09-08：三個素材入口收成一個輸入區之後，「一次只送一種」變成程式的責任而不是
 * 使用者的。平台的 action 一次帶一個 kind，送完一輪會話就進 working、下一個 action
 * 要等新的 revision——所以兩種素材同時在的時候要**先擋下來並說出順序**，不是連送兩次
 * 讓第二次撞 409。這支測試守的是「一個 POST 都沒有發出去」，不只是那句話有出現。
 *
 * ── 稍晚同日：這裡只剩「圖＋參考」──────────────────────────────────────
 * 文字曾經也算一種素材，因為 `diagram` 與 `select_references` 兩個 action 不收
 * `message`。現在收了（creation.go 的 `attachNote`），所以擋的只剩真正的兩個 kind。
 */
test("a diagram and reference Skills at once are refused before anything is sent", async () => {
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
  await openReferencePicker();
  await click("選擇摘要參考");
  await attachDiagram();
  await click("開始互動創作");
  await waitFor(() => box.textContent!.includes("一次只能送一種"));
  expect(box.textContent).toContain("文字說明可以跟著任一種一起送");
  expect(posts, "擋下來之前就已經送出去了").toHaveLength(0);
});
/**
 * 圖和「這張圖是要做什麼」是同一句話的兩半。後端收下之後（creation.go 的
 * `attachNote`），這裡守的是前端真的把它們放進**同一個** action，而不是擋下來叫人
 * 分兩次送。
 */
test("a diagram carries the sentence that came with it, in one action", async () => {
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
  await input("想完成的任務", "這是我的流程，我想把它變成待辦清單 Skill。");
  await attachDiagram();
  await click("開始互動創作");
  await waitFor(() => posts.length === 2);
  expect(posts[0].message, "會話本身不帶那句話，它跟著素材走").toBe("");
  expect(posts[1]).toMatchObject({
    kind: "diagram",
    message: "這是我的流程，我想把它變成待辦清單 Skill。",
    diagram: { media_type: "image/png", data: btoa("diagram") },
  });
});
/**
 * 「同一批的對話傳送」的另一半：圖不只要和文字**一起送出去**，還要和文字**一起
 * 出現在對話裡**。在這之前它送得出去但看不到——對話裡只有你的文字，圖變成畫面
 * 別處的一句「已附上流程圖」。
 *
 * 縮圖只可能來自這個瀏覽器自己手上那份 `File`：平台留指紋、不留位元組（ADR-066
 * 決策 4），沒有任何端點會把圖送回來。所以這支測試走完整條路——真的送出去、拿回
 * 帶著 `attachments` 的快照、再看那一則訊息裡有沒有圖。
 */
test("a picture and the words it came with are one turn in the conversation", async () => {
  const sent = sample({ revision: 2 });
  sent.snapshot.messages = [{ role: "user", content: "這是我的流程，幫我做成 Skill。" }];
  sent.snapshot.attachments = [
    { message_index: 0, media_type: "image/png", bytes: 7, sha256: "digest-1" },
  ];
  let posts = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts += 1;
        return response(posts === 1 ? sample({ revision: 1 }) : sent);
      }
      return routeGet(url, [], sent);
    }),
  );
  await render();
  await input("這次預算上限（美元）", ".5");
  await input("想完成的任務", "這是我的流程，幫我做成 Skill。");
  // 送出前就看得到縮圖：那是唯一能回答「我選到的是不是我要的那張」的東西。
  await attachDiagram();
  expect(box.querySelector("img.chip-thumb"), "輸入區裡沒有預覽").not.toBe(null);
  await click("開始互動創作");
  // 注意不能等文字：textarea 自己就帶著它。等對話本身出現。
  await waitFor(() => !!box.querySelector(".creation-log"));
  const mine = [...box.querySelectorAll('.creation-log > li[data-role="user"]')];
  expect(mine).toHaveLength(1);
  expect(mine[0].textContent).toContain("這是我的流程，幫我做成 Skill。");
  expect(mine[0].querySelector("img"), "圖沒有和它的文字在同一則訊息裡").not.toBe(null);
});
/**
 * 兩件事一起守：①沒打字的上傳自己是一塊，位置在模型回話之前——它確實發生在那兩輪
 * 之間；②**第二次上傳不會把第一次從歷史裡抹掉**。第二點是 `attachments` 這個清單
 * 存在的理由：`diagram_fingerprint` 那三個欄位是「最新那一張」，一個對話不能弄丟
 * 自己的回合。
 */
test("a second picture does not erase the first, and a wordless one is its own turn", async () => {
  const v = sample({ revision: 3 });
  v.snapshot.messages = [
    { role: "assistant", content: "我看到一張流程圖。" },
    { role: "user", content: "再看看這一張。" },
    { role: "assistant", content: "兩張都讀到了。" },
  ];
  v.snapshot.attachments = [
    { message_index: 0, media_type: "image/png", bytes: 7, sha256: "digest-1" },
    { message_index: 1, media_type: "image/webp", bytes: 9, sha256: "digest-2" },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  const blocks = [...box.querySelectorAll(".creation-attachments")];
  expect(blocks, "第二張把第一張蓋掉了").toHaveLength(2);
  const rows = [...box.querySelectorAll(".creation-log > li")];
  // 沒打字的那一張排在模型那句話**之前**，而帶文字的那一張在你自己的訊息**裡**。
  expect(rows[0].getAttribute("data-role")).toBe("user");
  expect(rows[0].querySelector(".creation-attachments")).not.toBe(null);
  expect(rows[0].textContent).not.toContain("我看到一張流程圖");
  expect(rows[1].textContent).toContain("我看到一張流程圖");
  const second = rows.find((r) => r.textContent!.includes("再看看這一張。"))!;
  expect(second.querySelector(".creation-attachments")).not.toBe(null);
});
/**
 * 換一台裝置、或只是重新整理，那張圖就不在了——平台不保存原圖。這時候那一輪不能
 * 變成空白，也不能假裝有圖：它說出附了什麼，以及為什麼看不到。
 */
test("a conversation without the browser that sent the picture describes it instead", async () => {
  const v = sample({ revision: 2 });
  v.snapshot.messages = [{ role: "user", content: "這是我的流程。" }];
  v.snapshot.attachments = [
    { message_index: 0, media_type: "image/png", bytes: 1234, sha256: "digest-elsewhere" },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.querySelector(".creation-attachment img"), "沒有人手上有這張圖").toBe(null);
  expect(box.textContent).toContain("image/png");
  expect(box.textContent).toContain("1234 位元組");
  expect(box.textContent).toContain("平台不保存原圖");
});
/**
 * 送出成功之後，輸入區裡的東西要清乾淨——**參考 Skill 一直沒有清**。留下來的 chip
 * 會被下一次的守門讀成「你又挑了參考」：你想補一句話，卻被擋，而錯誤訊息叫你去做你
 * 剛剛做完的事。這支測試不看 chip，看的是下一次送出真的是一則 `message`。
 */
test("references are cleared once they have been sent, so the next turn can be words", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample({ revision: 2 });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(JSON.parse(String(init.body)));
        return response(sample({ revision: posts.length + 1 }));
      }
      return routeGet(url, [], v);
    }),
  );
  await render();
  await input("這次預算上限（美元）", ".5");
  await openReferencePicker();
  await click("選擇摘要參考");
  await click("開始互動創作");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toMatchObject({ kind: "select_references", reference_skill_ids: ["ref-1"] });
  await waitFor(() => !box.textContent!.includes("移除參考"));
  await input("想完成的任務", "請照這個風格，但輸出成表格。");
  await click("送出");
  await waitFor(() => posts.length === 3);
  expect(posts[2]).toMatchObject({ kind: "message", message: "請照這個風格，但輸出成表格。" });
});
/**
 * 附加素材的兩個控制項，四件都要對：
 *
 * 1. 檔案輸入**沒有** `aria-label`。它原本掛著「流程圖」，蓋掉可見的「附一張流程圖」，
 *    於是語音操作念畫面上的字點不到它（WCAG 2.5.3）。
 * 2. 兩個上限那句話有 `id`，而且兩個控制項都 `aria-describedby` 指著它。
 * 3. 那句話在控制項**之前**——它原本在送出鍵之後，那時候它已經不是在講上限，是在
 *    解釋失敗。
 * 4. 展開鈕的 `aria-controls` 指的元素，展開之後真的在。
 */
test("the two attachment controls name themselves and carry their limits", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [], sample())),
  );
  await render();
  const fileEl = box.querySelector('input[type="file"]') as HTMLInputElement;
  expect(fileEl.getAttribute("aria-label"), "它會蓋掉看得見的那五個字").toBe(null);
  expect(fileEl.closest("label")!.textContent).toContain("附一張流程圖");
  expect(fileEl.getAttribute("aria-describedby")).toBe("composer-limits");
  const picker = button("參考目錄裡的 Skill");
  expect(picker.getAttribute("aria-controls")).toBe("composer-references");
  expect(picker.getAttribute("aria-describedby")).toBe("composer-limits");
  const limits = box.querySelector("#composer-limits")!;
  expect(limits.textContent).toContain("4,000,000");
  expect(
    limits.compareDocumentPosition(button("開始互動創作")) & Node.DOCUMENT_POSITION_FOLLOWING,
    "上限說在送出鍵之後就不是在講上限，是在解釋失敗",
  ).toBeTruthy();
  expect(box.querySelector("#composer-references")).toBe(null);
  await openReferencePicker();
  expect(box.querySelector("#composer-references"), "aria-controls 指著一個不存在的 id").not.toBe(
    null,
  );
});
/**
 * `<input type="file">` 是非受控的：`setFile(undefined)` 只清掉 React 那一份，DOM
 * 的 `value` 還握著同一個路徑，於是**再選同一張圖不會觸發 `change`**——移除之後那張
 * 圖就再也選不回來了。
 *
 * jsdom 不模擬檔案輸入的 `value`（設不進去也讀不出來），所以這裡直接看元件有沒有把
 * 它清成空字串：那正是瀏覽器要的那一個動作，也正是 `GenerateSkill.tsx` 一直在做的。
 */
test("removing the diagram clears the file input, not just React's copy", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [], sample())),
  );
  await render();
  const el = await attachDiagram();
  let cleared = 0;
  Object.defineProperty(el, "value", {
    configurable: true,
    get: () => "",
    set: (next: string) => {
      if (next === "") cleared += 1;
    },
  });
  await waitFor(() => box.textContent!.includes("移除流程圖：flow.png"));
  await click("移除流程圖：flow.png");
  expect(cleared, "只清了 React 那一份，同一張圖再也選不回來").toBeGreaterThan(0);
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
      tier: "curated",
      scan_status: "scanned",
      warnings: 2,
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
  await openReferencePicker();
  await click("選擇摘要參考");
  await click("開始互動創作");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toMatchObject({ kind: "select_references", reference_skill_ids: ["ref-1"] });
  await waitFor(() => box.textContent!.includes("以這些為參考"));
  expect(box.textContent).toContain("整理輸入並輸出摘要");
  expect(box.textContent).toContain("需要文字輸入");
  expect(box.textContent).toContain("Bash");
  expect(box.textContent).toContain("固定版本");
  expect(box.textContent).toContain("精選");
  expect(box.textContent).toContain("2 個警告");
  expect(box.textContent).not.toContain("secret-model");
  expect(box.textContent).not.toContain("secret-prompt");
  const urls = (fetch as unknown as { mock: { calls: [string, RequestInit?][] } }).mock.calls.map(
    ([url]) => url,
  );
  expect(urls.some((url) => /latest|detail|files/.test(url))).toBe(false);
  await click("以這些為參考");
  await waitFor(() => posts.length === 3);
  expect(posts[2]).toMatchObject({ kind: "confirm_references", expected_revision: 7 });
});
test("catalog hit on the first message offers adopt, confirm, or decline", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample();
  v.snapshot.catalog_checked = true;
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
  expect(box.textContent).toContain("目錄裡已有相近的 Skill");
  await click("直接採用");
  await waitFor(() => posts.length === 1);
  expect(posts[0]).toMatchObject({
    kind: "adopt_reference",
    reference_skill_ids: ["ref-1"],
  });
  await click("都不是，從頭寫");
  await waitFor(() => posts.length === 2);
  expect(posts[1]).toMatchObject({ kind: "decline_references" });
});
test("materialize-time duplicates offer adopt or confirm and hide the private-candidate button", async () => {
  const posts: Record<string, unknown>[] = [];
  const v = sample({ state: "draft_ready" });
  v.snapshot.pending_action = "confirm_duplicate";
  v.snapshot.pending_materialize = "materialize";
  v.snapshot.draft = {
    revision: 1,
    content_hash: "b".repeat(64),
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
  v.snapshot.duplicates = [
    {
      skill_id: "dup-1",
      version_id: "v1",
      name: "既有摘要 Skill",
      available: true,
      confirmed: false,
      description: "整理輸入並輸出摘要",
      scan_status: "unavailable",
    },
  ];
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
  expect(box.textContent).toContain("目錄已有相近的 Skill");
  expect(box.textContent).toContain("既有摘要 Skill");
  expect(box.textContent).toContain("沒有掃描紀錄");
  expect(() => button("建立私人候選版本")).toThrow();
  await click("仍然建立");
  await waitFor(() => posts.length === 1);
  expect(posts[0]).toMatchObject({
    kind: "confirm_duplicate",
    content_hash: "b".repeat(64),
  });
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
  await click("送出");
  await waitFor(() => box.textContent!.includes("輸入仍保留"));
  expect((box.querySelector("textarea") as HTMLTextAreaElement).value).toBe("保留這個修訂");
  expect(posts).toHaveLength(1);
  await click("送出");
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
  await click("送出");
  await waitFor(() => !!box.querySelector('[role="alert"]'));
  expect(box.textContent).toContain("網路連線失敗");
  await click("送出");
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
test("a model-overwritten brief shows both the new and the confirmed-away text (05 R-54 #4)", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.pending_action = "confirm_brief";
  v.snapshot.brief = "整理輸入並輸出十行摘要";
  v.snapshot.model_changed = { brief: "整理輸入並輸出三行摘要" };
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).toContain("整理輸入並輸出十行摘要");
  expect(box.textContent).toContain("原本是");
  expect(box.textContent).toContain("整理輸入並輸出三行摘要");
  expect(box.textContent).toContain("我看過差異，確認新的需求摘要");
});
test("a confirm_brief screen with no model_changed never says 原本是", async () => {
  const v = sample({ state: "waiting_confirmation" });
  v.snapshot.pending_action = "confirm_brief";
  v.snapshot.brief = "整理輸入並輸出摘要";
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => routeGet(url, [v], v)),
  );
  await render();
  await resume();
  expect(box.textContent).not.toContain("原本是");
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
  // GEN-010: a candidate with no run attached must still say it was not tried.
  expect(box.textContent).toContain("這份草稿尚未試跑");
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
