import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { RunCompare } from "./pages/RunCompare";
import { OTHER_RUN, RUN, RUNS, TEST_CASE, platformResponse } from "./fixtures/platform";

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

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: RUN }),
  // 到站狀態:`against` 是空的,也就是 RunTrace 連過來時的樣子。
  useSearch: () => ({}),
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
function stubPlatform() {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (pathOf(url) === "/runs") {
      return json(url.includes(`test_case_id=${TEST_CASE}`) ? RUNS : { runs: [FOREIGN_RUN] });
    }
    const { body, status } = platformResponse(url);
    return json(body, status);
  });
}

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

test("EVAL-003 到站時就有同一個 Test Case 的候選,而且認得出它的不是 uuid", async () => {
  stubPlatform();
  await render(<RunCompare />);
  await waitFor(() => candidateButtons().length > 0);

  // 判定與時間——WorkspaceRuns / TestCases 兩份執行歷史用的同一組軸。
  expect(text()).toContain("任務判定：不符合");
  expect(candidateButtons()[0].textContent).toContain("2026-08-16T00:00:00Z");

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
