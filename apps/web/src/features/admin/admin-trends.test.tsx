import { act } from "react";
import { expect, test } from "vitest";
import { focusManager } from "@tanstack/react-query";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { daysOf, seriesOf, usd } from "./admin.service";
import {
  ADMIN_COST_STATISTICS,
  ADMIN_TREND_JOURNEYS,
  platformResponse,
} from "../../testing/fixtures/platform";
import {
  container,
  calls,
  stub,
  mountAt,
  go,
  waitFor,
  has,
  field,
  button,
  click,
} from "./testing/harness";

test("OPS-007: cost statistics show dollars and name a window with no samples", async () => {
  stub(true);
  await mountAt("/admin/cost-statistics");
  await waitFor(has("搜尋理由"));
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(-1);
  const table = field<HTMLTableElement>("table.responsive-table");
  const labels = ["種類", "統計期間", "樣本數", "p50", "p90", "p95", "最大"];
  expect(Array.from(table.querySelectorAll("thead th")).map((th) => th.textContent)).toEqual(
    labels,
  );
  expect(
    Array.from(table.tBodies[0].rows).map((row) =>
      Array.from(row.children).map((cell) => cell.getAttribute("data-label")),
    ),
  ).toEqual([labels, labels]);
  expect(
    Array.from(table.querySelectorAll('tbody th[scope="row"]')).map((th) => th.textContent),
  ).toEqual(["搜尋理由", "評審"]);
  expect(
    Array.from(table.querySelectorAll('tbody td[data-label="統計期間"]')).map((cell) =>
      Array.from(cell.querySelectorAll("time")).map((time) => time.dateTime),
    ),
  ).toEqual([
    ["2026-08-12T00:00:00Z", "2026-09-11T00:00:00Z"],
    ["2026-08-12T00:00:00Z", "2026-09-11T00:00:00Z"],
  ]);
  const rows = Array.from(container.querySelectorAll("tbody tr")).map((tr) => [
    tr.querySelector("th")?.textContent,
    ...Array.from(tr.querySelectorAll("td"))
      .slice(1)
      .map((td) => td.textContent),
  ]);
  expect(rows).toEqual([
    ["搜尋理由", "0", "未測量", "未測量", "未測量", "未測量"],
    ["評審", "40", "$0.0012", "$0.0034", "$0.0041", "$0.0090"],
  ]);
});

test("OPS-007: unreadable cost statistics hide cached figures and recover on retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/cost-statistics") return undefined;
    reads += 1;
    return reads === 1 || reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_COST_STATISTICS, status: 200 };
  });
  await mountAt("/admin/cost-statistics");
  await waitFor(has("暫時無法讀取成本統計"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(container.querySelector("table")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("搜尋理由"));
  expect(has("成本統計清單上次取得於")()).toBe(true);
  await click(button("重新整理"));
  await waitFor(has("先前載入的數字已隱藏"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("$0.0090")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("搜尋理由"));
  expect(reads).toBe(4);
});

test("OPS-007: nonzero micro-dollar costs remain distinct from zero and missing values", () => {
  expect(usd(null)).toBe("未測量");
  expect(usd(0)).toBe("$0.0000");
  expect(usd(1)).toBe("$0.000001");
  expect(usd(49)).toBe("$0.000049");
  expect(usd(50)).toBe("$0.00005");
  expect(usd(100)).toBe("$0.0001");
  expect(usd(1_234_567)).toBe("$1.234567");
});

test("OPS-008: the day list runs from the first to the last day inclusive, across a month end", () => {
  expect(daysOf("2026-08-30", "2026-09-02")).toEqual([
    "2026-08-30",
    "2026-08-31",
    "2026-09-01",
    "2026-09-02",
  ]);
  expect(daysOf("2026-09-12", "2026-09-12")).toEqual(["2026-09-12"]);
  expect(daysOf("2026-09-13", "2026-09-12")).toEqual([]);
});

test("OPS-008: a series fills a day with no bucket with zero and drops a bucket outside the range", () => {
  const { days, series } = seriesOf(
    {
      from: "2026-09-10",
      to: "2026-09-12",
      buckets: [
        { day: "2026-09-12", key: "review", count: 2, total: 30 },
        { day: "2026-09-10", key: "review", count: 1, total: 5 },
        { day: "2026-09-09", key: "review", count: 9, total: 900 },
        { day: "2026-09-11", key: "generate", count: 1, total: 7 },
      ],
    },
    (bucket) => bucket.total,
  );
  expect(days).toEqual(["2026-09-10", "2026-09-11", "2026-09-12"]);
  expect(series).toEqual([
    { key: "generate", counts: [0, 1, 0], values: [0, 7, 0] },
    { key: "review", counts: [1, 0, 2], values: [5, 0, 30] },
  ]);
});

const TREND_PATHS = ["cost", "credits", "funnel", "journeys", "operator-actions", "runs"];
const trendCalls = () => calls.filter((c) => c.url.startsWith("/admin/trends/")).map((c) => c.url);
const trendsFor = (days: number) =>
  new Set(TREND_PATHS.map((path) => `/admin/trends/${path}?days=${days}`));

test("OPS-008: the trends page asks each owner for 30 days by default and draws one chart per kind with events", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(new Set(trendCalls())).toEqual(trendsFor(30));
  expect(
    Array.from(container.querySelectorAll<HTMLElement>("figure .table-scroll")).map(
      (scroll) => scroll.tabIndex,
    ),
  ).toEqual(Array(14).fill(0));
  expect(has("2026-08-14 到 2026-09-12（UTC），共 30 天。")()).toBe(true);
  expect(
    Array.from(container.querySelectorAll('canvas[role="img"]')).map((c) =>
      c.getAttribute("aria-label"),
    ),
  ).toEqual(
    [
      "單次生成",
      "評審",
      "扣點",
      "授予",
      "執行失敗",
      "執行完成",
      "查詢帳號",
      "授予點數",
      "開始試跑",
      "搜尋",
      "看小工具詳情",
      "24 小時內按下下載",
      "搜尋過的工作階段",
      "10 分鐘內按下下載",
    ].map((name) => `${name}：每日長條圖，逐日數字在下方的表`),
  );
});

test("aligned trends put four period totals before the daily charts", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  const overview = field<HTMLElement>("#admin-trend-overview");
  expect(
    Array.from(overview.querySelectorAll("dl > div")).map((row) => [
      row.querySelector("dt")?.textContent,
      row.querySelector("dd")?.textContent,
    ]),
  ).toEqual([
    ["模型成本（含估計）", "$0.0936"],
    ["點數淨異動", "+118 點"],
    ["建立 Run", "6 筆"],
    ["後台動作", "2 筆"],
  ]);
  expect(
    overview.compareDocumentPosition(field<HTMLElement>("#admin-trend-cost")) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(trendCalls()).toHaveLength(6);
});

test("OPS-008: the range comes before compact read metadata on a complete trends page", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  const text = container.textContent ?? "";
  expect(text).toContain("2026-08-14 到 2026-09-12（UTC），共 30 天。");
  expect(text).toContain("已取得 6/6 組 ·");
  expect(text.indexOf("2026-08-14 到 2026-09-12（UTC），共 30 天。")).toBeLessThan(
    text.indexOf("已取得 6/6 組 ·"),
  );
  expect(text).not.toContain("秒前");
});

test("OPS-008: a failed trend hides its cached chart while other trends remain usable and refresh recovers it", async () => {
  let creditReads = 0;
  stub(true, (path) => {
    if (path !== "/admin/trends/credits") return undefined;
    creditReads += 1;
    return creditReads === 2 ? { body: { error: "service unavailable" }, status: 503 } : undefined;
  });
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));

  const credits = Array.from(container.querySelectorAll("section")).find(
    (section) => section.querySelector("h2")?.textContent === "每日點數異動（淨額）",
  )!;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.trend("credits", 30) });
  });
  await waitFor(() => credits.querySelector('[role="alert"]') !== null);
  expect(credits.textContent).toContain("先前載入的趨勢已隱藏");
  expect(credits.textContent).not.toContain("全平台目前餘額總和：1268 點。");
  expect(credits.querySelector("figure")).toBeNull();
  expect(has("評審：3 筆，合計 $0.0036")()).toBe(true);
  expect(has("已取得 5/6 組 ·")()).toBe(true);
  expect(container.querySelector("#admin-trend-overview")).toBeNull();

  await click(button("重新整理"));
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(creditReads).toBe(3);
  expect(has("已取得 6/6 組 ·")()).toBe(true);
});

test("OPS-008: losing all trend reads hides the cached range until refresh succeeds", async () => {
  let unavailable = false;
  stub(true, (path) =>
    unavailable && path.startsWith("/admin/trends/")
      ? { body: { error: "service unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));

  unavailable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: ["admin", "trends"] });
  });
  await waitFor(has("目前沒有可用趨勢。"));
  expect(has("2026-08-14 到 2026-09-12（UTC），共 30 天。")()).toBe(false);
  expect(container.querySelectorAll("figure")).toHaveLength(0);

  unavailable = false;
  await click(button("重新整理"));
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(has("2026-08-14 到 2026-09-12（UTC），共 30 天。")()).toBe(true);
});

test("OPS-008: different response windows are not presented as one comparable range", async () => {
  stub(true, (path, _method, url) => {
    if (path !== "/admin/trends/runs") return undefined;
    const response = platformResponse(url);
    return { ...response, body: { ...(response.body as object), from: "2026-08-15" } };
  });
  await mountAt("/admin/trends");
  await waitFor(has("已取得 6/6 組 ·"));
  expect(has("取得的趨勢日期範圍與選擇不一致，請重新整理後再比較。")()).toBe(true);
  expect(has("2026-08-14 到 2026-09-12（UTC），共 30 天。")()).toBe(false);
  expect(container.querySelector("#admin-trend-overview")).toBeNull();
});

test("OPS-008: a seven-day response does not masquerade as the selected 30 days", async () => {
  stub(true, (path, _method, url) => {
    if (!path.startsWith("/admin/trends/")) return undefined;
    const response = platformResponse(url);
    return { ...response, body: { ...(response.body as object), from: "2026-09-06" } };
  });
  await mountAt("/admin/trends");
  await waitFor(has("已取得 6/6 組 ·"));
  expect(has("取得的趨勢日期範圍與選擇不一致，請重新整理後再比較。")()).toBe(true);
  expect(has("2026-09-06 到 2026-09-12（UTC），共 7 天。")()).toBe(false);
});

test("OPS-008: a kind's figure totals its range and its table shows zero on the days it had nothing", async () => {
  stub(true);
  await mountAt("/admin/trends", { days: "7" });
  await waitFor(has("評審：3 筆，合計 $0.0036"));
  expect(has("扣點：4 筆，合計 -32 點")()).toBe(true);
  const review = Array.from(container.querySelectorAll("figure")).find((figure) =>
    figure.textContent?.startsWith("評審"),
  );
  expect(Array.from(review!.querySelectorAll("tbody tr")).map((tr) => tr.textContent)).toEqual([
    "2026-09-062$0.0024",
    "2026-09-070$0.0000",
    "2026-09-081$0.0012",
    "2026-09-090$0.0000",
    "2026-09-100$0.0000",
    "2026-09-110$0.0000",
    "2026-09-120$0.0000",
  ]);
});

test("OPS-008: the funnel says what one count means at every stage, from the server's own sentences", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("每個瀏覽工作階段一天算一次，這一段系統性偏高。"));
  const funnelSection = Array.from(container.querySelectorAll("section")).find(
    (section) => section.querySelector("h2")?.textContent === "漏斗各段每天到達的數量",
  )!;
  const grains = Array.from(funnelSection.querySelectorAll("dl > div")).map((row) => [
    row.querySelector("dt")?.textContent,
    row.querySelector("dd")?.textContent,
  ]);
  expect(grains).toEqual([
    ["搜尋", "每個瀏覽工作階段一天算一次，這一段系統性偏高。"],
    ["看小工具詳情", "粒度同搜尋。"],
    ["開始試跑", "每個工作區一天算一次，不能相除成轉換率。"],
    ["按下下載", "每個工作區一天算一次；打包仍可能被拒。"],
  ]);
  expect(has("這段期間沒有事件：按下下載。")()).toBe(true);
  expect(has("搜尋：12 筆")()).toBe(true);
  expect(
    funnelSection
      .querySelector("dl")!
      .compareDocumentPosition(funnelSection.querySelector("figure")!) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

const journeySection = () =>
  Array.from(container.querySelectorAll("section")).find(
    (section) => section.querySelector("h2")?.textContent === "從搜尋到下載",
  );

test.each([
  {
    name: "a period with downloads states both shares and the median minutes",
    period: { searched: 20, downloaded: 5, within_promise: 2, median_minutes: 14.5 },
    sentence:
      "搜尋過的 20 個工作階段中，5 個（25%）在 24 小時內按下下載，2 個（10%）在 10 分鐘內。從第一次搜尋到按下下載的中位數是 14.5 分鐘。",
  },
  {
    name: "a period without a download states the shares and no median",
    period: { searched: 3, downloaded: 0, within_promise: 0 },
    sentence: "搜尋過的 3 個工作階段中，0 個（0%）在 24 小時內按下下載，0 個（0%）在 10 分鐘內。",
  },
  {
    name: "a period without a search says so instead of dividing by zero",
    period: { searched: 0, downloaded: 0, within_promise: 0 },
    sentence: "這段期間沒有搜尋過的工作階段。",
  },
])("OPS-008: the search-to-download journey — $name", async ({ period, sentence }) => {
  stub(true, (path) =>
    path === "/admin/trends/journeys"
      ? { body: { ...ADMIN_TREND_JOURNEYS, period }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/trends");
  await waitFor(() => journeySection()?.querySelector("p") != null);
  expect(journeySection()!.querySelector("p")!.textContent).toBe(sentence);
});

test("OPS-008: the journey chart names its three lines from the promise the server states", async () => {
  stub(true, (path) =>
    path === "/admin/trends/journeys"
      ? {
          body: { ...ADMIN_TREND_JOURNEYS, promise_minutes: 15, journey_window_hours: 48 },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/trends");
  await waitFor(has("搜尋過的工作階段：20 筆"));
  expect(has("48 小時內按下下載：5 筆")()).toBe(true);
  expect(has("15 分鐘內按下下載：2 筆")()).toBe(true);
});

test("OPS-008: a kind with no events in the range is named instead of drawn", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和"));
  expect(has("這段期間沒有事件：儲值、更正。")()).toBe(true);
  expect(
    has(
      "這段期間沒有事件：創作步驟、創作會話、搜尋向量、搜尋意圖分析、平台 Agent、索引增強、評審前的注入檢查、改善建議、試跑、搜尋理由。",
    )(),
  ).toBe(true);
  const absent = Array.from(container.querySelectorAll("details")).find((details) =>
    details.textContent?.includes("這段期間沒有事件：創作步驟"),
  )!;
  expect(absent.open).toBe(false);
  expect(absent.querySelector("summary")?.textContent).toBe("這段期間有 10 類沒有事件");
});

test("OPS-008: a range in the address is asked for, and a range the page does not offer falls back to 30", async () => {
  stub(true);
  await mountAt("/admin/trends", { days: "7" });
  await waitFor(has("全平台目前餘額總和"));
  expect(new Set(trendCalls())).toEqual(trendsFor(7));
  expect(has("2026-09-06 到 2026-09-12（UTC），共 7 天。")()).toBe(true);

  calls.length = 0;
  await go("/admin/trends", { days: "90" });
  await waitFor(has("2026-06-15 到 2026-09-12（UTC），共 90 天。"));
  expect(new Set(trendCalls())).toEqual(trendsFor(90));

  calls.length = 0;
  await go("/admin/trends", { days: "8" });
  await waitFor(() => trendCalls().length >= 6);
  expect(new Set(trendCalls())).toEqual(trendsFor(30));
});

test("OPS-008: exactly the queried range is marked as selected after navigation and fallback", async () => {
  stub(true);
  await mountAt("/admin/trends");
  const selectedRange = () =>
    Array.from(container.querySelectorAll('nav[aria-label="時間範圍"] a[aria-current="page"]')).map(
      (link) => link.textContent,
    );
  await waitFor(() => selectedRange().join() === "30 天");
  expect(selectedRange()).toEqual(["30 天"]);

  await go("/admin/trends", { days: "7" });
  await waitFor(() => selectedRange().join() === "7 天");
  expect(selectedRange()).toEqual(["7 天"]);

  await go("/admin/trends", { days: "8" });
  await waitFor(() => selectedRange().join() === "30 天");
  expect(selectedRange()).toEqual(["30 天"]);
});

test("OPS-008: the trends are asked for once, not again when the window regains focus", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和"));
  const asked = trendCalls().length;
  await act(async () => {
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  focusManager.setFocused(undefined);
  expect(trendCalls().length).toBe(asked);
});

test("OPS-008: useTrend itself refuses a focus refetch even when the app default would allow it", async () => {
  const defaults = queryClient.getDefaultOptions();
  queryClient.setDefaultOptions({
    queries: { ...defaults.queries, refetchOnWindowFocus: true, staleTime: 0 },
  });
  try {
    stub(true);
    await mountAt("/admin/trends");
    await waitFor(has("全平台目前餘額總和"));
    const asked = trendCalls().length;
    expect(asked).toBeGreaterThan(0);
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(trendCalls().length).toBe(asked);
  } finally {
    focusManager.setFocused(undefined);
    queryClient.setDefaultOptions(defaults);
  }
});

test("OPS-008: a member who types the trends address gets the missing page and no trend request", async () => {
  stub(false);
  await mountAt("/admin/trends");
  await waitFor(has("這一頁現在不存在"));
  expect(trendCalls()).toEqual([]);
});
