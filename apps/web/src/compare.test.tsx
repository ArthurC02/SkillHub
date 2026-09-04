import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { RunCompare } from "./pages/RunCompare";
import { CompareTable } from "./pages/Compare";
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

/**
 * 02:EVAL-003 — 比較頁的門把。
 *
 * 這一頁的內容一直是完整的,缺的是入口:要用它,使用者得先到別的分頁把另一個 Run
 * 的 36 字元 uuid 抄過來。候選清單補的就是那一段,而它只是換「讀哪一個既有的 Run」
 * ——全程 GET,沒有任何寫入路徑(comparison.go 檔頭:一鍵重跑會變成第二條啟動 Run
 * 的路,而它唯一能跳過的就是 TEST-009 的權限確認畫面)。
 *
 * 四件事被押在這裡:
 *
 *  1. 候選來自**同一個 Test Case**——所以伺服器的回應依 `test_case_id` 分岔,
 *     把參數拿掉會讓畫面渲染出別的 Test Case 的 Run(規則 9 的第二個突變)。
 *  2. `against` 是空的時候候選就要在,而且**辨識它的是判定與時間,不是 uuid**
 *     ——使用者不必看到 uuid 正是這件事的全部重點。
 *  3. 自己不在候選裡:自比自伺服器回 400。
 *  4. 選了之後寫進網址,比較仍然可以被連結分享。
 */

/** 一個屬於別的 Test Case 的 Run。只有在候選查詢忘了帶 `test_case_id` 時才會上場。 */
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

/**
 * 到站狀態是 `against` 空的（RunTrace 連過來時的樣子），而它以前是寫死的——所以
 * 這個檔案渲染不出比較表本身，2026-09-03 丙-142 對 `RunCompare` 的四項改動因此
 * 一支測試都沒有。改成可變的之後，`beforeEach` 會把它復位，預設行為不變。
 */
let search: { against?: string } = {};

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: RUN }),
  useSearch: () => search,
  useNavigate: () => (options: { search?: { against?: string } }) => {
    navigations.push(options);
    return Promise.resolve();
  },
  Link: ({ children }: { children?: unknown }) => children as never,
}));

/** `apiFetch` 送出的是絕對網址(client.ts 的 `API_BASE_URL`),host 要先剝掉。 */
const pathOf = (url: string) => url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

/**
 * `/runs` 依 query 分岔,其餘沿用共用 fixture(`/runs/{id}` 已帶 `test_case_id`)。
 *
 * 分岔是刻意的:fixture 的 `/runs` 剝掉 query 才比對,所以一個忘記帶 `test_case_id`
 * 的請求會拿到一模一樣的答案,測試也就押不住「候選來自同一個 Test Case」。
 */
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

/** 候選清單裡的每一個「選這一個」按鈕。 */
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

  expect(text()).toContain("來源網址：未提供");
  expect(text()).not.toContain("來源網址：未測量");
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
  expect(versionRow?.querySelector("td")?.textContent).toBe("未提供");
});

/**
 * 慣例 9 的牙齒,而且這一列曾經沒有牙齒。
 *
 * 「相容性」的 `signature` 內插的是三個 `Labelled` **物件**,於是每一個 Skill 都算出
 * 同一串 `[object Object]/[object Object]/[object Object]`,`differs` 恆為 false
 * ——一個規格驗證通過、一個未通過的兩個 Skill,那一列不上底色也不出「有差異」。
 * 整張表是拿來挑候選的,而唯一被靜音的那一列正是「這個 Skill 在我的環境跑不跑得動」。
 *
 * 這支測試押的是**那個徽章**,不是那三個字的呈現:把三個 `.value` 拿掉就變紅。
 */
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

  // 判定與時間——WorkspaceRuns / TestCases 兩份執行歷史用的同一組軸。
  expect(text()).toContain("任務判定：不符合");
  // The instant moved from the sentence into the element that carries it:
  // `<time dateTime>` is machine-readable and the rendered text is the
  // reader's own clock (components/Timestamp.tsx).
  expect(
    candidateButtons()[0].querySelector('time[datetime="2026-08-16T00:00:00Z"]'),
  ).not.toBeNull();

  // 候選來自同一個 Test Case:別的 Test Case 的 Run 不得出現。
  expect(text()).not.toContain("這是別的 Test Case 的 Run。");

  // 這一整件事的重點:清單裡看不到 uuid。
  const list = container.querySelector("ul.download-list");
  expect(list?.textContent ?? "").not.toContain(OTHER_RUN);

  // 手貼輸入框留著——另一個 Skill 的 Run 只能這樣選(設計 §2.4)。
  expect(container.querySelector("#against")).not.toBeNull();
  expect(text()).not.toContain("這個 Test Case 目前只有這一次 Run");
});

test("EVAL-003 自己不在候選裡:自比自伺服器會回 400", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  const buttons = candidateButtons();
  expect(buttons.length, "候選清單把這一邊自己也列了進去").toBe(1);
  // 這一邊自己是 2026-08-17 那一次;它的日期不得出現在任何一個候選按鈕上。
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
    // 只有這一次 Run——也就是比較頁自己那一側。
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

/**
 * 2026-09-03（丙-142，設計 §2.13）：`RunCompare` 的四項去重以前一支測試都沒有——
 * 這個檔案的 `useSearch` 是寫死的空物件，所以比較表本身從來沒有被渲染過。
 *
 * 押的是**次數**不是「有沒有這句話」：一句印在左右兩格的話對 `toContain` 永遠是綠的，
 * 把它搬回格子裡也是綠的。
 */
const occurrences = (needle: string) => text().split(needle).length - 1;

const RUN_COST_AUTHORITY = "模型閘道對這個 Run 的 per-key 實付（ADR-017）";
const COST_NOTE = `這是下界，不是總額。權威來源：${RUN_COST_AUTHORITY}`;

test("§2.13 去重 1：兩側相同的成本但書印在列首，一次", async () => {
  stubPlatform();
  search = { against: OTHER_RUN };
  await render(<RunCompare />);
  await waitFor(() => text().includes("Run 成本"));

  expect(occurrences(COST_NOTE), "同一句但書在左右兩格各印了一次").toBe(1);
  expect(occurrences("與上一列分開列，不相加。")).toBe(1);

  // 而且它在列首，不是在某一格裡——這一條分辨「搬對了」與「剛好只有一格有值」。
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
  await waitFor(() => text().includes("Run 成本"));

  // 這一支才是「一律搬到列首」那個錯誤實作的照妖鏡：兩句不一樣時它們必須留在格子裡。
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
  await waitFor(() => text().includes("Run 成本"));

  // 這一頁沒有任何寫入控制項，所以「不會改動歷史資料」回答的是沒有人問的問題。
  expect(text()).not.toContain("比較只是讀取");
  // 前半（仍在／不在）是 §2.4 的原因，留著；後半的敘述縮成一個括號。
  expect(text()).not.toContain("仍須在那裡確認一次才會開始 Run");
  expect(text()).toContain("（會先經過權限確認）");
});

test("§3 第 14 條：挑另一個 Run 的說明在同一屏上只有一段", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  expect(occurrences("後開始比較")).toBe(1);
});
