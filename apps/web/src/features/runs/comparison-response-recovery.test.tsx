import { expect, test } from "vitest";
import { COMPARISON, OTHER_RUN, RUN } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

for (const fault of ["foreign-run", "reversed-runs", "reversed-matrix"] as const) {
  for (const cached of [false, true]) {
    test(`comparison rejects ${fault} ${cached ? "during refresh" : "on first read"} and recovers`, async () => {
      const page = app();
      const reads: string[] = [];
      const writes = () => page.requests.filter((request) => request.method !== "GET");
      let corrupt = !cached;
      page.route(`**/runs/${RUN}/comparison?*`, ({ url }) => {
        reads.push(url.searchParams.get("against") ?? "");
        const body = structuredClone(COMPARISON);
        if (corrupt) {
          if (fault === "foreign-run") body.runs[0].run_id = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
          else if (fault === "reversed-runs") body.runs.reverse();
          else body.criterion_matrix[0].results.reverse();
        }
        return { json: body };
      });
      await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
      const main = page.locator("main");
      const table = main.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" });
      if (cached) {
        await expect(table).toBeVisible();
        corrupt = true;
        await page.getByRole("button", { name: /^更新比較/ }).click();
      }
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取比較結果");
      await expect(table).toHaveCount(0);
      await expect(main.locator(".verdict")).toHaveCount(0);
      expect(await page.axeViolations("main")).toEqual([]);
      corrupt = false;
      await main.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await expect(table).toBeVisible();
      await expect(main.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#run-comparison-heading")).toBeFocused();
      const matrix = main.locator("table").filter({ hasText: "驗收條件判定矩陣對比" });
      await expect(matrix.locator("td")).toHaveText("未通過模型評估 未評估");
      expect(reads).toEqual(cached ? [OTHER_RUN, OTHER_RUN, OTHER_RUN] : [OTHER_RUN, OTHER_RUN]);
      expect(writes()).toEqual([]);
      expect(page.search()).toBe(`?against=${OTHER_RUN}`);
    });
  }
}
