import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { RUN, SKILL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [name, rejected] of [
  ["object list", { length: 1 }],
  ["null entry", [null]],
  ["unknown reason", [{ suggestion_id: "s1", blocked_reason: "future", message: "原因未知。" }]],
  ["empty reasons", []],
] as const) {
  test(`a refused apply with ${name} retains its confirmed refusal and recovers reasons without another write`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let writes = 0;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) => {
      writes += 1;
      return route.fulfill({
        status: 422,
        json: {
          error: "not one of the suggestions could be applied, so no version was created",
          rejected_suggestions: rejected,
        },
      });
    });
    await page.route("**/suggestions/s1/diff", async (route) => {
      reads += 1;
      if (reads === 1)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      await gate;
      return route.fulfill({ json: { applicable: false, blocked_reason: "target_changed" } });
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const region = page.getByRole("region", { name: "改善建議", exact: true });
      await region
        .getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true })
        .click();
      const failure = region.getByRole("alert", { name: "建立版本結果", exact: true });
      await expect(failure).toContainText("沒有一項建議可以套用，所以沒有建立新版本。");
      await expect(failure).toContainText("逐項拒絕原因未完整讀取");
      await expect(failure.getByRole("list")).toHaveCount(0);
      await expect(failure).not.toContainText("undefined");
      await region.getByRole("button", { name: "查看差異", exact: true }).first().click();
      const diff = region.getByRole("region", { name: "建議差異", exact: true });
      await expect(diff.getByRole("alert")).toContainText("暫時無法讀取差異");
      const retry = diff.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(2);
      await expect(diff).toBeFocused();
      const position = await diff.evaluate((element) => ({
        top: element.getBoundingClientRect().top,
        header: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      }));
      expect(position.top).toBeGreaterThanOrEqual(position.header);
      expect(writes).toBe(1);
      release();
      await expect(diff).toContainText("目前無法套用：目標檔案已經和建議產生當時不同");
      await expect(diff.getByRole("alert")).toHaveCount(0);
      await expect(diff.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
      await expect(diff).toBeFocused();
      await expect(failure).toContainText("逐項拒絕原因未完整讀取");
      expect(writes).toBe(1);
      expect(reads).toBe(2);
      expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("apply-refusal-recovered.png") });
    } finally {
      release();
    }
  });
}
