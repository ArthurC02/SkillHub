import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  RUNS,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

for (const spelling of ["lower", "upper"]) {
  for (const hasOther of [false, true]) {
    test(`${spelling} case source excludes itself ${hasOther ? "and compares another Run" : "and explains there are no other candidates"}`, async () => {
      const page = app();
      const sourceId = spelling === "upper" ? RUN.toUpperCase() : RUN;
      const source = RUNS.runs.find((run) => run.run_id === RUN)!;
      const other = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
      const comparisons: string[] = [];
      const writes = () => page.requests.filter((request) => request.method !== "GET");
      page.route("**/runs?*", () => ({ json: { runs: hasOther ? [source, other] : [source] } }));
      page.route(`**/runs/${sourceId}`, () => ({ json: platformResponse(`/runs/${RUN}`).body }));
      page.route(`**/runs/${sourceId}/comparison?*`, ({ url }) => {
        comparisons.push(url.searchParams.get("against")!);
        return { json: COMPARISON };
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
        await page.press("Enter");
        await expect(
          page.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" }),
        ).toBeVisible();
        expect(page.pathname()).toBe(`/runs/${sourceId}/compare`);
        expect(page.search()).toBe(`?against=${OTHER_RUN}`);
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
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    });
  }
}
