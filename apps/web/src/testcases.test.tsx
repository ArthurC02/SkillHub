import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { TestCaseDetail, TestCaseList } from "./pages/TestCases";
import type { TestCase } from "./api/testcases";

// 03:TEST-012, whose bar is 02:TEST-001 第 3 條: 「使用者可新增、編輯、刪除及
// 確認驗收條件」. The four verbs have a user as their subject, so the test drives
// the buttons and asserts the requests that come out — an assertion about fields
// being present on screen would not have caught the state TEST-003 was handed
// back for.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  listSearch = {};
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const TEST_CASE = "33333333-3333-3333-3333-333333333333";
const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";

/** The list route's `?skill=`, controlled per test. */
let listSearch: { skill?: string } = {};

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ testCaseId: TEST_CASE }),
  useNavigate: () => () => Promise.resolve(),
  useSearch: () => listSearch,
  Link: ({ children }: { children?: unknown }) => children,
}));

const draft: TestCase = {
  test_case_id: TEST_CASE,
  skill_id: SKILL,
  name: "去重複列",
  user_prompt: "把重複的列刪掉，保留第一次出現的那一列。",
  acceptance_criteria: [
    {
      id: "c1",
      text: "輸出的列數少於輸入",
      source: "user",
      confirmed_at: "2026-08-17T01:00:00Z",
    },
  ],
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T01:00:00Z",
};

const RUN = {
  run_id: "99999999-9999-9999-9999-999999999999",
  status: "succeeded",
  skill_id: SKILL,
  skill_name: "去重複工具",
  skill_version_id: VERSION,
  test_case_id: TEST_CASE,
  provider: "self-hosted",
  cleanup_status: "cleaned",
  // 04 丙-32: the second axis. Required and never null — 未評估 is a value, not an
  // omission, because an empty verdict beside 「執行完成」 reads as a pass.
  evaluation: {
    value: "met",
    label: "符合",
    note: "依這個 Run 當時的驗收條件判定為符合。",
  },
  created_at: "2026-08-18T00:00:00Z",
  finished_at: "2026-08-18T00:04:00Z",
};

type Overrides = {
  /** POST .../criteria/suggest — proposals, or an ApiError status to fail with. */
  suggest?: { suggestions: { text: string }[] } | { status: number; error: string };
  runs?: unknown[];
  datasets?: unknown[];
  testCases?: unknown[];
};

function stubPlatform(over: Overrides = {}) {
  const calls: { url: string; method: string; body?: string }[] = [];
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    calls.push({ url, method: init?.method ?? "GET", body: init?.body as string | undefined });
    if (url.includes("/criteria/suggest")) {
      const s = over.suggest ?? { suggestions: [] };
      return "status" in s ? json({ error: s.error }, s.status) : json(s);
    }
    if (url.includes("/datasets")) return json({ datasets: over.datasets ?? [], total_bytes: 0 });
    if (path === "/runs") return json({ runs: over.runs ?? [] });
    if (path === "/skills")
      return json({ skills: [{ skill_id: SKILL, name: "去重複工具", summary: "" }] });
    if (path === "/test-cases") return json({ test_cases: over.testCases ?? [] });
    if (init?.method === "DELETE" && path === `/test-cases/${TEST_CASE}`)
      return json({ deleted: true, datasets_deleted: 2, note: "server note" });
    return json(draft);
  });

  return calls;
}

async function renderComponent(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
}

async function render() {
  await renderComponent(<TestCaseDetail />);
  await waitFor(() => !(container.textContent ?? "").includes("載入 Test Case 中"));
}

async function renderList() {
  await renderComponent(<TestCaseList />);
  await waitFor(() => !(container.textContent ?? "").includes("載入中…"));
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

function button(label: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === label,
  );
  if (!found) throw new Error(`no ${label} button; DOM was:\n${container.textContent}`);
  return found;
}

function setValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    input instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

test("02:TEST-001 第 3 條 a user can add, delete and withdraw the confirmation of a criterion", async () => {
  const calls = stubPlatform();
  await render();

  const box = container.querySelector<HTMLInputElement>("#new-criterion");
  expect(box).not.toBeNull();
  await act(async () => setValue(box as HTMLInputElement, "不得改動其他欄位"));
  await act(async () => button("新增").click());
  const added = calls.find((c) => c.method === "POST" && c.url.endsWith("/criteria"));
  expect(added?.body).toContain("不得改動其他欄位");

  // The stored criterion is confirmed, so the control on offer is the one that
  // withdraws it — confirming twice is not a thing a user can ask for.
  await act(async () => button("取消確認").click());
  const withdrawn = calls.find((c) => c.method === "PATCH" && c.url.includes("/criteria/c1"));
  expect(withdrawn?.body).toContain('"confirmed":false');

  // 設計系統 §2.8: deleting a criterion destroys text the user wrote, so it goes
  // through the same two-step control as every other destructive action — and
  // the first click must send nothing, because the scope sentence is the whole
  // disclosure and it is only on screen between the two clicks.
  await act(async () => button("刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  await act(async () => button("確認刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE" && c.url.includes("/criteria/c1"))).toBe(true);
});

test("CONTENT-007 a rubric line is written against a criterion and saved with its version", async () => {
  const calls = stubPlatform();
  await render();

  // The editor offers one box per acceptance criterion, so the item's id is the
  // criterion's by construction — the id correspondence is not something the
  // user can get wrong.
  const box = container.querySelector<HTMLTextAreaElement>("#rubric-c1");
  expect(box).not.toBeNull();
  await act(async () => setValue(box as HTMLTextAreaElement, "引出顯示列數變少的那一句。"));
  const version = container.querySelector<HTMLInputElement>("#rubric-version");
  await act(async () => setValue(version as HTMLInputElement, "content-007/writing/v1"));
  const evidence = container.querySelector<HTMLInputElement>("#rubric-evidence-c1");
  await act(async () => (evidence as HTMLInputElement).click());

  await act(async () => button("儲存 Rubric").click());
  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("rubric"));
  expect(saved?.body).toContain('"version":"content-007/writing/v1"');
  expect(saved?.body).toContain('"id":"c1"');
  expect(saved?.body).toContain('"evidence_required":true');
});

test("CONTENT-007 clearing every line removes the rubric rather than storing an empty one", async () => {
  const calls = stubPlatform();
  await render();

  // Nothing was typed, so there is no rubric — and "no rubric" is null, not an
  // empty item list, because the two are different statements to a reader.
  expect(container.textContent).toContain("儲存等於移除這個 Test Case 的 rubric");
  await act(async () => button("儲存 Rubric").click());
  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("rubric"));
  expect(saved?.body).toContain('"rubric":null');
});

test("iron rule 4 the screen says a run freezes a snapshot and that editing clears a confirmation", async () => {
  stubPlatform();
  await render();

  // The snapshot rule is on screen rather than assumed: editing a draft must not
  // read as rewriting what a past run executed.
  expect(container.textContent).toContain("凍結成快照");

  const edit = container.querySelector<HTMLInputElement>("#criterion-c1");
  expect(edit).not.toBeNull();
  await act(async () => setValue(edit as HTMLInputElement, "輸出的列數必須少於輸入"));

  // Confirmation applied to the old words, so the page warns before the save
  // rather than letting the cleared state look like a bug.
  expect(container.textContent).toContain("會清除這一條的確認");
});

// --- TEST-001 可選強化: 建議是預覽，採納才落庫 ------------------------------

test("TEST-002 asking for suggestions writes nothing; each one is adopted or ignored by the user", async () => {
  const calls = stubPlatform({
    suggest: { suggestions: [{ text: "保留欄位標題列" }, { text: "輸出仍是 CSV" }] },
  });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("尚未加入"));

  // The proposals are on screen and the draft is untouched: asking is not
  // writing, which is the whole point of the change (TEST-001 確認權在使用者).
  expect(container.textContent).toContain("保留欄位標題列");
  expect(container.textContent).toContain("輸出仍是 CSV");
  expect(container.textContent).toContain("尚未加入");
  expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/criteria"))).toBe(false);

  // Adopting one goes through the ordinary add route, labelled as the model's
  // wording and therefore unconfirmed.
  await act(async () => button("採納").click());
  await waitFor(() => calls.some((c) => c.method === "POST" && c.url.endsWith("/criteria")));
  const adopted = calls.find((c) => c.method === "POST" && c.url.endsWith("/criteria"));
  expect(adopted?.body).toContain("保留欄位標題列");
  expect(adopted?.body).toContain('"source":"suggested"');

  // The adopted one leaves the proposal list; the other is still on offer.
  await waitFor(() => !(container.textContent ?? "").includes("保留欄位標題列"));
  expect(container.textContent).toContain("輸出仍是 CSV");

  // Ignoring writes nothing at all.
  const before = calls.length;
  await act(async () => button("忽略").click());
  expect(container.textContent).not.toContain("輸出仍是 CSV");
  expect(calls.length).toBe(before);
});

test("TEST-002 an adopted suggestion is listed as 系統建議，尚未確認", async () => {
  // The draft the server answers with after the adoption: the label on the row
  // is what tells a reader these words are a model's, not their own.
  const withSuggested: TestCase = {
    ...draft,
    acceptance_criteria: [
      { id: "c9", text: "保留欄位標題列", source: "suggested", confirmed_at: null },
    ],
  };
  const json = (body: unknown) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.split("?")[0] === "/runs") return json({ runs: [] });
    if (url.includes("/datasets")) return json({ datasets: [], total_bytes: 0 });
    return json(withSuggested);
  });
  await render();

  expect(container.textContent).toContain("系統建議，尚未確認");
});

test("TEST-002 a 503 is worded as unavailable and names the manual path", async () => {
  stubPlatform({ suggest: { status: 503, error: "suggestions are unavailable right now" } });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("目前無法自動建議"));
  expect(container.textContent).toContain("可以自己手動輸入");
});

// --- 步驟 6: 回程 ------------------------------------------------------------

test("執行歷史 lists this test case's runs and links to each one", async () => {
  const calls = stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("執行歷史"));

  // Filtered server-side, not by reading the whole workspace and sieving here.
  expect(
    calls.some((c) => c.url.includes(`/runs?`) && c.url.includes(`test_case_id=${TEST_CASE}`)),
  ).toBe(true);
  expect(container.textContent).toContain("2026-08-18T00:00:00Z");
  expect(container.textContent).toContain(VERSION);
  // ADR-025: worded as execution, never as a pass.
  expect(container.textContent).toContain("執行完成");
  // Two axes with the verdict first (ADR-025, 04 丙-32). The old assertion was
  // for the footnote that stood in for the missing axis.
  expect(container.textContent).toContain("任務判定：符合");
  const t = container.textContent ?? "";
  expect(t.indexOf("任務判定")).toBeLessThan(t.indexOf("執行狀態"));
});

test("執行歷史 with no runs says 尚無執行 rather than rendering a zero", async () => {
  stubPlatform({ runs: [] });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("執行歷史"));

  expect(container.textContent).toContain("尚無執行");
  expect(container.textContent).toContain("要跑哪一個 Skill Version 在那個頁面上選");
  // The copy the picker made obsolete must be gone (丙-14).
  expect(container.textContent).not.toContain("還需要填入");
});

// --- WS-002: 刪除 -----------------------------------------------------------

test("02:WS-002 deleting states its scope before it runs and its actual reach after", async () => {
  const calls = stubPlatform({ runs: [] });
  await render();

  // Before: what goes and what survives, on screen, with nothing sent yet.
  await act(async () => button("刪除整個 Test Case").click());
  expect(container.textContent).toContain("已經跑過的 Run 及其快照不受影響");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除整個 Test Case").click());
  await waitFor(() => (container.textContent ?? "").includes("已刪除這個 Test Case"));
  const sent = calls.find((c) => c.method === "DELETE");
  expect(sent?.url).toContain(`/test-cases/${TEST_CASE}`);

  // After: the server's own count, not a guess, plus what it did not touch.
  expect(container.textContent).toContain("2 個上傳檔案");
  expect(container.textContent).toContain("快照與歷史 Run 不受影響");
  expect(container.textContent).toContain("回到 Test Case 列表");
});

// --- 步驟 1: 列表彙總與篩選 --------------------------------------------------

const LIST_ROW = {
  ...draft,
  skill_name: "去重複工具",
  criteria_confirmed: 1,
  criteria_total: 3,
  has_rubric: true,
};

test("列表 shows the skill's name, the confirmed count and whether a rubric exists", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(container.textContent).toContain("去重複工具");
  expect(container.textContent).toContain("已確認 1/3 條");
  expect(container.textContent).toContain("Rubric 有");
  // The bare UUID it replaces must not be back on the row.
  expect(container.textContent).not.toContain(SKILL);
});

test("列表 ?skill= narrows the request and says so, with a way back to the full list", async () => {
  listSearch = { skill: SKILL };
  const calls = stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(
    calls.some((c) => c.url.includes("/test-cases?") && c.url.includes(`skill_id=${SKILL}`)),
  ).toBe(true);
  expect(container.textContent).toContain("只顯示");
  expect(container.textContent).toContain("顯示全部");
});
