import AxeBuilder from "@axe-core/playwright";
import { expect } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { test } from "./socket-diagnostics";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const kind of ["cost", "journeys"] as const) {
    test(`an incomplete ${kind} trend withholds its conclusion and recovers at ${width}px`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      let failing = false;
      let reads = 0;
      const writes: string[] = [];
      await page.route("**/admin/trends/*?*", (route, request) => {
        if (request.method() !== "GET") writes.push(request.url());
        const body = platformResponse(request.url()).body as {
          buckets: Record<string, unknown>[];
          period?: Record<string, unknown>;
        };
        if (new URL(request.url()).pathname === `/admin/trends/${kind}`) {
          reads++;
          if (failing)
            return route.fulfill({
              json:
                kind === "cost"
                  ? { ...body, buckets: body.buckets.map((bucket) => ({ ...bucket, total: null })) }
                  : { ...body, period: { ...body.period, median_minutes: null } },
            });
        }
        return route.fulfill({ json: body });
      });
      await page.goto("/admin/trends?days=7");
      const overview = page.getByRole("region", { name: "本期概覽", exact: true });
      await expect(overview).toBeVisible();
      const section = page.locator(`#admin-trend-${kind}`).locator("..");
      await expect(section.locator("canvas")).toHaveCount(kind === "cost" ? 2 : 3);
      failing = true;
      await page.getByRole("button", { name: "重新整理六組趨勢", exact: true }).click();
      await expect(section.getByRole("alert")).toContainText("暫時無法讀取");
      await expect(section.locator("canvas, table")).toHaveCount(0);
      await expect(overview).toHaveCount(kind === "cost" ? 0 : 1);
      await expect(page.getByRole("main")).toContainText("已取得 5/6 組");
      expect(reads).toBe(2);
      const failedReport = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(
        test.info().outputPath("trend-unreadable-a11y.json"),
        JSON.stringify(failedReport),
      );
      expect(failedReport.violations).toEqual([]);
      expect(failedReport.incomplete).toEqual([]);
      failing = false;
      await page.getByRole("button", { name: "重新整理六組趨勢", exact: true }).click();
      await expect(section.getByRole("alert")).toHaveCount(0);
      await expect(overview).toBeVisible();
      await expect(page.getByRole("main")).toContainText("已取得 6/6 組");
      await expect(page).toHaveURL(/\/admin\/trends\?days=7$/);
      expect(reads).toBe(3);
      expect(writes).toEqual([]);
      const report = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(test.info().outputPath("trend-recovered-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("trend-recovered.png") });
    });
  }
}
