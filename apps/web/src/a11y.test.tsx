import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import axe from "axe-core";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
/**
 * The fixtures live in `./fixtures/platform` so the browser tier can ask the
 * same questions of the same data (ADR-036). Only the seven ids and the one
 * body the tests below name directly are pulled in; the rest of the set is
 * reached through `platformResponse`.
 */
import {
  OTHER_RUN,
  RUN,
  SEARCH,
  SKILL,
  SKILL_B,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "./fixtures/platform";

/**
 * 03:QA-009 / DESIGN-013 — the accessibility check, run as a test so it keeps
 * being true. The bar is 02:NFR-007: 主要操作可使用鍵盤完成、表單具有標籤與清楚的
 * 驗證訊息、風險與相容與評估狀態必須同時提供文字。
 *
 * Every route in router.tsx is rendered against mocked API data and scanned with
 * axe-core; any violation fails the build. The last two sections of this file are
 * the keyboard walkthrough and the validation messages (04 丙-21 ①②), which axe
 * answers nothing about.
 *
 * **Adding a route? It needs a case here.** The list below used to be enumerated
 * by hand, which meant a new route was simply never scanned and nothing said so —
 * the quietest kind of hole, because the suite stayed green. `SCANNED_ROUTES` and
 * the test right under it now compare this file against the router's own route
 * table, so forgetting fails instead of passing (04 丙-22).
 *
 * Three things this cannot see, recorded here rather than switched off:
 *
 * 1. **Colour contrast.** axe answers `incomplete` for `color-contrast` on every
 *    node here, and importing index.css with `css: true` does not change that —
 *    jsdom computes no layout, so axe cannot resolve what is behind a pixel. The
 *    rule stays enabled (it is simply never decided); the palette was measured by
 *    hand instead, which is what removed the `opacity` mutes and the literal
 *    `#d33` from index.css. **A contrast regression will not fail this test.**
 * 2. **Page-level rules** (`html-has-lang`, `document-title`, …) do not run
 *    against an element context. `index.html` carries them and no page can
 *    change them at runtime.
 * 3. **The browser's own key handling.** jsdom does not turn Enter on a focused
 *    button into a click, does not implement Tab, and does not open a `<details>`
 *    on Enter over its `<summary>`. So the walkthrough below asserts the half a
 *    test can own — that every step of a journey is a native control, in the
 *    tab sequence, focusable, and that activating it moves the journey on — and
 *    leaves the half the platform owns to the platform. **What it cannot prove is
 *    that a real browser's Tab order matches DOM order**; nothing here fakes that.
 */

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

/** One fake platform for every route; ordered most specific first. */
function stubPlatform() {
  // The routing table moved to ./fixtures/platform; what stays here is the half
  // that is specific to this runner — turning a body into a `fetch` Response.
  vi.stubGlobal("fetch", (input: string) => {
    const { body, status } = platformResponse(String(input));
    return json(body, status);
  });
}

async function mount() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 4000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const has = (needle: string) => () => (container.textContent ?? "").includes(needle);

/**
 * Every rule axe knows, minus nothing: the WCAG 2.0/2.1/2.2 A and AA tags plus
 * the best-practice set. Lowering this list to make a page pass would be exactly
 * the move QA-009 exists to prevent.
 */
const TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"];

async function scan(where: string) {
  const results = await axe.run(container, {
    runOnly: { type: "tag", values: TAGS },
    resultTypes: ["violations"],
  });
  const report = results.violations.map(
    (v) =>
      `${v.id} (${v.impact}): ${v.help}\n    ${v.nodes
        .map((n) => n.html.slice(0, 160))
        .join("\n    ")}`,
  );
  expect(report, `${where} has accessibility violations:\n  ${report.join("\n  ")}`).toEqual([]);

  // Keyboard reach, asserted structurally because axe cannot press Tab: a
  // <details> without a <summary> is a disclosure no keyboard can open, and a
  // positive tabindex reorders focus away from reading order.
  const details = container.querySelectorAll("details");
  for (const node of details) {
    expect(
      node.querySelector(":scope > summary"),
      `${where}: <details> without <summary>`,
    ).not.toBeNull();
  }
  expect(
    container.querySelectorAll('[tabindex]:not([tabindex="0"]):not([tabindex="-1"])'),
    `${where}: positive tabindex breaks focus order`,
  ).toHaveLength(0);

  // system.md §2.4, in the principle's own words: the reason may be *in* a
  // tooltip, but never *only* there. A disabled control with no stated cause
  // reads as a bug, and `title` is invisible to touch and to most readers.
  for (const el of container.querySelectorAll("[disabled][title]")) {
    expect(
      el.getAttribute("aria-describedby"),
      `${where}: a disabled control whose reason exists only as a title tooltip (§2.4)`,
    ).not.toBeNull();
  }

  // system.md §2.3 / 02:NFR-007. Colour is the second channel and the border is
  // the third; the word is the first. A badge with no text has skipped to the
  // second and made the tint the fact.
  for (const badge of container.querySelectorAll(".badge")) {
    expect(
      badge.textContent?.trim(),
      `${where}: a badge with no word — colour is carrying the state alone`,
    ).not.toBe("");
  }

  // system.md §3 item 6 / §6 — the one rule with a known defect and no gate.
  // axe fails a *skipped* level, never a level that should have gone down and
  // didn't, so eight wrong `h2` in RunTrace lived for weeks. This does not
  // judge the outline; it publishes it, so the next wrong one arrives as a diff
  // somebody has to approve. Same shape as KNOWN_DEVIATIONS and SCANNED_ROUTES:
  // a checked-in list whose changes must be argued for.
  const outline = Array.from(container.querySelectorAll("h1,h2,h3,h4,h5,h6"))
    .map((h) => `${h.tagName.toLowerCase()} ${h.textContent?.trim().slice(0, 60)}`)
    .join("\n");
  await expect(outline).toMatchFileSnapshot(
    `./__outlines__/${where.replace(/[^\w一-鿿]+/g, "-").replace(/^-|-$/g, "")}.txt`,
  );
}

/**
 * The tab sequence as the platform would build it: native interactive elements
 * in DOM order, minus the ones a browser skips — disabled controls, anything
 * hidden from the accessibility tree, and the contents of a closed `<details>`
 * (its `<summary>` stays, which is how a keyboard opens it).
 *
 * Positive `tabindex` is asserted absent in scan(), so DOM order IS the sequence
 * here. That equivalence is the assumption this helper rests on, and it is the
 * one thing only a real browser can confirm (QA-008).
 */
const FOCUSABLE =
  'a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';

function tabbables(): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => {
    if (el.hasAttribute("disabled") || el.getAttribute("aria-hidden") === "true") return false;
    const closed = el.closest("details:not([open])");
    return closed === null || el === closed.querySelector(":scope > summary");
  });
}

/**
 * Reaches a control the way a keyboard user does — find it in the tab sequence,
 * put focus on it, activate it — and fails loudly when it is not in that
 * sequence at all. `click()` stands in for Enter/Space on a focused native
 * control, which is the browser behaviour jsdom does not implement.
 */
async function keyboardActivate(label: string, match: (el: HTMLElement) => boolean) {
  const target = tabbables().find(match);
  expect(target, `${label} is not reachable by keyboard`).toBeDefined();
  target!.focus();
  expect(document.activeElement, `${label} did not take focus`).toBe(target);
  await act(async () => target!.click());
}

const byText = (text: string) => (el: HTMLElement) => (el.textContent ?? "").includes(text);

// --- one case per route in router.tsx ---------------------------------------

/**
 * Every route this file scans below. Kept as data rather than as a comment so
 * the next test can hold it against the router itself: a route with no case here
 * is a page nobody checks, and the failure mode of the old hand-kept list was
 * that it looked exactly like a route with no problems.
 */
const SCANNED_ROUTES = [
  "/",
  "/compare",
  "/policy",
  "/skills/$skillId",
  "/skills/$skillId/files",
  "/skills/$skillId/package",
  "/lab/run",
  "/lab/datasets",
  "/lab/test-cases",
  "/lab/test-cases/$testCaseId",
  "/runs/$runId",
  "/runs/$runId/compare",
  "/workspace/account",
  "/workspace/downloads",
  "/workspace/import",
  "/workspace/runs",
  "/workspace/skills",
];

test("QA-009: Skill import", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/import" });
  });
  await waitFor(() => container.querySelector("form") !== null);
  await scan("/workspace/import");
}, 30000);

test("QA-009: 每一條路由都有一個掃描案例", () => {
  const declared = Object.keys(router.routesById).filter((id) => id !== "__root__");
  // Sorted rather than ordered: the router's order is its own business, and a
  // reshuffle of routeTree is not a reason to fail an accessibility suite.
  expect([...declared].sort(), "a route in router.tsx has no axe case in this file").toEqual(
    [...SCANNED_ROUTES].sort(),
  );
});

test("QA-009: 首頁與搜尋結果", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));
  await scan("/");
}, 30000);

test("NFR-007: 搜尋結果的即時區是筆數，不是整份清單", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));

  // A live region wrapping the result cards makes a reader recite every card in
  // full on each search — louder than announcing nothing, and the reason this
  // assertion exists rather than just the count one below.
  const list = container.querySelector(".search-results")!;
  expect(list.getAttribute("aria-live")).toBe(null);

  const count = Array.from(container.querySelectorAll('[role="status"]')).find((el) =>
    (el.textContent ?? "").includes("找到"),
  );
  expect(count?.textContent).toContain(`找到 ${SEARCH.results.length} 個 Skill`);
}, 30000);

test("QA-009: Skill 詳情", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: SKILL } });
  });
  await waitFor(has("可散布性與打包"));
  await scan("/skills/$skillId");
}, 30000);

test("QA-009: Skill 檔案（進階模式）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId/files", params: { skillId: SKILL } });
  });
  await waitFor(has("scripts/run.py"));
  await scan("/skills/$skillId/files");
}, 30000);

test("QA-009: 打包與下載", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/skills/$skillId/package",
      params: { skillId: SKILL },
      search: { version: VERSION },
    });
  });
  await waitFor(has("打包預覽"));
  await scan("/skills/$skillId/package");
}, 30000);

test("QA-009: 下載紀錄", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/downloads" });
  });
  await waitFor(has("pdf-summariser-v2.zip"));
  await scan("/workspace/downloads");

  // 02:WS-002 第 3 條 is a two-step delete, and the second step only exists
  // after the first: scan the confirming state too, and check that focus went
  // with it rather than being dropped on <body>.
  const ask = [...container.querySelectorAll("button")].find((b) => b.textContent === "刪除")!;
  await act(async () => ask.click());
  const confirm = [...container.querySelectorAll("button")].find(
    (b) => b.textContent === "確認刪除",
  );
  expect(document.activeElement).toBe(confirm);
  await scan("/workspace/downloads（確認刪除）");
}, 30000);

test("QA-009: 並排比較", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/compare", search: { ids: `${SKILL},${SKILL_B}` } });
  });
  await waitFor(() => container.querySelector("table.compare-table") !== null);
  await scan("/compare");
}, 30000);

test("QA-009: 執行前權限確認", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/lab/run",
      search: { skill: SKILL, version: VERSION, test_case: TEST_CASE },
    });
  });
  await waitFor(has("資源上限"));
  await scan("/lab/run");
}, 30000);

test("QA-009: Dataset 上傳", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: TEST_CASE } });
  });
  await waitFor(has("上傳前請先確認"));
  await scan("/lab/datasets");
}, 30000);

test("QA-009: Run 結果（一般與進階模式）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/runs/$runId", params: { runId: RUN } });
  });
  await waitFor(has("任務判定"));
  await waitFor(has("最終輸出"));
  await scan("/runs/$runId（一般模式）");

  const advanced = [...container.querySelectorAll("button")].find(
    (b) => b.textContent === "進階模式",
  )!;
  await act(async () => advanced.click());
  await waitFor(() => container.querySelector("table") !== null);
  await scan("/runs/$runId（進階模式）");
}, 30000);

test("QA-009: Run 比較", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/runs/$runId/compare",
      params: { runId: RUN },
      search: { against: OTHER_RUN },
    });
  });
  await waitFor(has("逐條驗收條件"));
  await scan("/runs/$runId/compare");
}, 30000);

test("QA-009: Test Case 列表", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/test-cases" });
  });
  await waitFor(has("建立新的 Test Case"));
  await scan("/lab/test-cases");
}, 30000);

test("QA-009: Test Case 詳情", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/lab/test-cases/$testCaseId",
      params: { testCaseId: TEST_CASE },
    });
  });
  await waitFor(has("Rubric（選用）"));
  await scan("/lab/test-cases/$testCaseId");
}, 30000);

test("QA-009: Run 歷史", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/runs" });
  });
  await waitFor(has("PDF Summariser"));
  await scan("/workspace/runs");
}, 30000);

test("QA-009: 我的 Skill", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/skills" });
  });
  await waitFor(has("我的 Skill"));
  await scan("/workspace/skills");
}, 30000);

test("QA-009: 帳號與刪除", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/account" });
  });
  await waitFor(has("刪除申請中"));
  await scan("/workspace/account");
}, 30000);

test("QA-009: 資料保存政策", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/policy" });
  });
  await waitFor(has("search_performed"));
  await scan("/policy");
}, 30000);

// --- 02:NFR-007「主要操作可使用鍵盤完成」: the walkthrough (04 丙-21 ①) ---------
//
// Journeys rather than pages: the four handbacks this project has recorded all
// happened between two steps, and a per-page check is exactly what cannot see a
// seam. Each step asserts the same three things — in the tab sequence, takes
// focus, activating it moves the journey on.

test("NFR-007: 搜尋 → 詳情 → 打包，全程鍵盤可達", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));

  // The search field and its submit are both in the sequence before anything is
  // typed: a form whose only route in is a mouse click on a suggestion is not
  // keyboard-operable, however well labelled it is.
  expect(tabbables().some((el) => el.tagName === "INPUT")).toBe(true);

  await keyboardActivate("搜尋結果連結", byText("PDF Summariser"));
  await waitFor(has("可散布性與打包"));

  await keyboardActivate("打包入口", byText("打包並下載這個版本"));
  await waitFor(has("標準 Agent Skill 套件")); // the heading renders before the targets do

  // On the packaging page: pick a target, choose whether test cases travel, and
  // build. Radio and checkbox are native inputs, so the platform gives them
  // arrow/space handling — what matters here is that they are reachable and that
  // the button they gate is not offered while the preview refuses.
  //
  // Matched by name rather than by type: the site-wide feedback form also has
  // radios, and an earlier version of this walkthrough passed by activating one
  // of those instead of a packaging target.
  await keyboardActivate("打包目標選項", (el) => el.getAttribute("name") === "packaging-target");
  await keyboardActivate("Test Case 選項", (el) => el.getAttribute("type") === "checkbox");
  await waitFor(has("這些設定可以打包"));

  const build = tabbables().find(byText("建立下載套件"));
  expect(build, "建立下載套件 is not reachable by keyboard").toBeDefined();
  build!.focus();
  expect(document.activeElement).toBe(build);
}, 30000);

test("NFR-007: 全站回報入口是一個 <details>，用鍵盤打得開也送得出", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/downloads" });
  });
  await waitFor(has("pdf-summariser-v2.zip"));

  // Closed: the summary is the only thing in the sequence, and the form inside
  // is deliberately not — a control a browser skips must not count as reachable.
  const summary = tabbables().find(byText("回報問題"));
  expect(summary?.tagName).toBe("SUMMARY");
  expect(tabbables().some((el) => el.id === "feedback-message")).toBe(false);

  await act(async () => (summary as HTMLElement).click());
  const opened = tabbables();
  expect(opened.some((el) => el.id === "feedback-message")).toBe(true);
  expect(opened.some((el) => el.getAttribute("type") === "submit")).toBe(true);
}, 30000);

// --- 02:NFR-007「清楚的驗證訊息」 (04 丙-21 ②) --------------------------------
//
// axe covers the labels; it says nothing about what a form does when it refuses.
// The rule these three share: a refusal names what to fix, and a control that
// cannot be used yet says why rather than sitting there dead.

test("NFR-007: 空白的回報被擋下來時說得出要補什麼", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/" });
  });
  await waitFor(has("回報問題"));

  const summary = tabbables().find(byText("回報問題"))!;
  await act(async () => summary.click());
  await act(async () => {
    container
      .querySelector(".feedback-entry form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });

  const alert = container.querySelector('.feedback-entry [role="alert"]');
  expect(alert?.textContent).toContain("內容不能空白");
  await scan("/（回報問題，驗證訊息）");
}, 30000);

test("NFR-007: 不能建立的 Test Case 表單說得出還缺哪幾項", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/test-cases" });
  });
  await waitFor(has("建立新的 Test Case"));

  // The disabled submit is not the message; the sentence beside it is.
  const submit = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "建立",
  )!;
  expect(submit.disabled).toBe(true);
  const status = Array.from(container.querySelectorAll('[role="status"]')).find((el) =>
    (el.textContent ?? "").includes("還不能建立"),
  );
  expect(status?.textContent).toContain("選一個 Skill");
  expect(status?.textContent).toContain("填名稱");
  expect(status?.textContent).toContain("寫 User Prompt");
}, 30000);

test("NFR-007: 沒選檔案就按上傳，說的是下一步而不是錯誤碼", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: TEST_CASE } });
  });
  await waitFor(has("上傳前請先確認"));

  await keyboardActivate("上傳", byText("上傳"));
  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("請先選擇一個檔案");
}, 30000);
