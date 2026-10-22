import { expect, test } from "@playwright/test";
import { TEST_CASE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  test(`ordinary wheel scrolls the page without changing the check diagram at ${width}px`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/lab/test-cases/${TEST_CASE}`);
    await page.locator("#new-criterion").fill("新的驗收條件");
    await page.locator("#check-editor-root").selectOption("sequence");
    await page.getByRole("button", { name: "開始設計", exact: true }).click();
    await expect(page.locator(".react-flow__node")).toHaveCount(1);
    const canvas = page.locator(".react-flow");
    const viewport = page.locator(".react-flow__viewport");
    await expect(viewport).toHaveAttribute("style", /scale\(/);
    for (const delta of [200, -200]) {
      await canvas.scrollIntoViewIfNeeded();
      const box = await canvas.boundingBox();
      expect(box).not.toBeNull();
      await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
      const scroll = await page.evaluate(() => scrollY);
      const transform = await viewport.evaluate((element) => element.style.transform);
      await page.mouse.wheel(0, delta);
      if (delta > 0) await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(scroll);
      else await expect.poll(() => page.evaluate(() => scrollY)).toBeLessThan(scroll);
      expect(await viewport.evaluate((element) => element.style.transform)).toBe(transform);
    }
    await expect(page.locator("#new-criterion")).toHaveValue("新的驗收條件");
  });
}
