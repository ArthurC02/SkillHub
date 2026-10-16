import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_COST_STATISTICS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const WINDOW = ADMIN_COST_STATISTICS.statistics[0];

async function inspectCostStatistics(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(label + "-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
  await page.screenshot({ path: test.info().outputPath(label + "-viewport.png") });
}

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
    test(`cost statistics with ${name} on ${reread ? "reread" : "arrival"} recover locally without inventing measurements`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 812 });
      await page.emulateMedia({ colorScheme: reread ? "dark" : "light" });
      await stubPlatform(page);
      let reads = 0;
      let documents = 0;
      let writes = 0;
      page.on("domcontentloaded", () => documents++);
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/admin/cost-statistics", async (route) => {
        if (route.request().resourceType() === "document") return route.continue();
        reads++;
        if (route.request().method() !== "GET") writes++;
        const failedRead = reread ? 2 : 1;
        if (reads === failedRead) return route.fulfill({ json: invalid });
        if (reads > failedRead) await gate;
        return route.fulfill({ json: ADMIN_COST_STATISTICS });
      });
      try {
        await page.goto("/admin/cost-statistics");
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const main = page.getByRole("main");
        const table = main.getByRole("table", { name: "每一種呼叫最新的統計窗（美元）" });
        if (reread) {
          await expect(table).toBeVisible();
          await main.getByRole("button", { name: "重新整理", exact: true }).click();
        }
        const alert = main.getByRole("alert");
        await expect(alert).toContainText("暫時無法讀取成本統計");
        if (reread) await expect(alert).toContainText("先前載入的數字已隱藏");
        const heading = main.getByRole("heading", { level: 1, name: "成本統計", exact: true });
        await expect(heading).toBeVisible();
        await expect(table).toHaveCount(0);
        await expect(main).not.toContainText("統計窗：0 個");
        await expect(main).not.toContainText("$NaN");
        await expect(main.getByText("成本統計清單上次取得於", { exact: false })).toHaveCount(0);
        await expect(main.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        expect(reads).toBe(reread ? 2 : 1);
        await inspectCostStatistics(page, "cost-read-error");
        await main.getByRole("button", { name: "再試一次", exact: true }).focus();
        await page.keyboard.press("Enter");
        await expect.poll(() => reads).toBe(reread ? 3 : 2);
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
        expect(documents).toBe(1);
        expect(writes).toBe(0);
        expect(reads).toBe(reread ? 3 : 2);
        await inspectCostStatistics(page, "cost-read-recovered");
      } finally {
        await writeFile(
          test.info().outputPath("cost-read-state.json"),
          JSON.stringify({
            name,
            reread,
            reads,
            documents,
            writes,
            text: await page.locator("body").innerText(),
          }),
        );
        release();
      }
    });
  }
}

for (const kind of ["empty", "zero and unmeasured", "unknown kind"] as const) {
  test(`readable ${kind} cost statistics retain their actual meaning`, async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await stubPlatform(page);
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
    await page.route("**/admin/cost-statistics", (route) => {
      if (route.request().resourceType() === "document") return route.continue();
      reads++;
      return route.fulfill({ json: { statistics } });
    });
    await page.goto("/admin/cost-statistics");
    const main = page.getByRole("main");
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
    await inspectCostStatistics(page, "cost-readable");
  });
}
