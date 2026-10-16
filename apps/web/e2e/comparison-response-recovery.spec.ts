import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { COMPARISON, OTHER_RUN, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const fault of ["foreign-run", "reversed-runs", "reversed-matrix"] as const) {
  for (const cached of [false, true]) {
    test(`comparison rejects ${fault} ${cached ? "during refresh" : "on first read"} and recovers`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: cached ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: cached ? "dark" : "light" });
      const reads: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      let corrupt = !cached;
      await page.route(`**/runs/${RUN}/comparison?*`, (route) => {
        reads.push(new URL(route.request().url()).searchParams.get("against") ?? "");
        const body = structuredClone(COMPARISON);
        if (corrupt) {
          if (fault === "foreign-run") body.runs[0].run_id = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
          else if (fault === "reversed-runs") body.runs.reverse();
          else body.criterion_matrix[0].results.reverse();
        }
        return route.fulfill({ json: body });
      });
      await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
      const main = page.getByRole("main");
      const table = main.getByRole("table", {
        name: "試跑紀錄任務判定與執行狀態對比",
        exact: true,
      });
      if (cached) {
        await expect(table).toBeVisible();
        corrupt = true;
        await page.getByRole("button", { name: /^更新比較/ }).click();
      }
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取比較結果");
      await expect(table).toHaveCount(0);
      await expect(main.locator(".verdict")).toHaveCount(0);
      const accessibility = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(
        testInfo.outputPath("unreadable-comparison-axe.json"),
        JSON.stringify(accessibility),
      );
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
      expect(accessibility.passes.length).toBeGreaterThan(10);
      await page.screenshot({
        path: testInfo.outputPath("unreadable-comparison.png"),
        fullPage: true,
      });
      corrupt = false;
      await main.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(table).toBeVisible();
      await expect(main.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#run-comparison-heading")).toBeFocused();
      const matrix = main.getByRole("table", { name: "驗收條件判定矩陣對比", exact: true });
      await expect(matrix.getByRole("cell")).toHaveText(["未通過模型評估", "未評估"]);
      expect(reads).toEqual(cached ? [OTHER_RUN, OTHER_RUN, OTHER_RUN] : [OTHER_RUN, OTHER_RUN]);
      expect(writes).toEqual([]);
      expect(new URL(page.url()).searchParams.get("against")).toBe(OTHER_RUN);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    });
  }
}
