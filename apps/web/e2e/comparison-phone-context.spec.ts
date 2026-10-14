import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OTHER_RUN, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

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
