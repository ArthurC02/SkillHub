import { expect, test } from "vitest";
import { ADMIN_COST_STATISTICS } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const WINDOW = ADMIN_COST_STATISTICS.statistics[0];

for (const [name, invalid] of [
  ["null envelope", null],
  ["missing list", {}],
  ["null list", { statistics: null }],
  ["object list", { statistics: {} }],
  ["null row", { statistics: [null] }],
  ["missing amount", { statistics: [{ ...WINDOW, p50_usd_micros: undefined }] }],
  ["string amount", { statistics: [{ ...WINDOW, p50_usd_micros: "1" }] }],
  ["unreadable period", { statistics: [{ ...WINDOW, window_start: "not-a-time" }] }],
] as const) {
  for (const reread of [false, true]) {
    test(`cost statistics with ${name} on ${reread ? "reread" : "arrival"} recover locally without inventing measurements`, async () => {
      const page = app();
      let reads = 0;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/admin/cost-statistics", async () => {
        reads++;
        const failedRead = reread ? 2 : 1;
        if (reads === failedRead) return { json: invalid };
        if (reads > failedRead) await gate;
        return { json: ADMIN_COST_STATISTICS };
      });
      try {
        await page.goto("/admin/cost-statistics");
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const main = page.locator("main");
        const table = main.locator("table").filter({ hasText: "每一種呼叫最新的統計窗（美元）" });
        if (reread) {
          await expect(table).toBeVisible();
          await main.getByRole("button", { name: "重新整理", exact: true }).click();
        }
        const alert = main.getByRole("alert");
        await expect(alert).toContainText("暫時無法讀取成本統計");
        if (reread) await expect(alert).toContainText("先前載入的數字已隱藏");
        const heading = main.locator("h1");
        await expect(heading).toHaveText("成本統計");
        await expect(table).toHaveCount(0);
        await expect(main).not.toContainText("統計窗：0 個");
        await expect(main).not.toContainText("$NaN");
        await expect(main.getByText("成本統計清單上次取得於")).toHaveCount(0);
        await expect(main.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        expect(reads).toBe(reread ? 2 : 1);
        expect(await page.axeViolations("body")).toEqual([]);
        await main.getByRole("button", { name: "再試一次", exact: true }).focus();
        await page.press("Enter");
        await page.eventually(() => expect(reads).toBe(reread ? 3 : 2));
        if (reread)
          await expect(
            main.getByRole("button", { name: "重新讀取中…", exact: true }),
          ).toBeDisabled();
        else await expect(main.getByRole("status")).toContainText("載入成本統計中");
        await expect(heading).toBeFocused();
        await expect(table).toHaveCount(0);
        release();
        await expect(table).toBeVisible();
        await expect(alert).toHaveCount(0);
        await expect(table.locator("tbody tr")).toHaveCount(2);
        await expect(table.locator("time").first()).toHaveAttribute(
          "datetime",
          "2026-08-12T00:00:00Z",
        );
        await expect(table.locator("tbody tr").first().locator('[data-label="p50"]')).toHaveText(
          "未測量",
        );
        await expect(heading).toBeFocused();
        expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
        expect(reads).toBe(reread ? 3 : 2);
        expect(await page.axeViolations("body")).toEqual([]);
      } finally {
        release();
      }
    });
  }
}

for (const kind of ["empty", "zero and unmeasured", "unknown kind"] as const) {
  test(`readable ${kind} cost statistics retain their actual meaning`, async () => {
    const page = app();
    const statistics =
      kind === "empty"
        ? []
        : kind === "unknown kind"
          ? [{ ...WINDOW, kind: "future_kind" }]
          : [
              WINDOW,
              {
                ...ADMIN_COST_STATISTICS.statistics[1],
                sample_count: 1,
                p50_usd_micros: 0,
                p90_usd_micros: 0,
                p95_usd_micros: 0,
                max_usd_micros: 0,
              },
            ];
    let reads = 0;
    page.route("**/admin/cost-statistics", () => {
      reads++;
      return { json: { statistics } };
    });
    await page.goto("/admin/cost-statistics");
    const main = page.locator("main");
    if (kind === "empty") {
      await expect(
        main.getByText("統計窗：0 個。每日統計跑過之後才會有。", { exact: true }),
      ).toBeVisible();
      await expect(main.getByRole("table")).toHaveCount(0);
    } else {
      const rows = main.locator("tbody tr");
      await expect(rows).toHaveCount(kind === "unknown kind" ? 1 : 2);
      if (kind === "unknown kind")
        await expect(rows.first().getByRole("rowheader")).toHaveText("future_kind");
      else {
        await expect(rows.first().locator('[data-label="p50"]')).toHaveText("未測量");
        await expect(rows.nth(1).locator('[data-label="p50"]')).toHaveText("$0.0000");
      }
    }
    await expect(main.getByRole("alert")).toHaveCount(0);
    expect(reads).toBe(1);
    expect(await page.axeViolations("body")).toEqual([]);
  });
}
