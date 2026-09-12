import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import axe from "axe-core";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
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

function stubPlatform() {
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

  for (const el of container.querySelectorAll("[disabled][title]")) {
    expect(
      el.getAttribute("aria-describedby"),
      `${where}: a disabled control whose reason exists only as a title tooltip (§2.4)`,
    ).not.toBeNull();
  }

  const SELF_EXPLAINING = ["送出中…", "已送出，無法取消", "打包中…", "載入中…", "重新整理中…"];
  for (const el of container.querySelectorAll("button[disabled], select[disabled]")) {
    const label = (el.textContent ?? "").trim();
    if (SELF_EXPLAINING.includes(label)) continue;
    expect(
      el.getAttribute("aria-describedby") ?? el.getAttribute("title"),
      `${where}: 「${label}」 is disabled and says why to nobody — ` +
        `wire the sentence beside it with aria-describedby, or state the cause in the label (§2.4)`,
    ).not.toBeNull();
  }

  for (const badge of container.querySelectorAll(".badge")) {
    expect(
      badge.textContent?.trim(),
      `${where}: a badge with no word — colour is carrying the state alone`,
    ).not.toBe("");
  }

  const tips = container.querySelectorAll("[data-tip]");
  expect(
    tips.length,
    `${where}: ${tips.length} Tips on one page — 一頁至多三個 (§2.13 第 5 條)：十個問號和沒有問號一樣沒有指向`,
  ).toBeLessThanOrEqual(3);
  const NOT_AN_ANCHOR = ["?", "？", "詳情", "說明", "更多", "為什麼", "說明？", "為什麼？"];
  for (const tip of tips) {
    const trigger = tip.querySelector(":scope > button.tip-trigger");
    expect(trigger, `${where}: a [data-tip] without its own button`).not.toBeNull();
    const label = (trigger!.textContent ?? "").trim();
    expect(
      NOT_AN_ANCHOR.includes(label),
      `${where}: Tip anchor 「${label}」 does not stand on its own (§2.13 第 3 條) — name the subject`,
    ).toBe(false);
    expect(
      label.length,
      `${where}: a Tip trigger with no visible text (§2.13 第 4 條)`,
    ).toBeGreaterThan(1);
    expect(
      trigger!.getAttribute("title"),
      `${where}: a Tip carried in title= (§2.13 第 2 條)`,
    ).toBeNull();
    const content = tip.querySelector(":scope > p.tip-content");
    expect(
      content,
      `${where}: Tip content is not a <p> — §4.7 says a <div> hides from the 40em measure`,
    ).not.toBeNull();
    expect(
      trigger!.getAttribute("aria-controls"),
      `${where}: the Tip button does not point at its content`,
    ).toBe(content!.id);
    expect(
      content!.querySelector(".note, .badge, [role=status], [role=alert]"),
      `${where}: a qualifier, a claim or live text inside a Tip — A／B／C／G never go in (§2.13 第 1 條)`,
    ).toBeNull();
  }

  for (const svg of container.querySelectorAll("svg[aria-hidden='true']")) {
    const beside = (svg.parentElement?.textContent ?? "").replace(/\s+/g, "");
    expect(beside, `${where}: an icon with no visible word beside it (§4.7)`).not.toBe("");
  }
  expect(
    container.querySelectorAll("svg:not([aria-hidden='true'])"),
    `${where}: an <svg> that is not aria-hidden — §4.7 icons never carry meaning on their own`,
  ).toHaveLength(0);

  for (const region of container.querySelectorAll("[role=alert], [role=status]")) {
    const text = (region.textContent ?? "").replace(/[\s\d\p{P}\p{S}]/gu, "");
    if (text === "") continue;
    expect(
      /\p{Script=Han}/u.test(text),
      `${where}: a live region with no Chinese in it — the server's English reached the screen: 「${(region.textContent ?? "").trim()}」`,
    ).toBe(true);
  }

  const REPEATED_QUALIFIER: string[] = [];
  const qualifiers = new Map<string, number>();
  for (const note of container.querySelectorAll(".note")) {
    if (note.closest("li, td, th")) continue;
    const text = (note.textContent ?? "").replace(/\s+/g, " ").trim();
    if (text.length < 8) continue;
    qualifiers.set(text, (qualifiers.get(text) ?? 0) + 1);
  }
  const saidTwice = [...qualifiers.entries()]
    .filter(([text, n]) => n > 1 && !REPEATED_QUALIFIER.includes(text))
    .map(([text, n]) => `x${n} 「${text.slice(0, 44)}」`);
  expect(
    saidTwice,
    `${where}: the same qualifier stated more than once on one screen (§3 第 14 條)`,
  ).toEqual([]);

  const LIST_NOTE_REPEATS: string[] = [
    "平台不曾執行它們——這是靜態掃描的結果,不是行為分析。",
    "版本：v2（最新）",
  ];
  const listRepeats: string[] = [];
  for (const list of container.querySelectorAll("ul, ol")) {
    const perList = new Map<string, number>();
    for (const item of list.querySelectorAll(":scope > li")) {
      const inThisRow = new Set<string>();
      for (const note of item.querySelectorAll(".note")) {
        if (note.closest("ul, ol") !== list) continue;
        if (note.tagName === "LABEL") continue;
        const text = (note.textContent ?? "").replace(/\s+/g, " ").trim();
        if (text.length < 8) continue;
        inThisRow.add(text);
      }
      for (const text of inThisRow) perList.set(text, (perList.get(text) ?? 0) + 1);
    }
    for (const [text, n] of perList) {
      if (n > 1 && !LIST_NOTE_REPEATS.includes(text)) {
        listRepeats.push(`x${n} 「${text.slice(0, 44)}」`);
      }
    }
  }
  expect(
    listRepeats.sort().join("\n"),
    `${where}: one sentence repeated on every row of a list (§2.13 去重 1). ` +
      `Print it once above the list — but only if every row still wears a word ` +
      `that maps back to it; otherwise add it to LIST_NOTE_REPEATS with the reason`,
  ).toBe("");

  const outline = Array.from(container.querySelectorAll("h1,h2,h3,h4,h5,h6"))
    .map((h) => `${h.tagName.toLowerCase()} ${h.textContent?.trim().slice(0, 60)}`)
    .join("\n");
  await expect(outline).toMatchFileSnapshot(
    `./__outlines__/${where.replace(/[^\w一-鿿]+/g, "-").replace(/^-|-$/g, "") || "index"}.txt`,
  );
}

const FOCUSABLE =
  'a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';

function tabbables(): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => {
    if (el.hasAttribute("disabled") || el.getAttribute("aria-hidden") === "true") return false;
    const closed = el.closest("details:not([open])");
    return closed === null || el === closed.querySelector(":scope > summary");
  });
}

async function keyboardActivate(label: string, match: (el: HTMLElement) => boolean) {
  const target = tabbables().find(match);
  expect(target, `${label} is not reachable by keyboard`).toBeDefined();
  target!.focus();
  expect(document.activeElement, `${label} did not take focus`).toBe(target);
  await act(async () => target!.click());
}

const byText = (text: string) => (el: HTMLElement) => (el.textContent ?? "").includes(text);

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
  "/workspace/creations",
  "/workspace/downloads",
  "/workspace/import",
  "/workspace/runs",
  "/workspace/skills",
  "/admin",
  "/admin/accounts",
  "/admin/skills",
  "/admin/dispatch",
  "/admin/rosters",
  "/admin/audit-log",
  "/admin/cost-statistics",
  "/admin/trends",
];

function stubOperator() {
  vi.stubGlobal("fetch", (input: string) => {
    const { body, status } = platformResponse(String(input));
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    return json(path === "/me" ? { ...(body as object), operator: true } : body, status);
  });
}

for (const [to, heading] of [
  ["/admin", "營運後台"],
  ["/admin/accounts", "帳號與點數"],
  ["/admin/dispatch", "停止派送"],
  ["/admin/rosters", "這個部署沒有設定封測名單"],
  ["/admin/audit-log", "授予點數"],
  ["/admin/cost-statistics", "搜尋理由"],
  ["/admin/trends", "全平台目前餘額總和"],
] as const) {
  test(`QA-009: 後台 ${to}`, async () => {
    stubOperator();
    await mount();
    await act(async () => {
      await router.navigate({ to });
    });
    await waitFor(has(heading));
    await scan(to);
  }, 30000);
}

test("QA-009: 後台 /admin/skills（查到一個）", async () => {
  stubOperator();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/admin/skills", search: { q: SKILL } });
  });
  await waitFor(has("對「PDF Summariser」的動作"));
  await scan("/admin/skills");
}, 30000);

test("QA-009: Skill import", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/import" });
  });
  await waitFor(() => container.querySelector("form") !== null);
  await scan("/workspace/import");
}, 30000);

test("QA-009: 創作（旗標關著）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/creations" });
  });
  await waitFor(has("這一頁現在不存在"));
  await scan("/workspace/creations");
}, 30000);

test("QA-009: 每一條路由都有一個掃描案例", () => {
  const declared = Object.keys(router.routesById).filter((id) => id !== "__root__");
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

test("QA-009: 首頁的目錄狀態（02:DISC-006）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/" });
  });
  await waitFor(has("目錄裡有什麼"));
  await scan("/ 目錄");
}, 30000);

test("NFR-007: 搜尋結果的即時區是筆數，不是整份清單", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));

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

test("NFR-007: 搜尋 → 詳情 → 打包，全程鍵盤可達", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));

  expect(tabbables().some((el) => el.tagName === "INPUT")).toBe(true);

  await keyboardActivate("搜尋結果連結", byText("PDF Summariser"));
  await waitFor(has("打包並下載這個版本"));

  await keyboardActivate("打包入口", byText("打包並下載這個版本"));
  await waitFor(has("標準 Agent Skill 套件"));

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

  const summary = tabbables().find(byText("回報問題"));
  expect(summary?.tagName).toBe("SUMMARY");
  expect(tabbables().some((el) => el.id === "feedback-message")).toBe(false);

  await act(async () => (summary as HTMLElement).click());
  const opened = tabbables();
  expect(opened.some((el) => el.id === "feedback-message")).toBe(true);
  expect(opened.some((el) => el.getAttribute("type") === "submit")).toBe(true);
}, 30000);

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

test("QA-009: 我的 Skill（未登入）", async () => {
  vi.stubGlobal("fetch", () => json({ error: "not authenticated" }, 401));
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/skills" });
  });
  await waitFor(has("需要登入"));

  expect(container.textContent).not.toContain("not authenticated");
  await scan("/workspace/skills 401");
}, 30000);

test("QA-009: 執行前權限確認（載入中）", async () => {
  vi.stubGlobal("fetch", () => new Promise(() => {}));
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/lab/run",
      search: { skill: SKILL, version: undefined, test_case: TEST_CASE },
    });
  });
  await waitFor(() => container.querySelector("[data-loading]") !== null);

  await scan("/lab/run loading");
}, 30000);

test("QA-009: Run 歷史（空的）", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    if (
      String(input)
        .replace(/^https?:\/\/[^/]+/, "")
        .split("?")[0] === "/runs"
    )
      return json({ runs: [] });
    const { body, status } = platformResponse(String(input));
    return json(body, status);
  });
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/runs" });
  });
  await waitFor(has("代表沒有發生過"));

  await scan("/workspace/runs empty");
}, 30000);
