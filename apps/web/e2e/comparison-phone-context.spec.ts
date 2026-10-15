import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OTHER_RUN, RUN, SKILL, SKILL_B, SKILL_C } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const ids of [
  [SKILL, SKILL_B],
  [SKILL, SKILL_B, SKILL_C],
]) {
  test(`phone comparison keeps long evidence inside each of ${ids.length} skill columns`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
    await page.goto(`/compare?ids=${ids.join(",")}`);
    const region = page.getByRole("region", { name: "小工具比較表，可左右捲動", exact: true });
    await expect(
      region.locator("td code").filter({ hasText: "ghcr.io/skillhub/runtime:2026.08-3" }),
    ).toHaveCount(ids.length);
    const overflow = await region.locator("td a, td code").evaluateAll((elements) =>
      elements.flatMap((element) => {
        const cell = element.closest("td")!.getBoundingClientRect();
        const range = document.createRange();
        range.selectNodeContents(element);
        return Array.from(range.getClientRects())
          .filter((rect) => rect.right > cell.right + 1 || rect.left < cell.left - 1)
          .map(() => element.textContent);
      }),
    );
    expect(overflow).toEqual([]);
    await region.evaluate((element) => {
      element.scrollLeft = element.scrollWidth;
    });
    expect(await region.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: test.info().outputPath("long-comparison-evidence.png"),
      fullPage: true,
    });
  });
}

test("phone comparison retains row context while reading the right-hand result", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  for (const name of ["試跑狀態比較表，可左右捲動", "驗收條件比較表，可左右捲動"]) {
    const region = page.getByRole("region", { name, exact: true });
    const label = region.getByRole("rowheader").first();
    await expect(label).toBeVisible();
    const before = await label.boundingBox();
    await region.evaluate((element) => {
      element.scrollLeft = element.scrollWidth;
    });
    await expect
      .poll(async () => Math.abs((await label.boundingBox())!.x - before!.x))
      .toBeLessThanOrEqual(1);
    expect(await region.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
    const rightResult = await region.getByRole("cell").last().boundingBox();
    const rowLabel = await label.boundingBox();
    expect(rightResult!.x).toBeGreaterThanOrEqual(rowLabel!.x + rowLabel!.width);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  await page
    .getByRole("region", { name: "試跑狀態比較表，可左右捲動", exact: true })
    .screenshot({ path: testInfo.outputPath("comparison-phone-scrolled.png") });
});
