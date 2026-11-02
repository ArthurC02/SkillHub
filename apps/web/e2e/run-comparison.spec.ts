import { expect, test } from "@playwright/test";
import { COMPARISON, OTHER_RUN, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [1280, 375]) {
  test(`long run outputs stay within their own comparison cells at ${width}px`, async ({
    page,
  }) => {
    await stubPlatform(page);
    const outputs = [
      "請提供要輸出的單字，以及要套用的技能名稱；我會依該技能規則直接輸出。".repeat(3),
      `first line\n${"unbroken-output-".repeat(40)}\nlast line`,
    ];
    await page.route(`**/runs/${RUN}/comparison?*`, (route) =>
      route.fulfill({
        json: {
          ...COMPARISON,
          runs: COMPARISON.runs.map((side, i) => ({ ...side, final_output: outputs[i] })),
        },
      }),
    );
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
    const output = page
      .getByRole("table", { name: "試跑紀錄任務判定與執行狀態對比" })
      .getByRole("row")
      .filter({ hasText: "最終輸出" })
      .locator("pre");
    await expect(output).toHaveText(outputs);
    for (const pre of await output.all()) {
      const bounds = await pre.evaluate((el) => {
        const range = document.createRange();
        range.selectNodeContents(el);
        const cell = el.closest("td")!.getBoundingClientRect();
        return [...range.getClientRects()].every(
          (rect) => rect.left >= cell.left && rect.right <= cell.right,
        );
      });
      expect(bounds, "output text painted outside its comparison cell").toBe(true);
    }
  });
}
