import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  RUNS,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const spelling of ["lower", "upper"]) {
  for (const hasOther of [false, true]) {
    test(`${spelling} case source excludes itself ${hasOther ? "and compares another Run" : "and explains there are no other candidates"}`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: hasOther ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: hasOther ? "dark" : "light" });
      const sourceId = spelling === "upper" ? RUN.toUpperCase() : RUN;
      const source = RUNS.runs.find((run) => run.run_id === RUN)!;
      const other = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
      await page.route("**/runs?*", (route) =>
        route.fulfill({ json: { runs: hasOther ? [source, other] : [source] } }),
      );
      await page.route(`**/runs/${sourceId}`, (route) =>
        route.request().resourceType() === "document"
          ? route.fallback()
          : route.fulfill({ json: platformResponse(`/runs/${RUN}`).body }),
      );
      const comparisons: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route(`**/runs/${sourceId}/comparison?*`, (route) => {
        comparisons.push(new URL(route.request().url()).searchParams.get("against")!);
        return route.fulfill({ json: COMPARISON });
      });
      await page.goto(`/runs/${sourceId}/compare`);
      await expect(
        page.getByRole("navigation", { name: "這個小工具的工作台", exact: true }),
      ).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      const own = page.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${RUN}$`) });
      const candidate = page.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${OTHER_RUN}$`) });
      await expect(own).toHaveCount(0);
      await expect(page.getByRole("alert")).toHaveCount(0);
      expect(comparisons).toEqual([]);
      if (hasOther) {
        await expect(candidate).toBeVisible();
        await candidate.focus();
        await page.keyboard.press("Enter");
        await expect(
          page.getByRole("table", { name: "試跑紀錄任務判定與執行狀態對比", exact: true }),
        ).toBeVisible();
        await expect(page).toHaveURL(`/runs/${sourceId}/compare?against=${OTHER_RUN}`);
        await expect(page.locator("#run-comparison-heading")).toBeFocused();
        await page.getByText("換一個要比較的試跑紀錄", { exact: true }).click();
        await expect(own).toHaveCount(0);
        await expect(candidate).toBeVisible();
        expect(comparisons).toEqual([OTHER_RUN]);
      } else {
        await expect(candidate).toHaveCount(0);
        await expect(
          page.getByText("這個測試題目前只有這一次試跑紀錄，沒有同一個測試題的其他試跑紀錄可選。", {
            exact: true,
          }),
        ).toBeVisible();
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        await page.getByLabel("要比較的另一個試跑紀錄 ID").fill("尚未送出的比較 ID");
        expect(comparisons).toEqual([]);
      }
      expect(writes).toEqual([]);
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(info.outputPath("source-candidates-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await page.screenshot({ path: info.outputPath("source-candidates.png") });
    });
  }
}
