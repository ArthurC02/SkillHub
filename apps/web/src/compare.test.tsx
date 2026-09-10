import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { RunCompare } from "./pages/RunCompare";
import { Compare, CompareTable } from "./pages/Compare";
import {
  COMPARISON,
  comparisonSide,
  OTHER_RUN,
  RUN,
  RUNS,
  TEST_CASE,
  platformResponse,
  skillDetail,
} from "./fixtures/platform";

const FOREIGN_RUN = {
  ...RUNS.runs[1],
  run_id: "0f0f0f0f-1111-4222-8333-444455556666",
  status_reason: "這是別的 Test Case 的 Run。",
  created_at: "2026-01-01T00:00:00Z",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  search = {};
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  navigations.length = 0;
});

const navigations: { search?: { against?: string } }[] = [];

let search: { against?: string; ids?: string } = {};

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: RUN }),
  useSearch: () => search,
  useNavigate: () => (options: { search?: { against?: string } }) => {
    navigations.push(options);
    return Promise.resolve();
  },
  Link: ({ children }: { children?: unknown }) => children as never,
}));

const pathOf = (url: string) => url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function stubPlatform(calls?: string[]) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls?.push(url);
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
}

test("EVAL-003 waits for the Test Case before loading candidate runs", async () => {
  const calls: string[] = [];
  stubPlatform(calls);
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  const runLists = calls.filter((url) => pathOf(url) === "/runs");
  expect(runLists.length).toBeGreaterThan(0);
  expect(runLists.every((url) => url.includes(`test_case_id=${TEST_CASE}`))).toBe(true);
});

async function render(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => queryClient.isFetching() === 0 && (container.textContent ?? "").length > 0);
}

async function waitFor(done: () => boolean, timeoutMs = 4000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (done()) return;
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const text = () => container.textContent ?? "";

const candidateButtons = () =>
  Array.from(container.querySelectorAll("button")).filter((b) =>
    (b.textContent ?? "").startsWith("與這一次比較"),
  );

test("DISC-009 comparison gives absent fields their actual state", async () => {
  const missingURL = skillDetail("a", "A");
  missingURL.source = { ...missingURL.source!, url: undefined };
  missingURL.version = undefined;
  missingURL.enrichment.tags = undefined;
  const pending = skillDetail("b", "B");
  pending.enrichment = { status: "pending", note: "正在處理" };
  await render(<CompareTable skills={[missingURL, pending]} />);

  expect(text()).toContain("來源網址：未測量");
  expect(text()).not.toContain("未提供");
  for (const heading of ["輸入", "輸出", "依賴"]) {
    const row = Array.from(container.querySelectorAll("tbody tr")).find((candidate) =>
      candidate.querySelector("th")?.textContent?.startsWith(heading),
    );
    expect(row, `${heading} row`).toBeDefined();
    const cells = row!.querySelectorAll("td");
    expect(cells[0].textContent, `${heading} enriched absence`).toBe("未測量");
    expect(cells[1].textContent, `${heading} pending absence`).toBe("處理中");
  }
  const versionRow = Array.from(container.querySelectorAll("tbody tr")).find((row) =>
    row.querySelector("th")?.textContent?.includes("版本與時間"),
  );
  expect(versionRow).toBeDefined();
  expect(versionRow?.querySelector("td")?.textContent).toBe("不適用");
});

test("DISC-009 最小匯入的 Skill：不印表外詞「未提供」", async () => {
  const minimal = skillDetail("m", "M");
  minimal.summary = "";
  minimal.allowed_tools = undefined;
  minimal.source = undefined;
  minimal.version = undefined;
  minimal.limitations = [];
  const full = skillDetail("f", "F");
  await render(<CompareTable skills={[minimal, full]} />);

  expect(text()).not.toContain("未提供");
  expect(text()).toContain("未測量");
});

test("DISC-009 生成的套件：來源網址是不適用，不是未測量", async () => {
  const generated = skillDetail("g", "G");
  generated.source = {
    ...generated.source!,
    url: undefined,
    trust: { value: "generated", label: "平台生成", note: "" },
  };
  const imported = skillDetail("i", "I");
  imported.source = { ...imported.source!, url: undefined };
  await render(<CompareTable skills={[generated, imported]} />);

  expect(text()).toContain("來源網址：不適用");
  expect(text()).toContain("來源網址：未測量");
});

test("DISC-009 相容性不同的兩個 Skill,那一列要說有差異", async () => {
  const runs = skillDetail("a", "A");
  const doesNot = skillDetail("b", "B");
  doesNot.compatibility = {
    ...doesNot.compatibility,
    spec_validation: { value: "failed", label: "未通過", note: "" },
    capability: { value: "unverified", label: "未驗證", note: "" },
  };
  await render(<CompareTable skills={[runs, doesNot]} />);

  const row = Array.from(container.querySelectorAll("tbody tr")).find((candidate) =>
    candidate.querySelector("th")?.textContent?.startsWith("相容性"),
  );
  expect(row, "相容性 row").toBeDefined();
  expect(row!.querySelector("th")?.textContent).toContain("有差異");
  expect(row!.className).toContain("compare-differs");
});

test("EVAL-003 到站時就有同一個 Test Case 的候選,而且認得出它的不是 uuid", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  expect(text()).toContain("任務判定：不符合");
  expect(
    candidateButtons()[0].querySelector('time[datetime="2026-08-16T00:00:00Z"]'),
  ).not.toBeNull();

  expect(text()).not.toContain("這是別的 Test Case 的 Run。");

  const list = container.querySelector("ul.download-list");
  expect(list?.textContent ?? "").not.toContain(OTHER_RUN);

  expect(container.querySelector("#against")).not.toBeNull();
  expect(text()).not.toContain("這個 Test Case 目前只有這一次 Run");
});

test("EVAL-003 自己不在候選裡:自比自伺服器會回 400", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  const buttons = candidateButtons();
  expect(buttons.length, "候選清單把這一邊自己也列了進去").toBe(1);
  expect(buttons.map((b) => b.textContent).join("|")).not.toContain("2026-08-17");
});

test("EVAL-003 選了候選就寫進網址,比較仍然可以被連結分享", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  await act(async () => candidateButtons()[0].click());

  expect(navigations.length).toBe(1);
  expect(navigations[0].search?.against).toBe(OTHER_RUN);
});

test("EVAL-003 只跑過一次的 Test Case:說原因,不是把控制項拿掉(設計 §2.4)", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") return json({ runs: [RUNS.runs[0]] });
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);
  await waitFor(() => text().includes("這個 Test Case 目前只有這一次 Run"));

  expect(candidateButtons().length).toBe(0);
  expect(container.querySelector("#against")).not.toBeNull();
  expect(text()).toContain("輸入另一個 Run 的 ID 後開始比較。");
});

test("EVAL-003 目前 Run 讀取失敗不會冒充沒有其他 Run", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === `/runs/${RUN}`) return json({ error: "boom" }, 500);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);

  expect(text()).toContain("無法讀取目前這次 Run");
  expect(text()).not.toContain("這個 Test Case 目前只有這一次 Run");
});

test("EVAL-003 候選歷史讀取失敗不會冒充空歷史", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") return json({ error: "boom" }, 500);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  await render(<RunCompare />);

  expect(text()).toContain("無法讀取可比較的 Run");
  expect(text()).not.toContain("這個 Test Case 目前只有這一次 Run");
});

const occurrences = (needle: string) => text().split(needle).length - 1;

const RUN_COST_AUTHORITY = "模型閘道對這個 Run 的 per-key 實付（ADR-017）";
const COST_NOTE = `這是下界，不是總額。權威來源：${RUN_COST_AUTHORITY}`;

test("§2.13 去重 1：兩側相同的成本但書印在列首，一次", async () => {
  stubPlatform();
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("Run 用掉的點數"));

  expect(occurrences(COST_NOTE), "同一句但書在左右兩格各印了一次").toBe(1);
  expect(occurrences("與上一列分開列，不相加。")).toBe(1);

  const heads = Array.from(container.querySelectorAll("th[scope=row]")).map(
    (th) => th.textContent ?? "",
  );
  expect(heads.some((h) => h.includes(COST_NOTE))).toBe(true);
  expect(heads.some((h) => h.includes("與上一列分開列，不相加。"))).toBe(true);
});

test("§2.13 去重 1：兩側的權威來源不同時，每一格各自留著自己的那一句", async () => {
  const mine = comparisonSide(RUN, true);
  const theirs = comparisonSide(OTHER_RUN, false);
  const differing = {
    ...COMPARISON,
    runs: [mine, { ...theirs, cost: { ...theirs.cost, authoritative_source: "另一個閘道的帳單" } }],
  };
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    if (pathOf(url).endsWith("/comparison")) return json(differing);
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("Run 用掉的點數"));

  expect(text()).toContain(`權威來源：${RUN_COST_AUTHORITY}`);
  expect(text()).toContain("權威來源：另一個閘道的帳單");
  const heads = Array.from(container.querySelectorAll("th[scope=row]")).map(
    (th) => th.textContent ?? "",
  );
  expect(heads.some((h) => h.includes("權威來源："))).toBe(false);
});

test("§2.13：回答沒有人問的問題那一句刪了；重跑的但書縮了", async () => {
  stubPlatform();
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("Run 用掉的點數"));

  expect(text()).not.toContain("比較只是讀取");
  expect(text()).not.toContain("仍須在那裡確認一次才會開始 Run");
  expect(text()).toContain("（會先經過權限確認）");
});

test("§3 第 14 條：挑另一個 Run 的說明在同一屏上只有一段", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  expect(occurrences("後開始比較")).toBe(1);
});

test("DISC-009 帶不到 2 個 id 到站是正常狀態，不是失敗", async () => {
  search = { ids: "a" };
  stubPlatform();
  await render(<Compare />);

  const notice = Array.from(container.querySelectorAll('[role="status"]')).find((el) =>
    (el.textContent ?? "").includes("請從首頁的搜尋結果或目錄選擇"),
  );
  expect(notice).toBeDefined();
  expect(
    Array.from(container.querySelectorAll('[role="alert"]')).some((el) =>
      (el.textContent ?? "").includes("請從首頁的搜尋結果或目錄選擇"),
    ),
  ).toBe(false);
});
